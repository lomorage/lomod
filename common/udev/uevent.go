package udev

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"unsafe"
)

// Mode determines event source: kernel events or udev-processed events.
// See libudev/libudev-monitor.c.
type Mode int

// Events that are processed by udev - much richer, with more attributes (such as vendor info, serial numbers and more).
const (
	// KernelEvent is to receive kernel event.
	KernelEvent Mode = 1
	// UdevEvent is to receive kernel event.
	UdevEvent Mode = 2
)

const (
	// SubsystemUSB is usb subsystem.
	SubsystemUSB = "usb"
	// SubsystemBlock is block storage subsystem.
	SubsystemBlock = "block"
	// SubsystemBDI is backing device info. This is mainly triggered for remote NFS/CIFS mount
	// See https://www.kernel.org/doc/Documentation/ABI/testing/sysfs-class-bdi.
	SubsystemBDI = "bdi"
)

// See: http://elixir.free-electrons.com/linux/v3.12/source/lib/kobject_uevent.c#L45.

const (
	// ADD is add action.
	ADD KObjAction = "add"
	// REMOVE is remove action.
	REMOVE KObjAction = "remove"
	// CHANGE is change action.
	CHANGE KObjAction = "change"
	// MOVE is move action.
	MOVE KObjAction = "move"
	// ONLINE is online action.
	ONLINE KObjAction = "online"
	// OFFLINE is offline action.
	OFFLINE KObjAction = "offline"
	// BIND is bind action.
	BIND KObjAction = "bind"
	// UNBIND is unbind action.
	UNBIND KObjAction = "unbind"
)

const (
	// KeyDevName is udev object key for device name
	KeyDevName = "DEVNAME"
	// KeyDevPath is udev object key for device path
	KeyDevPath = "DEVPATH"
	// KeyFsType is udev object key for file system type
	KeyFsType = "ID_FS_TYPE"
	// KeyFsUUID is udev object key for device uuid
	KeyFsUUID = "ID_FS_UUID"
	// KeySerialNo is udev object key for serial no
	KeySerialNo = "ID_SERIAL_SHORT"
	// KeyVendor is udev object key for vendor name
	KeyVendor = "ID_VENDOR"
)

// The magic value used by udev,
// see https://github.com/systemd/systemd/blob/v239/src/libudev/libudev-monitor.c#L57.
const libudevMagic = 0xfeedcafe

// KObjAction is action string for udev.
type KObjAction string

// String is to serialize object.
func (a KObjAction) String() string {
	return string(a)
}

// ParseKObjAction is to parse raw string.
func ParseKObjAction(raw string) (a KObjAction, err error) {
	a = KObjAction(raw)
	switch a {
	case ADD, REMOVE, CHANGE, MOVE, ONLINE, OFFLINE, BIND, UNBIND:
	default:
		err = fmt.Errorf("unknow kobject action (got: %s)", raw)
	}
	return
}

// UEvent is structure for udev.
type UEvent struct {
	Action    KObjAction
	KObj      string
	SubSystem string
	Env       map[string]string
}

// Pretty is to serialize uevent structure.
func (e UEvent) Pretty() string {
	var vendor, serialNo, devName, fsType, fsUUID string
	kv := []string{}
	for k, v := range e.Env {
		switch k {
		case KeyVendor:
			vendor = v
		case KeySerialNo:
			serialNo = v
		case KeyDevName:
			devName = v
		case KeyFsType:
			fsType = v
		case KeyFsUUID:
			fsUUID = v
		default:
			kv = append(kv, k+"="+v)
		}
	}
	return fmt.Sprintf("%s@%s,%s,%s,%s,%s,%s: %v\000", e.Action.String(), e.SubSystem,
		vendor, serialNo, devName, fsType, fsUUID, kv)
}

// String is string output of uevent
func (e UEvent) String() string {
	rv := fmt.Sprintf("%s@%s\000", e.Action.String(), e.KObj)
	for k, v := range e.Env {
		rv += k + "=" + v + "\000"
	}
	return rv
}

// Bytes is to return serialized data in byte array.
func (e UEvent) Bytes() []byte {
	return []byte(e.String())
}

