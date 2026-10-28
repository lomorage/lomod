package ext

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"bitbucket.org/lomoware/lomo-backend/common"
)

// The IDs are stored in the asset table, so they must never be renumbered: a new format
// is appended, never inserted. This pins the IDs a database already holds.
func TestExtIDsAreStable(t *testing.T) {
	for id, want := range map[int]string{
		ZIP: "zip", JPG: "jpg", PNG: "png", MP4: "mp4", MOV: "mov", MPG: "mpg",
		AVI: "avi", HEIF: "heic", HEIC: "heic", ThreeGP: "3gp", WebP: "webp",
		Dng: "dng", Bmp: "bmp", Tif: "tif", Enc: "enc", Arw: "arw", WebM: "webm",
		Mkv: "mkv", Gif: "gif", Avif: "avif",
	} {
		got, err := GetExtString(id)
		if err != nil {
			t.Errorf("GetExtString(%d): %v", id, err)
			continue
		}
		if got != want {
			t.Errorf("GetExtString(%d) = %q, want %q", id, got, want)
		}
	}
	if Avif != 21 {
		t.Errorf("Avif = %d, want 21: appending keeps the IDs already in the database", Avif)
	}
	if ZIP != 0 || Gif != 20 {
		t.Errorf("ZIP = %d, Gif = %d: an ID was inserted rather than appended", ZIP, Gif)
	}
}

func TestAvifExtID(t *testing.T) {
	id, err := GetExtID(AvifString)
	if err != nil {
		t.Fatalf("GetExtID(%q): %v", AvifString, err)
	}
	if id != Avif {
		t.Fatalf("GetExtID(%q) = %d, want %d", AvifString, id, Avif)
	}
	if _, err := GetExtID("jxl"); err != common.ErrNotImplementedFormat {
		t.Fatalf("GetExtID(\"jxl\") error = %v, want ErrNotImplementedFormat", err)
	}
}

func TestAvifIsASupportedStillImage(t *testing.T) {
	if !IsImageFile("IMG_0001.avif") {
		t.Error("IsImageFile(avif) = false, want true")
	}
	if !IsMediaFile("IMG_0001.avif") {
		t.Error("IsMediaFile(avif) = false, want true")
	}
	if !IsMediaFile("IMG_0001.AVIF") {
		t.Error("IsMediaFile is case sensitive for avif")
	}
	if IsVideoFile(AvifString) {
		t.Error("IsVideoFile(avif) = true, want false")
	}
	if IsVideoFileByID(Avif) {
		t.Error("IsVideoFileByID(Avif) = true, want false")
	}
	if IsSkipPreview(AvifString) {
		t.Error("IsSkipPreview(avif) = true: avif previews are generated")
	}
}

func TestAvifPreviewName(t *testing.T) {
	// an avif preview is a webp or a jpg, like every other still format
	if got := MkPreviewAssetName("20260918_42", AvifString, 320, 0, false, true); got != "20260918_42_320_0.webp" {
		t.Errorf("webp preview name = %q", got)
	}
	if got := MkPreviewAssetName("20260918_42", AvifString, 320, 0, false, false); got != "20260918_42_320_0.jpg" {
		t.Errorf("jpg preview name = %q", got)
	}
}

// Chromecast has no AVIF support, so it must be offered the jpg transcode, as HEIC is.
func TestAvifChromecastMIMEType(t *testing.T) {
	mime, codec, err := ChromecastMIMEType(Avif)
	if err != nil {
		t.Fatal(err)
	}
	if mime != "image/jpeg" || codec != "icodec=jpg" {
		t.Fatalf("ChromecastMIMEType(Avif) = %q, %q; want image/jpeg, icodec=jpg", mime, codec)
	}
}

func TestIsMediaFileMatchAvif(t *testing.T) {
	sample, err := ioutil.ReadFile(filepath.Join("testdata", "sample.avif"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()

	good := filepath.Join(dir, "good.avif")
	if err := ioutil.WriteFile(good, sample, 0644); err != nil {
		t.Fatal(err)
	}
	if err := IsMediaFileMatch(good); err != nil {
		t.Errorf("IsMediaFileMatch(%s) = %v, want nil", good, err)
	}

	// the extension lies: a jpeg renamed .avif must be reported, as for every other format
	bad := filepath.Join(dir, "bad.avif")
	if err := ioutil.WriteFile(bad, []byte("\xff\xd8\xff\xe0 not an avif at all"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := IsMediaFileMatch(bad); err == nil {
		t.Errorf("IsMediaFileMatch(%s) = nil, want an error", bad)
	}

	// a HEIC renamed .avif: ISO-BMFF, but not an AVIF brand
	heic := filepath.Join(dir, "heic.avif")
	src, err := ioutil.ReadFile(filepath.Join("..", "..", "cmd", "lomod", "test", "img", "12_2014_01_21.heic"))
	if err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(heic, src, 0644); err != nil {
		t.Fatal(err)
	}
	if err := IsMediaFileMatch(heic); err == nil {
		t.Errorf("IsMediaFileMatch(%s) = nil, want an error for a non-AVIF brand", heic)
	}

	// an encoder that lists many compatible brands makes ftyp longer than a minimal one;
	// the whole box must still be read, or a valid AVIF would be rejected
	brands := []string{"avif", "mif1", "miaf", "MA1B", "mia1", "miaB", "msf1", "heic",
		"heix", "avis", "av01", "iso8", "iso9", "isom", "mp41", "mp42"}
	box := make([]byte, 0, 16+4*len(brands))
	size := 16 + 4*len(brands)
	box = append(box, byte(size>>24), byte(size>>16), byte(size>>8), byte(size))
	box = append(box, []byte("ftyp")...)
	box = append(box, []byte("mif1")...) // major brand is not avif here
	box = append(box, 0, 0, 0, 0)        // minor version
	for _, b := range brands {
		box = append(box, []byte(b)...)
	}
	if len(box) <= 64 {
		t.Fatalf("ftyp box is %d bytes, too short to exercise the long-header case", len(box))
	}
	long := filepath.Join(dir, "long-ftyp.avif")
	if err := ioutil.WriteFile(long, append(box, make([]byte, 64)...), 0644); err != nil {
		t.Fatal(err)
	}
	if err := IsMediaFileMatch(long); err != nil {
		t.Errorf("IsMediaFileMatch(%s) = %v, want nil for an avif compatible brand past byte 64", long, err)
	}

	// a truncated file is rejected, not treated as AVIF
	short := filepath.Join(dir, "short.avif")
	if err := ioutil.WriteFile(short, sample[:8], 0644); err != nil {
		t.Fatal(err)
	}
	if err := IsMediaFileMatch(short); err == nil {
		t.Errorf("IsMediaFileMatch(%s) = nil, want an error", short)
	}

	if err := IsMediaFileMatch(filepath.Join(dir, "missing.avif")); !os.IsNotExist(errorsCause(err)) {
		t.Errorf("IsMediaFileMatch(missing) = %v, want a not-exist error", err)
	}
}

// errorsCause unwraps the os error IsMediaFileMatch returns for an unreadable file.
func errorsCause(err error) error {
	for {
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return err
		}
		err = u.Unwrap()
	}
}
