package udev

import (
	"context"
	"errors"
	"os"
	"syscall"

	"github.com/sirupsen/logrus"
)

var matchSubsystem = map[string]struct{}{SubsystemUSB: {}, SubsystemBlock: {}, SubsystemBDI: {}}

// NetlinkConn is generic connection.
type NetlinkConn struct {
	Fd   int
	Addr syscall.SockaddrNetlink
}

// UEventConn is udev event connection.
type UEventConn struct {
	NetlinkConn
}

// NewConn allow to connect to system socket AF_NETLINK with family NETLINK_KOBJECT_UEVENT to
// catch events about block/char device
// see:
// - http://elixir.free-electrons.com/linux/v3.12/source/include/uapi/linux/netlink.h#L23
// - http://elixir.free-electrons.com/linux/v3.12/source/include/uapi/linux/socket.h#L11.
func NewConn(mode Mode) (*UEventConn, error) {
	var err error
	c := &UEventConn{}
	if c.Fd, err = syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_RAW, syscall.NETLINK_KOBJECT_UEVENT); err != nil {
		return nil, err
	}

	c.Addr = syscall.SockaddrNetlink{
		Family: syscall.AF_NETLINK,
		Groups: uint32(mode),
	}

	if err := syscall.Bind(c.Fd, &c.Addr); err != nil {
		if err := syscall.Close(c.Fd); err != nil {
			logrus.Warnf("close uevent socket fd %d: %v", c.Fd, err)
		}
		return nil, err
	}
	return c, nil
}

// Close allow to close file descriptor and socket bound.
func (c *UEventConn) Close() error {
	return syscall.Close(c.Fd)
}

func (c *UEventConn) msgPeek() (int, *[]byte, error) {
	var n int
	var err error
	buf := make([]byte, os.Getpagesize())
	for {
		// Just read how many bytes are available in the socket
		// Warning: syscall.MSG_PEEK is a blocking call
		if n, _, err = syscall.Recvfrom(c.Fd, buf, syscall.MSG_PEEK); err != nil {
			return n, &buf, err
		}

		// If all message could be store inside the buffer : break
		if n < len(buf) {
			break
		}

		// Increase size of buffer if not enough
		buf = make([]byte, len(buf)+os.Getpagesize())
	}
	return n, &buf, err
}

func (c *UEventConn) msgRead(buf *[]byte) error {
	if buf == nil {
		return errors.New("empty buffer")
	}

	n, _, err := syscall.Recvfrom(c.Fd, *buf, 0)
	if err != nil {
		return err
	}

	// Extract only real data from buffer and return that
	*buf = (*buf)[:n]

	return nil
}

// Monitor run in background a worker to read netlink msg in loop and notify
// when msg receive inside a queue using channel.
// To be notified with only relevant message, use Matcher.
func (c *UEventConn) Monitor(ctx context.Context, queue chan UEvent) error {
	bufToRead := make(chan *[]byte, 1)
	count := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err() // stop iteration in case of stop signal received
		case buf := <-bufToRead: // Read one by one
			err := c.msgRead(buf)
			if err != nil {
				logrus.Warnf("Unable to read uevent, err: %v", err)
				return err // stop iteration in case of error
			}

			uevent, err := ParseUEvent(*buf)
			if err != nil {
				logrus.Warnf("Unable to parse uevent, err: %v", err)
				continue // Drop uevent if not known
			}

			if !c.match(uevent) {
				continue
			}
			count++
			logrus.Infof("#%d uevent: %v", count, *uevent)
			queue <- *uevent
		default:
			_, buf, err := c.msgPeek()
			if err != nil {
				logrus.Warnf("Unable to check available uevent, err: %v", err)
				return err // stop iteration in case of error
			}
			bufToRead <- buf
		}
	}
}

func (c *UEventConn) match(uevent *UEvent) (ok bool) {
	if uevent.Action != ADD && uevent.Action != REMOVE {
		return false
	}
	uevent.SubSystem, ok = uevent.Env["SUBSYSTEM"]
	if !ok {
		return
	}
	_, ok = matchSubsystem[uevent.SubSystem]
	return ok
}
