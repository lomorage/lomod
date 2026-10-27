package preview

import (
	"context"
	"errors"
	"fmt"
	"image/jpeg"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
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

func resetHEIFSupport() {
	for c := range heifSupport.decoder {
		heifSupport.decoder[c] = heifUntried
	}
}

func newTestRunner(engine avutil.Engine) *Runner {
	resetHEIFSupport() // each test starts as a fresh lomod would
	r := NewRunner(context.Background(), "", "", "", 0755, nil, nil, 1, nil)
	r.avengine = engine
	return r
}

func TestThumbnailFallsBackToDecoderForHEIC(t *testing.T) {
	for _, name := range []string{"IMG_0001.heic", "IMG_0002.HEIF", "live_image.heic", "IMG_0003.avif"} {
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
		"f.avif": true, "i.AVIF": true,
		"e.jpg": false, "g.png": false, "h": false,
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
	// a file vips can't read still falls back for that file, but a non-decoder
	// error must not latch ffmpeg for every later HEIC
	src := notAnImage(t, t.TempDir(), "a.heic")
	engine := &fakeEngine{w: 64, h: 64}
	r := newTestRunner(engine)
	if err := r.generatePreviewWebp(src, filepath.Join(t.TempDir(), "p.webp"), 32, 0); err != nil {
		t.Fatal(err)
	}
	if len(engine.decoded) != 1 {
		t.Fatalf("decoder calls = %v, want one fallback for this file", engine.decoded)
	}
	if heifSupport.decoder[codecHEVC] != heifUntried {
		t.Fatalf("decoder = %v, want heifUntried after a file-specific vips failure",
			heifSupport.decoder[codecHEVC])
	}

	// vips reads it (a JPEG named .heic): the decoder isn't used, and one success
	// does not record "this build decodes HEIF"
	jpg, err := ioutil.ReadFile(filepath.Join("..", "..", "cmd", "lomod", "test", "img", "1_2003_01_17.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	src = filepath.Join(t.TempDir(), "b.heic")
	if err := ioutil.WriteFile(src, jpg, 0644); err != nil {
		t.Fatal(err)
	}
	engine = &fakeEngine{w: 64, h: 64}
	r = newTestRunner(engine)
	if err := r.generatePreviewWebp(src, filepath.Join(t.TempDir(), "p.webp"), 32, 0); err != nil {
		t.Fatal(err)
	}
	if heifSupport.decoder[codecHEVC] != heifUntried || len(engine.decoded) != 0 {
		t.Fatalf("decoder = %v, decode calls %v; want heifUntried and none",
			heifSupport.decoder[codecHEVC], engine.decoded)
	}

	// a decoder failure leaves it undecided (a broken file says nothing about vips)
	r = newTestRunner(&fakeEngine{err: errors.New("bad file")})
	_ = r.generatePreviewWebp(notAnImage(t, t.TempDir(), "c.heic"), filepath.Join(t.TempDir(), "p.webp"), 32, 0)
	if heifSupport.decoder[codecHEVC] != heifUntried {
		t.Fatalf("decoder = %v, want heifUntried", heifSupport.decoder[codecHEVC])
	}
}

func TestMissingDecoderLatchesFallback(t *testing.T) {
	defer resetHEIFSupport()
	resetHEIFSupport()
	src := filepath.Join(t.TempDir(), "a.heic")
	dst := filepath.Join(t.TempDir(), "p.jpg")
	vipsCalls := 0
	err := WithDecodeFallback(src, dst,
		func(_, out string) error { return writePPM(out, 8, 8) },
		func(path string) error {
			if path == src {
				vipsCalls++
				return errors.New("heif: Unsupported feature: Unsupported codec (4.3000)")
			}
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if vipsCalls != 1 || heifSupport.decoder[codecHEVC] != heifByFallback {
		t.Fatalf("vips calls %d, decoder %v; want 1 and heifByFallback",
			vipsCalls, heifSupport.decoder[codecHEVC])
	}

	err = WithDecodeFallback(src, filepath.Join(t.TempDir(), "q.jpg"),
		func(_, out string) error { return writePPM(out, 8, 8) },
		func(path string) error {
			if path == src {
				t.Fatal("vips attempt after a missing-decoder latch")
			}
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
}

// A libheif build decodes HEVC and AV1 independently: Windows libvips has AV1 but not
// HEVC, the libheif on the devices has HEVC but is too old for AV1. One missing decoder
// must not route the other codec through ffmpeg.
func TestMissingDecoderLatchesOnlyItsOwnCodec(t *testing.T) {
	defer resetHEIFSupport()

	for _, tc := range []struct {
		name         string
		missing      string // the file whose codec libvips can't decode
		other        string // the other codec, which it decodes fine
		latched      heifCodec
		stillUntried heifCodec
	}{
		{"AV1 missing", "a.avif", "b.heic", codecAV1, codecHEVC},
		{"HEVC missing", "c.heic", "d.avif", codecHEVC, codecAV1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetHEIFSupport()
			missing := filepath.Join(t.TempDir(), tc.missing)
			decode := func(_, out string) error { return writePPM(out, 8, 8) }

			if err := WithDecodeFallback(missing, filepath.Join(t.TempDir(), "p.jpg"), decode,
				func(path string) error {
					if path == missing {
						return errors.New("heif: Unsupported feature: Unsupported codec (4.3000)")
					}
					return nil
				}); err != nil {
				t.Fatal(err)
			}
			if heifSupport.decoder[tc.latched] != heifByFallback {
				t.Fatalf("%v = %v, want heifByFallback", tc.latched, heifSupport.decoder[tc.latched])
			}
			if heifSupport.decoder[tc.stillUntried] != heifUntried {
				t.Fatalf("%v = %v, want heifUntried: the other codec says nothing about it",
					tc.stillUntried, heifSupport.decoder[tc.stillUntried])
			}

			// the other codec still goes to vips, and ffmpeg is not used for it
			other := filepath.Join(t.TempDir(), tc.other)
			vipsCalls := 0
			if err := WithDecodeFallback(other, filepath.Join(t.TempDir(), "q.jpg"),
				func(_, _ string) error {
					t.Fatal("decoded a codec libvips reads")
					return nil
				},
				func(path string) error {
					if path == other {
						vipsCalls++
					}
					return nil
				}); err != nil {
				t.Fatal(err)
			}
			if vipsCalls != 1 {
				t.Fatalf("vips calls for %s = %d, want 1", tc.other, vipsCalls)
			}

			// while the latched codec skips vips from now on
			if err := WithDecodeFallback(missing, filepath.Join(t.TempDir(), "r.jpg"), decode,
				func(path string) error {
					if path == missing {
						t.Fatal("vips attempt after a missing-decoder latch")
					}
					return nil
				}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestFailedHEIFLoadsStaySerialized(t *testing.T) {
	defer resetHEIFSupport()
	resetHEIFSupport()
	src := filepath.Join(t.TempDir(), "a.heic")
	if err := WithDecodeFallback(src, filepath.Join(t.TempDir(), "ok.jpg"),
		func(_, _ string) error { t.Fatal("decoder used for a vips success"); return nil },
		func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if heifSupport.decoder[codecHEVC] != heifUntried {
		t.Fatalf("decoder = %v after a vips success, want heifUntried", heifSupport.decoder[codecHEVC])
	}

	var mu sync.Mutex
	current, max := 0, 0
	var wg sync.WaitGroup
	start := make(chan struct{})
	outDir := t.TempDir()
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			err := WithDecodeFallback(src, filepath.Join(outDir, "p.jpg"),
				func(_, out string) error { return writePPM(out, 4, 4) },
				func(path string) error {
					if path != src {
						return nil
					}
					mu.Lock()
					current++
					if current > max {
						max = current
					}
					mu.Unlock()
					time.Sleep(40 * time.Millisecond)
					mu.Lock()
					current--
					mu.Unlock()
					return errors.New("vips: not a known file format")
				})
			if err != nil {
				t.Error(err)
			}
		}()
	}
	close(start)
	wg.Wait()
	if max != 1 {
		t.Fatalf("concurrent vips attempts = %d, want 1", max)
	}
}

func TestWriteAtomicallyLeavesDestinationOnFailure(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "p.jpg")
	if err := ioutil.WriteFile(dst, []byte("good"), 0644); err != nil {
		t.Fatal(err)
	}
	err := WriteAtomically(dst, func(string) error {
		return errors.New("sharing violation")
	})
	if err == nil {
		t.Fatal("expected write to fail")
	}
	got, err := ioutil.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "good" {
		t.Fatalf("destination = %q, want the previous contents", got)
	}
	entries, _ := ioutil.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("temp file left behind: %v", entries)
	}

	if err := WriteAtomically(dst, func(tmp string) error {
		return ioutil.WriteFile(tmp, []byte("new"), 0644)
	}); err != nil {
		t.Fatal(err)
	}
	got, err = ioutil.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("destination = %q, want the completed write", got)
	}
	assertPublishedMode(t, dst)

	// a saver that removes the temp and recreates it 0600 must not publish that mode
	if err := WriteAtomically(dst, func(tmp string) error {
		if err := os.Remove(tmp); err != nil {
			return err
		}
		return ioutil.WriteFile(tmp, []byte("replaced"), 0600)
	}); err != nil {
		t.Fatal(err)
	}
	got, err = ioutil.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "replaced" {
		t.Fatalf("destination = %q, want the replaced write", got)
	}
	assertPublishedMode(t, dst)
}

func assertPublishedMode(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0644 {
		t.Fatalf("mode = %o, want 0644", fi.Mode().Perm())
	}
}
