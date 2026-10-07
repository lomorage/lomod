package preview

import (
	"context"
	"errors"
	"fmt"
	"image/jpeg"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common/avutil"
	"github.com/leslie-wang/govips/pkg/vips"
)

func TestMain(m *testing.M) {
	// same as lomod (handler.go): no operation cache, which would hold files open
	vips.Startup(&vips.Config{MaxCacheFiles: 0, MaxCacheMem: 0, MaxCacheSize: 0})
	code := m.Run()
	vips.Shutdown()
	os.Exit(code)
}

// writePPM writes a solid w x h binary PPM, the format DecodeToTemp asks the decoder for.
func writePPM(path string, w, h int) error {
	data := []byte(fmt.Sprintf("P6\n%d %d\n255\n", w, h))
	for i := 0; i < w*h; i++ {
		data = append(data, 200, 40, 40)
	}
	return ioutil.WriteFile(path, data, 0644)
}

// fakeEngine stands in for ffmpeg: it "decodes" any input into a fixed-size PPM.
type fakeEngine struct {
	avutil.Engine
	w, h    int
	err     error
	delay   time.Duration
	mu      sync.Mutex
	decoded []string // input paths it was asked to decode
}

func (f *fakeEngine) DecodeImage(assetpath, outpath string) error {
	f.mu.Lock()
	f.decoded = append(f.decoded, assetpath)
	f.mu.Unlock()
	time.Sleep(f.delay)
	if f.err != nil {
		return f.err
	}
	return writePPM(outpath, f.w, f.h)
}

