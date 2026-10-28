package asset

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/jpeg"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"bitbucket.org/lomoware/lomo-backend/common"
	"github.com/leslie-wang/govips/pkg/vips"
)

func TestMain(m *testing.M) {
	// same as lomod (handler.go): no operation cache, which would hold files open
	vips.Startup(&vips.Config{MaxCacheFiles: 0, MaxCacheMem: 0, MaxCacheSize: 0})
	code := m.Run()
	vips.Shutdown()
	os.Exit(code)
}

func decodedJPEGSize(t *testing.T, p string) (int, int) {
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cfg, err := jpeg.DecodeConfig(f)
	if err != nil {
		t.Fatalf("%s is not a jpeg: %v", p, err)
	}
	return cfg.Width, cfg.Height
}

// XcodeImage used vipsload, which only reads vips' own .v format, so every full-size
// transcode (e.g. /asset/<id>?icodec=jpg) failed.
func TestXcodeImageFullSize(t *testing.T) {
	for _, tc := range []struct {
		fixture string
		w, h    int
	}{
		{"1_2003_01_17.jpg", 687, 1024},
		{"3_2003_11_01.jpg", 1600, 900},
		{"9_2013_07_28.png", 0, 0},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			dst := filepath.Join(t.TempDir(), "full.jpg")
			src := filepath.Join("..", "..", "cmd", "lomod", "test", "img", tc.fixture)
			if err := XcodeImage(context.Background(), src, dst, 0755, nil); err != nil {
				t.Fatalf("transcode %s: %v", tc.fixture, err)
			}
			w, h := decodedJPEGSize(t, dst)
			if tc.w != 0 && (w != tc.w || h != tc.h) {
				t.Fatalf("got %dx%d, want the original %dx%d", w, h, tc.w, tc.h)
			}
		})
	}
}

func TestXcodeImageDecodeFallback(t *testing.T) {
	src := filepath.Join(t.TempDir(), "IMG_0001.heic")
	if err := ioutil.WriteFile(src, []byte("not an image vips can read"), 0644); err != nil {
		t.Fatal(err)
	}
	outDir := t.TempDir()
	dst := filepath.Join(outDir, "full.jpg")
	calls := 0
	decode := func(in, out string) error {
		calls++
		data := []byte("P6\n300 200\n255\n")
		for i := 0; i < 300*200; i++ {
			data = append(data, 10, 120, 200)
		}
		return ioutil.WriteFile(out, data, 0644)
	}

	if err := XcodeImage(context.Background(), src, dst, 0755, decode); err != nil {
		t.Fatalf("transcode with fallback: %v", err)
	}
	if calls != 1 {
		t.Fatalf("decoder called %d times, want 1", calls)
	}
	if w, h := decodedJPEGSize(t, dst); w != 300 || h != 200 {
		t.Fatalf("got %dx%d, want 300x200", w, h)
	}
	entries, _ := ioutil.ReadDir(outDir)
	if len(entries) != 1 {
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("decode temp file left behind: %v", names)
	}

	// without a decoder it fails as before
	if err := XcodeImage(context.Background(), src, filepath.Join(outDir, "other.jpg"), 0755, nil); err == nil {
		t.Fatal(fmt.Sprintf("expected %s to fail without a decoder", src))
	}
}