// Equal is to compile another uevent.
func (e UEvent) Equal(e2 UEvent) (bool, error) {
	if e.Action != e2.Action {
		return false, fmt.Errorf("Wrong action (got: %s, wanted: %s)", e.Action, e2.Action)
	}

	if e.KObj != e2.KObj {
		return false, fmt.Errorf("Wrong kobject (got: %s, wanted: %s)", e.KObj, e2.KObj)
	}

	if len(e.Env) != len(e2.Env) {
		return false, fmt.Errorf("Wrong length of env (got: %d, wanted: %d)", len(e.Env), len(e2.Env))
	}

	var found bool
	for k, v := range e.Env {
		found = false
		for i, e := range e2.Env {
			if i == k && v == e {
				found = true
				break
			}
		}
		if !found {
			return false, fmt.Errorf("Unable to find %s=%s env var from uevent", k, v)
		}
	}
	return true, nil
}

// GetDevName returns device name defined in env
func (e UEvent) GetDevName() string {
	return e.Env[KeyDevName]
}

// GetDevPath returns device path defined in env
func (e UEvent) GetDevPath() string {
	return e.Env[KeyDevPath]
}

// GetFsType returns fs type defined in env
func (e UEvent) GetFsType() string {
	return e.Env[KeyFsType]
}

// GetFsUUID returns fs uuid defined in env
func (e UEvent) GetFsUUID() string {
	return e.Env[KeyFsUUID]
}

// GetSerialNo returns serial number defined in env
func (e UEvent) GetSerialNo() string {
	return e.Env[KeySerialNo]
}

// GetVendor returns vendor defined in env
func (e UEvent) GetVendor() string {
	return e.Env[KeyVendor]
}

// Parse udev event created by udevd.
// The format of the data header is internal to udev and defined in libudev-monitor.c - see the udev_monitor_netlink_header struct.
// go-udev only looks at the "magic" number to filter out possibly invalid packets, and at the payload offset. Other fields of the header
// are ignored.
// Note, only some of the fields of the header use network byte order, for the rest udev uses native byte order of the platform.
func parseUdevEvent(raw []byte) (e *UEvent, err error) {
	// the magic number is stored in network byte order.
	magic := binary.BigEndian.Uint32(raw[8:])
	if magic != libudevMagic {
		return nil, fmt.Errorf("cannot parse libudev event: magic number mismatch")
	}

	// the payload offset int is stored in native byte order.
	payloadoff := *(*uint32)(unsafe.Pointer(&raw[16]))
	if payloadoff >= uint32(len(raw)) {
		return nil, fmt.Errorf("cannot parse libudev event: invalid data offset")
	}

	fields := bytes.Split(raw[payloadoff:], []byte{0x00}) // 0x00 = end of string
	if len(fields) == 0 {
		err = fmt.Errorf("cannot parse libudev event: data missing")
		return
	}

	envdata := make(map[string]string, 0)
	for _, envs := range fields[0 : len(fields)-1] {
		env := bytes.Split(envs, []byte("="))
		if len(env) != 2 {
			err = fmt.Errorf("cannot parse libudev event: invalid env data")
			return
		}
		envdata[string(env[0])] = string(env[1])
	}

	var action KObjAction
	action, err = ParseKObjAction(strings.ToLower(envdata["ACTION"]))
	if err != nil {
		return
	}

	// XXX: do we need kobj?
	kobj := envdata["DEVPATH"]

	e = &UEvent{
		Action: action,
		KObj:   kobj,
		Env:    envdata,
	}
	return
}

// ParseUEvent is to parse raw data.
func ParseUEvent(raw []byte) (e *UEvent, err error) {
	if len(raw) > 40 && bytes.Compare(raw[:8], []byte("libudev\x00")) == 0 {
		return parseUdevEvent(raw)
	}
	fields := bytes.Split(raw, []byte{0x00}) // 0x00 = end of string

	if len(fields) == 0 {
		err = fmt.Errorf("Wrong uevent format")
		return
	}

	headers := bytes.Split(fields[0], []byte("@")) // 0x40 = @
	if len(headers) != 2 {
		err = fmt.Errorf("Wrong uevent header")
		return
	}

	action, err := ParseKObjAction(string(headers[0]))
	if err != nil {
		return
	}

	e = &UEvent{
		Action: action,
		KObj:   string(headers[1]),
		Env:    make(map[string]string, 0),
	}

	for _, envs := range fields[1 : len(fields)-1] {
		env := bytes.Split(envs, []byte("="))
		if len(env) != 2 {
			err = fmt.Errorf("Wrong uevent env")
			return
		}
		e.Env[string(env[0])] = string(env[1])
	}
	return
}