// notAnImage writes bytes no vips loader accepts, standing in for an HEVC HEIC on a libvips
// build without an HEVC decoder.
func notAnImage(t *testing.T, dir, name string) string {
	p := filepath.Join(dir, name)
	if err := ioutil.WriteFile(p, []byte("not an image vips can read"), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func jpegSize(t *testing.T, p string) (int, int) {
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

func onlyFile(t *testing.T, dir string) string {
	entries, err := ioutil.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 1 {
		t.Fatalf("want exactly one file in %s (no leftover decode temp), got %v", dir, names)
	}
	return names[0]
}

func newTestRunner(engine avutil.Engine) *Runner {
	heifSupport.decoder = heifUntried // each test starts as a fresh lomod would
	r := NewRunner(context.Background(), "", "", "", 0755, nil, nil, 1, nil)
	r.avengine = engine
	return r
}

func TestThumbnailFallsBackToDecoderForHEIC(t *testing.T) {
	for _, name := range []string{"IMG_0001.heic", "IMG_0002.HEIF", "live_image.heic"} {
		t.Run(name, func(t *testing.T) {
			src := notAnImage(t, t.TempDir(), name)
			outDir := t.TempDir()
			dst := filepath.Join(outDir, "p_320_0.jpg")
			engine := &fakeEngine{w: 640, h: 480}

			if err := newTestRunner(engine).generatePreviewJPGByResize(src, dst, 320, 0); err != nil {
				t.Fatalf("generate preview: %v", err)
			}
			if len(engine.decoded) != 1 || engine.decoded[0] != src {
				t.Fatalf("decoder calls = %v, want one for %s", engine.decoded, src)
			}
			if w, h := jpegSize(t, dst); w != 320 || h != 240 {
				t.Fatalf("preview is %dx%d, want 320x240 from the decoded 640x480", w, h)
			}
			if got := onlyFile(t, outDir); got != "p_320_0.jpg" {
				t.Fatalf("unexpected file %s", got)
			}
		})
	}
}

func TestThumbnailFallbackForEveryOutputFormat(t *testing.T) {
	for _, gen := range []struct {
		name string
		file string
		run  func(r *Runner, src, dst string) error
	}{
		{"webp", "p.webp", func(r *Runner, src, dst string) error { return r.generatePreviewWebp(src, dst, 100, 0) }},
		{"png", "p.png", func(r *Runner, src, dst string) error { return r.generatePreviewPNG(src, dst, 100, 0) }},
		{"jpg", "p.jpg", func(r *Runner, src, dst string) error { return r.generatePreviewJPGByResize(src, dst, 100, 0) }},
	} {
		t.Run(gen.name, func(t *testing.T) {
			src := notAnImage(t, t.TempDir(), "a.heic")
			outDir := t.TempDir()
			dst := filepath.Join(outDir, gen.file)
			if err := gen.run(newTestRunner(&fakeEngine{w: 200, h: 100}), src, dst); err != nil {
				t.Fatal(err)
			}
			if fi, err := os.Stat(dst); err != nil || fi.Size() == 0 {
				t.Fatalf("no %s preview written: %v", gen.name, err)
			}
			onlyFile(t, outDir)
		})
	}
}

func TestThumbnailNoFallbackForOtherFormats(t *testing.T) {
	src := notAnImage(t, t.TempDir(), "broken.jpg")
	outDir := t.TempDir()
	engine := &fakeEngine{w: 640, h: 480}

	if err := newTestRunner(engine).generatePreviewWebp(src, filepath.Join(outDir, "p.webp"), 320, 0); err == nil {
		t.Fatal("a broken jpg must fail, not be handed to the decoder")
	}
	if len(engine.decoded) != 0 {
		t.Fatalf("decoder called for a non-HEIF file: %v", engine.decoded)
	}
}

func TestThumbnailFallbackDecoderFailure(t *testing.T) {
	src := notAnImage(t, t.TempDir(), "a.heic")
	outDir := t.TempDir()
	engine := &fakeEngine{err: errors.New("ffmpeg exploded")}

	err := newTestRunner(engine).generatePreviewWebp(src, filepath.Join(outDir, "p.webp"), 320, 0)
	if err == nil {
		t.Fatal("expected an error when both vips and the decoder fail")
	}
	if entries, _ := ioutil.ReadDir(outDir); len(entries) != 0 {
		t.Fatalf("left files behind after a failed decode: %v", entries)
	}
}

func TestNeedsDecodeFallback(t *testing.T) {
	for path, want := range map[string]bool{
		"a.heic": true, "b.HEIC": true, "c.heif": true, "d_image.heic": true,
		"e.jpg": false, "f.avif": false, "g.png": false, "h": false,
	} {
		if got := NeedsDecodeFallback(path); got != want {
			t.Errorf("NeedsDecodeFallback(%q) = %v, want %v", path, got, want)
		}
	}
}

// The background worker and an on-demand request can generate the same preview at once; each
// must decode into its own temp file rather than overwrite or delete the other's.
func TestThumbnailFallbackConcurrentSameTarget(t *testing.T) {
	src := notAnImage(t, t.TempDir(), "a.heic")
	outDir := t.TempDir()
	dst := filepath.Join(outDir, "p_320_0.jpg")
	r := newTestRunner(&fakeEngine{w: 640, h: 480, delay: 50 * time.Millisecond})

	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = r.generatePreviewJPGByResize(src, dst, 320, 0)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("generation %d: %v", i, err)
		}
	}
	if w, h := jpegSize(t, dst); w != 320 || h != 240 {
		t.Fatalf("preview is %dx%d, want 320x240", w, h)
	}
	onlyFile(t, outDir)
}

func TestHEIFSupportIsRemembered(t *testing.T) {
	// vips fails and the fallback works: later HEIF files skip vips entirely
	src := notAnImage(t, t.TempDir(), "a.heic")
	r := newTestRunner(&fakeEngine{w: 64, h: 64})
	if err := r.generatePreviewWebp(src, filepath.Join(t.TempDir(), "p.webp"), 32, 0); err != nil {
		t.Fatal(err)
	}
	if heifSupport.decoder != heifByFallback {
		t.Fatalf("decoder = %v, want heifByFallback", heifSupport.decoder)
	}

	// vips reads it (a JPEG named .heic): HEIF stays with vips, the decoder isn't used
	jpg, err := ioutil.ReadFile(filepath.Join("..", "..", "cmd", "lomod", "test", "img", "1_2003_01_17.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	src = filepath.Join(t.TempDir(), "b.heic")
	if err := ioutil.WriteFile(src, jpg, 0644); err != nil {
		t.Fatal(err)
	}
	engine := &fakeEngine{w: 64, h: 64}
	r = newTestRunner(engine)
	if err := r.generatePreviewWebp(src, filepath.Join(t.TempDir(), "p.webp"), 32, 0); err != nil {
		t.Fatal(err)
	}
	if heifSupport.decoder != heifByVips || len(engine.decoded) != 0 {
		t.Fatalf("decoder = %v, decode calls %v; want heifByVips and none", heifSupport.decoder, engine.decoded)
	}

	// a decoder failure leaves it undecided (a broken file says nothing about vips)
	r = newTestRunner(&fakeEngine{err: errors.New("bad file")})
	_ = r.generatePreviewWebp(notAnImage(t, t.TempDir(), "c.heic"), filepath.Join(t.TempDir(), "p.webp"), 32, 0)
	if heifSupport.decoder != heifUntried {
		t.Fatalf("decoder = %v, want heifUntried", heifSupport.decoder)
	}
}
