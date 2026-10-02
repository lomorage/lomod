// +build !windows

package timezone

import (
	"runtime"
	"strings"

	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

// refer https://socketloop.com/tutorials/golang-display-list-of-timezones-with-gmt

// zoneDirs adapted from https://golang.org/src/time/zoneinfo_unix.go

// https://golang.org/doc/install/source#environment
// list of available GOOS as of 10th Feb 2017
// android, darwin, dragonfly, freebsd, linux, netbsd, openbsd, plan9, solaris,windows

var zoneDirs = map[string]string{
	"android":   "/system/usr/share/zoneinfo/",
	"darwin":    "/usr/share/zoneinfo/",
	"dragonfly": "/usr/share/zoneinfo/",
	"freebsd":   "/usr/share/zoneinfo/",
	"linux":     "/usr/share/zoneinfo/",
	"netbsd":    "/usr/share/zoneinfo/",
	"openbsd":   "/usr/share/zoneinfo/",
	// "plan9":"/adm/timezone/", -- no way to test this platform
	"solaris": "/usr/share/lib/zoneinfo/",
	"windows": `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Time Zones\`,
}

func isLink(p string) (bool, error) {
	fi, err := os.Lstat(p)
	if err != nil {
		return false, err
	}
	return fi.Mode()&os.ModeSymlink != 0, nil
}

func resolvLink(p string) (string, error) {
	yes, err := isLink(p)
	if err != nil {
		return p, err
	}
	if !yes {
		return p, nil
	}
	np, err := filepath.EvalSymlinks(p)
	if err != nil {
		return p, err
	}
	yes, err = isLink(np)
	if err != nil {
		return p, err
	}
	if yes {
		return resolvLink(np)
	}
	return np, err
}

// List available time zone in the system.
func List() ([]string, error) {
	dir, ok := zoneDirs[runtime.GOOS]
	if !ok {
		return nil, errors.Errorf("Unsupported platform: %s", runtime.GOOS)
	}
	zonedir, err := resolvLink(dir)
	if err != nil {
		return nil, err
	}
	tzs := []string{}
	err = filepath.Walk(zonedir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		tz := strings.TrimPrefix(path, zonedir)
		if filepath.IsAbs(tz) {
			tz = strings.TrimPrefix(tz, "/")
		}
		tzs = append(tzs, tz)
		return nil
	})
	return tzs, err
}
