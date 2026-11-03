//go:build darwin
// +build darwin

package preview

import (
	"os"
	"path/filepath"
)

// libvips 8.13+ can build some of its loaders -- heif (HEIC/AVIF), jxl, magick, openslide --
// as dynamically loaded modules instead of linking them into libvips itself, and Homebrew's
// vips is built that way. vips_init looks for them in <libdir>/vips-modules-<major>.<minor>,
// and takes libdir from vips_guess_libdir(argv0, "VIPSHOME"), which tries, in order:
//
//  1. $VIPSHOME, if set;
//  2. a guess from argv0, which only works when the program sits in a bin/ directory -- and
//     govips passes the literal string "govips" as argv0, so it never resolves to anything;
//  3. otherwise the prefix compiled into libvips, which on a Homebrew build is
//     /opt/homebrew/Cellar/vips/<version>/lib and does not exist on an end user's Mac.
//
// The release bundle puts lomod at its root, libvips in libs/ and the modules in
// lib/vips-modules-<version>/ (scripts/macos/collect-deps.sh), so none of the three finds
// them: without VIPSHOME no module loads, heifload/jxlload/magickload do not exist as
// operations, and HEIF/AVIF previews fall through to ffmpeg while JPEG XL fails outright
// (lomorage/lomod#20).
//
// Only set it when the directory holding this binary really has that layout, so a dev build
// run from the repo or through `go run` keeps using Homebrew's compiled-in prefix instead of
// being pointed at a directory with no modules in it. vips_init runs from govips's Startup,
// never from a package init, so doing this in one is early enough.
func init() {
	if os.Getenv("VIPSHOME") != "" {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return
	}
	home := filepath.Dir(exe)
	modules, err := filepath.Glob(filepath.Join(home, "lib", "vips-modules-*"))
	if err != nil || len(modules) == 0 {
		return
	}
	os.Setenv("VIPSHOME", home)
}
