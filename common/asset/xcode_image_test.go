package asset

import (
	"context"
	"fmt"
	"image/jpeg"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

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
