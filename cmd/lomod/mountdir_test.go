package main

import "testing"

func TestResolveMountDir(t *testing.T) {
	cases := []struct {
		goos, base, mount string
		set               bool
		want              string
	}{
		{"darwin", "/Users/u/Pictures/Lomorage", "/media", false, "/Users/u/Pictures/Lomorage"},
		{"darwin", "/Users/u/Pictures/Lomorage", "/Volumes/Ext", true, "/Volumes/Ext"},
		{"windows", `C:\\Lomorage`, "/media", false, `C:\\Lomorage`},
		{"windows", `C:\\Lomorage`, `D:\\Photos`, true, `D:\\Photos`},
		{"linux", "/var/lib/lomod", "/media", false, "/media"},
		{"linux", "/var/lib/lomod", "/mnt/usb", true, "/mnt/usb"},
	}
	for _, c := range cases {
		if got := resolveMountDir(c.goos, c.base, c.mount, c.set); got != c.want {
			t.Errorf("resolveMountDir(%q,%q,%q,%v)=%q want %q", c.goos, c.base, c.mount, c.set, got, c.want)
		}
	}
}
