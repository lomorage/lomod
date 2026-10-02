package main

import (
	"archive/tar"
	"compress/gzip"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

type tarEntry struct {
	hdr  tar.Header
	body string
}

func writeTarGz(t *testing.T, path string, entries []tarEntry) {
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		e.hdr.Size = int64(len(e.body))
		if err := tw.WriteHeader(&e.hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(e.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestUncompressTarGz(t *testing.T) {
	tmp, err := ioutil.TempDir("", "lomoupg-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	// Same shape as `tar -C dist-macos -czf ... .`: a leading "./" entry, "./"-prefixed names.
	src := filepath.Join(tmp, "release")
	writeTarGz(t, src, []tarEntry{
		{hdr: tar.Header{Name: "./", Typeflag: tar.TypeDir, Mode: 0755}},
		{hdr: tar.Header{Name: "./lomod", Typeflag: tar.TypeReg, Mode: 0755}, body: "binary"},
		{hdr: tar.Header{Name: "./exiftool-pkg/bin/exiftool", Typeflag: tar.TypeReg, Mode: 0755}, body: "perl"},
		{hdr: tar.Header{Name: "./exiftool", Typeflag: tar.TypeSymlink, Linkname: "exiftool-pkg/bin/exiftool"}},
		{hdr: tar.Header{Name: "./libs/", Typeflag: tar.TypeDir, Mode: 0755}},
		{hdr: tar.Header{Name: "./libs/libvips.dylib", Typeflag: tar.TypeReg, Mode: 0644}, body: "dylib"},
	})

	dst := filepath.Join(tmp, "out")
	if err := uncompress(src, dst); err != nil {
		t.Fatal(err)
	}

	fi, err := os.Stat(filepath.Join(dst, "lomod"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0100 == 0 {
		t.Errorf("lomod lost its executable bit: %v", fi.Mode())
	}
	if link, err := os.Readlink(filepath.Join(dst, "exiftool")); err != nil || link != "exiftool-pkg/bin/exiftool" {
		t.Errorf("exiftool symlink = %q, %v", link, err)
	}
	if b, err := ioutil.ReadFile(filepath.Join(dst, "exiftool")); err != nil || string(b) != "perl" {
		t.Errorf("read through exiftool symlink = %q, %v", b, err)
	}
	if b, err := ioutil.ReadFile(filepath.Join(dst, "libs", "libvips.dylib")); err != nil || string(b) != "dylib" {
		t.Errorf("libs/libvips.dylib = %q, %v", b, err)
	}
}

func TestUncompressTarGzRejectsEscapes(t *testing.T) {
	cases := map[string][]tarEntry{
		"path": {
			{hdr: tar.Header{Name: "../evil", Typeflag: tar.TypeReg, Mode: 0644}, body: "x"},
		},
		"symlink": {
			{hdr: tar.Header{Name: "./link", Typeflag: tar.TypeSymlink, Linkname: "../outside"}},
			{hdr: tar.Header{Name: "./link/evil", Typeflag: tar.TypeReg, Mode: 0644}, body: "x"},
		},
	}
	for name, entries := range cases {
		tmp, err := ioutil.TempDir("", "lomoupg-test")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmp)

		src := filepath.Join(tmp, "release")
		writeTarGz(t, src, entries)
		if err := uncompress(src, filepath.Join(tmp, "out")); err == nil {
			t.Errorf("%s: expected an error", name)
		}
		if _, err := os.Stat(filepath.Join(tmp, "evil")); err == nil {
			t.Errorf("%s: wrote outside the destination", name)
		}
		if _, err := os.Stat(filepath.Join(tmp, "outside", "evil")); err == nil {
			t.Errorf("%s: wrote outside the destination", name)
		}
	}
}
