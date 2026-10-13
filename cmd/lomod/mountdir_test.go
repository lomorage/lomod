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

func TestResolveStorageDirs(t *testing.T) {
	for _, goos := range []string{"darwin", "windows", "linux"} {
		t.Run(goos, func(t *testing.T) {
			defaultMount := "/media"
			if goos == "darwin" || goos == "windows" {
				defaultMount = "/base"
			}
			envDefaultMount := "/media"
			if goos == "darwin" || goos == "windows" {
				envDefaultMount = "/env/base"
			}
			cases := []struct {
				name                string
				mount               string
				set                 bool
				baseEnv, mountEnv   string
				wantBase, wantMount string
			}{
				{"defaults", "/media", false, "", "", "/base", defaultMount},
				{"base environment", "/media", false, "/env/base", "", "/env/base", envDefaultMount},
				{"explicit mount", "/cli/mount", true, "", "", "/base", "/cli/mount"},
				{"base environment with explicit mount", "/cli/mount", true, "/env/base", "", "/env/base", "/cli/mount"},
				{"mount environment", "/media", false, "", "/env/mount", "/base", "/env/mount"},
				{"both environments", "/media", false, "/env/base", "/env/mount", "/env/base", "/env/mount"},
				{"mount environment overrides explicit mount", "/cli/mount", true, "", "/env/mount", "/base", "/env/mount"},
				{"both environments with explicit mount", "/cli/mount", true, "/env/base", "/env/mount", "/env/base", "/env/mount"},
			}
			for _, c := range cases {
				t.Run(c.name, func(t *testing.T) {
					base, mount := resolveStorageDirs(goos, "/base", c.mount, c.set, c.baseEnv, c.mountEnv)
					if base != c.wantBase || mount != c.wantMount {
						t.Errorf("resolveStorageDirs() = (%q, %q), want (%q, %q)", base, mount, c.wantBase, c.wantMount)
					}
				})
			}
		})
	}
}