// xcodeImageOnce only ever asked vips for a JPEG or a WebP, but took the output
// extension from the caller's icodec. So /asset/<id>?icodec=avif (or png, bmp, tif, gif)
// wrote WebP bytes into a file named .avif, which http.ServeFile then served as
// Content-Type: image/avif with a year-long immutable cache.
func TestXcodeImageRejectsFormatsItCannotWrite(t *testing.T) {
	src := filepath.Join("..", "..", "cmd", "lomod", "test", "img", "1_2003_01_17.jpg")

	for _, tc := range []struct {
		ext   string
		magic []byte
	}{
		{"jpg", []byte{0xff, 0xd8}},
		{"webp", []byte("RIFF")},
		{"png", []byte{0x89, 'P', 'N', 'G'}},
	} {
		t.Run("writes real "+tc.ext, func(t *testing.T) {
			dst := filepath.Join(t.TempDir(), "full."+tc.ext)
			if err := XcodeImage(context.Background(), src, dst, 0755, nil); err != nil {
				t.Fatalf("transcode to %s: %v", tc.ext, err)
			}
			b, err := ioutil.ReadFile(dst)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.HasPrefix(b, tc.magic) {
				t.Fatalf("%s holds %x..., want a real %s starting %x",
					filepath.Base(dst), b[:4], tc.ext, tc.magic)
			}
		})
	}

	for _, extension := range []string{"avif", "bmp", "tif", "gif"} {
		t.Run("rejects "+extension, func(t *testing.T) {
			outDir := t.TempDir()
			dst := filepath.Join(outDir, "full."+extension)
			err := XcodeImage(context.Background(), src, dst, 0755, nil)
			if err != common.ErrNotImplementedFormat {
				t.Fatalf("transcode to %s = %v, want ErrNotImplementedFormat so the "+
					"handler answers 400 rather than serving mislabelled bytes", extension, err)
			}
			entries, _ := ioutil.ReadDir(outDir)
			if len(entries) != 0 {
				names := []string{}
				for _, e := range entries {
					names = append(names, e.Name())
				}
				t.Fatalf("left a file behind for a format it cannot write: %v", names)
			}
		})
	}
}

// The format check used to live in xcodeImageOnce, the closure XcodeImage hands to
// preview.WithDecodeFallback. For a HEIF-family master that ran only after the fallback
// had engaged: a rejected format paid for a full-resolution ffmpeg decode first, and when
// that decode failed decodeThenAttempt wrapped the sentinel, so the client got a 500
// instead of a 400.
func TestXcodeImageRejectsBeforeDecoding(t *testing.T) {
	src := filepath.Join(t.TempDir(), "IMG_0001.heic")
	if err := ioutil.WriteFile(src, []byte("not an image vips can read"), 0644); err != nil {
		t.Fatal(err)
	}

	calls := 0
	decode := func(_, out string) error {
		calls++
		data := []byte("P6\n64 64\n255\n")
		for i := 0; i < 64*64; i++ {
			data = append(data, 1, 2, 3)
		}
		return ioutil.WriteFile(out, data, 0644)
	}

	outDir := t.TempDir()
	err := XcodeImage(context.Background(), src, filepath.Join(outDir, "x.bmp"), 0755, decode)
	if err != common.ErrNotImplementedFormat {
		t.Fatalf("err = %v, want ErrNotImplementedFormat", err)
	}
	if calls != 0 {
		t.Fatalf("decoder called %d times for a format that cannot be written; "+
			"the master must not be decoded for a request that can only fail", calls)
	}
	if entries, _ := ioutil.ReadDir(outDir); len(entries) != 0 {
		t.Fatalf("created the preview directory's contents for a doomed request: %v", entries)
	}

	// a failing decoder must not turn the 400 into a 500 by wrapping the sentinel
	failing := func(_, _ string) error { return errors.New("no ffmpeg on this box") }
	err = XcodeImage(context.Background(), src, filepath.Join(t.TempDir(), "y.bmp"), 0755, failing)
	if err != common.ErrNotImplementedFormat {
		t.Fatalf("err = %v (wrapped: %v), want the bare sentinel so errToHTTPStatus still answers 400",
			err, err != nil)
	}

	// a format it can write still reaches the decoder, as before
	calls = 0
	if err := XcodeImage(context.Background(), src, filepath.Join(t.TempDir(), "z.jpg"), 0755, decode); err != nil {
		t.Fatalf("transcode to jpg: %v", err)
	}
	if calls != 1 {
		t.Fatalf("decoder called %d times for jpg, want 1", calls)
	}
}
