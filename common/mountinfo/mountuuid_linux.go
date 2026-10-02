package mountinfo

import (
	"bufio"
	"bytes"
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/cmd"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	arpIPAddr int = iota
	arpHWType     // nolint
	arpFlags      // nolint
	arpHWAddr
	arpMask   // nolint
	arpDevice // nolint
)

const (
	arpDir       = "/proc/net/arp"
	uuidDir      = "/dev/disk/by-uuid/"
	blkidUUIDKey = "UUID"
)

const (
	// DefaultRemoteMountUUID is default remote mount UUID as last resort
	DefaultRemoteMountUUID = "remote_mount"
)

// GetDeviceUUID read device UUID from os
func GetDeviceUUID(info *Info) (string, error) {
	if info.IsRemoteMount() {
		return getRemoteMountUUID(info.Source, info.FSType)
	}
	uuid, err := getDeviceUUIDByLsblk(info.Source)
	if err == nil {
		if uuid != `""` {
			return uuid, nil
		}
		logrus.Tracef("got empty %s uuid via lsblk: %v", info.Source, err)
	} else {
		logrus.Tracef("read %s uuid via lsblk: %v", info.Source, err)
	}
	uuid, err = getDeviceUUIDByFS(info.Source)
	if err == nil {
		if uuid != "" {
			return uuid, nil
		}
		logrus.Tracef("unable to find %s uuid from %s: %v", info.Source, uuidDir, err)
	} else {
		logrus.Tracef("read %s uuid from %s: %v", info.Source, uuidDir, err)
	}
	uuid, err = getDeviceUUIDByBlkid(info.Source)
	if err == nil {
		if uuid != "" {
			return uuid, nil
		}
		logrus.Tracef("got empty %s uuid via blkid: %v", info.Source, err)
	} else {
		logrus.Tracef("read %s uuid via blkid: %v", info.Source, err)
	}
	return "", common.ErrDeviceNotLocate
}

func getRemoteMountUUID(source, fsType string) (string, error) {
	host := parseRemoteMountHost(source, fsType)
	if net.ParseIP(host) == nil {
		logrus.Tracef("remote mount host %s is domain name, use it as UUID", host)
		return host, nil
	}

	f, err := os.Open(arpDir)
	if err != nil {
		return "", err
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	s.Scan() // skip the field descriptions

	for s.Scan() {
		line := s.Text()
		fields := strings.Fields(line)
		if fields[arpIPAddr] != host {
			continue
		}
		return fields[arpHWAddr], nil
	}
	logrus.Warnf("unable to find %s's MAC address", host)
	return DefaultRemoteMountUUID, nil
}

// in case its form is username:password@host:port
func normalizeRemoteMountHost(host string) string {
	parts := strings.Split(host, "@")
	if len(parts) > 1 {
		host = parts[len(parts)-1]
	}
	return strings.Split(host, ":")[0]
}

func parseRemoteMountHost(source, fsType string) string {
	logrus.Tracef("parse remote mount source %s: %s", fsType, source)
	// try to use url package to parse firstly
	u, err := url.Parse(source)
	if err == nil {
		return normalizeRemoteMountHost(u.Host)
	}
	for {
		if !strings.HasPrefix(source, "/") {
			break
		}
		source = strings.TrimPrefix(source, "/")
	}
	delimiter := ":"
	if fsType == cifs {
		delimiter = "/"
	}
	return normalizeRemoteMountHost(strings.Split(source, delimiter)[0])
}

// read device UUID from /dev/disk/by-uuid
func getDeviceUUIDByFS(blockDev string) (string, error) {
	logrus.Tracef("probe uuid directory: %s", uuidDir)
	files, err := ioutil.ReadDir(uuidDir)
	if err != nil {
		return "", err
	}
	for _, f := range files {
		dir := filepath.Join(uuidDir, f.Name())
		link, err := os.Readlink(dir)
		if err != nil {
			logrus.Tracef("readlink %s: %v", dir, err)
			continue
		}
		logrus.Tracef("%s -> %s\n", dir, link)
		if filepath.Base(link) == filepath.Base(blockDev) {
			return f.Name(), nil
		}
	}
	return "", common.ErrNotFound
}

func getDeviceUUIDByBlkid(blockDev string) (string, error) {
	out, err := cmd.RunWithSudo("blkid", "-o", "export", "-p", "--uuid", "-i", blockDev)
	if err != nil {
		return "", err
	}
	s := bufio.NewScanner(bytes.NewBuffer(out))
	for s.Scan() {
		parts := strings.SplitN(s.Text(), "=", 2)
		if parts[0] == blkidUUIDKey {
			if len(parts) == 2 {
				return parts[1], nil
			}
			return "", errors.Errorf("empty UUID in output of blkid: %s", string(out))
		}
	}
	return "", common.ErrNotFound
}

func getDeviceUUIDByLsblk(blockDev string) (string, error) {
	out, err := cmd.RunWithSudo("lsblk", "-o", "UUID", "-P", blockDev)
	if err != nil {
		return "", err
	}
	parts := strings.SplitN(strings.TrimSpace(string(out)), "=", 2)
	if parts[0] == blkidUUIDKey {
		if len(parts) == 2 {
			return parts[1], nil
		}
		return "", errors.Errorf("empty UUID in output of lsblk: %s", string(out))
	}
	return "", common.ErrNotFound
}
