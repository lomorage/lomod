package avutil

import (
	"context"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These exercise the real ffmpeg binary against real files: the point of the faststart change
// is what ends up in the generated .mp4, which no amount of argument-list assertion can show.

func lookupFFmpeg(t *testing.T) (ffmpeg, ffprobe string) {
	t.Helper()
	var err error
	if ffmpeg, err = exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg on PATH")
	}
	if ffprobe, err = exec.LookPath("ffprobe"); err != nil {
		t.Skip("no ffprobe on PATH")
	}
	return ffmpeg, ffprobe
}

// mkSourceVideo synthesises a source video with the given encoder, so the test doesn't depend
// on any checked-in or external media.
func mkSourceVideo(t *testing.T, ffmpegBin, dir, name, encoder string) string {
	t.Helper()
	if out, err := exec.Command(ffmpegBin, "-hide_banner", "-encoders").Output(); err != nil {
		t.Skipf("cannot list %s encoders: %v", ffmpegBin, err)
	} else if !strings.Contains(string(out), encoder) {
		t.Skipf("this ffmpeg has no %s encoder", encoder)
	}

	path := filepath.Join(dir, name)
	// A couple of seconds of colour bars plus a tone, big enough that moov placement matters.
	cmd := exec.Command(ffmpegBin, "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=size=640x360:rate=25:duration=2",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=2",
		"-c:v", encoder, "-c:a", "aac", "-pix_fmt", "yuv420p", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate %s source: %v\n%s", encoder, err, out)
	}
	return path
}

// topLevelAtoms lists an MP4's top-level boxes in the order they appear on disk.
func topLevelAtoms(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	size := fi.Size()

	var atoms []string
	for off := int64(0); off < size; {
		header := make([]byte, 8)
		if _, err := f.ReadAt(header, off); err != nil {
			break
		}
		boxLen := int64(binary.BigEndian.Uint32(header[:4]))
		atoms = append(atoms, string(header[4:8]))
		switch boxLen {
		case 0:
			// box extends to end of file
			off = size
		case 1:
			extSize := make([]byte, 8)
			if _, err := f.ReadAt(extSize, off+8); err != nil {
				t.Fatalf("read 64-bit size in %s: %v", path, err)
			}
			// 8-byte header + 8-byte extended size: anything smaller cannot describe a box,
			// and a value below 8 would walk the offset backwards.
			if boxLen = int64(binary.BigEndian.Uint64(extSize)); boxLen < 16 {
				t.Fatalf("bad 64-bit box length %d in %s", boxLen, path)
			}
			off += boxLen
		default:
			if boxLen < 8 {
				t.Fatalf("bad box length %d in %s", boxLen, path)
			}
			off += boxLen
		}
	}
	return atoms
}

func indexOfAtom(atoms []string, want string) int {
	for i, a := range atoms {
		if a == want {
			return i
		}
	}
	return -1
}

func videoCodecOf(t *testing.T, ffprobeBin, path string) string {
	t.Helper()
	out, err := exec.Command(ffprobeBin, "-v", "error",
		"-select_streams", "v:0", "-show_entries", "stream=codec_name",
		"-of", "default=noprint_wrappers=1:nokey=1", path).Output()
	if err != nil {
		t.Fatalf("ffprobe %s: %v", path, err)
	}
	return strings.TrimSpace(string(out))
}

// A generated video preview must be streamable: moov ahead of mdat, so a browser knows the
// duration from the first bytes instead of range-requesting the tail (or failing outright when
// it cannot range-request at all).
func TestXcodeVideoToVideoPutsMoovBeforeMdat(t *testing.T) {
	ffmpegBin, ffprobeBin := lookupFFmpeg(t)
	dir := t.TempDir()
	src := mkSourceVideo(t, ffmpegBin, dir, "src.mp4", "libx264")
	preview := filepath.Join(dir, "src_480_0.mp4")

	engine := DefaultEngine(ffmpegBin, ffprobeBin)
	if err := engine.XcodeVideoToVideo(context.Background(), src, preview, 480, 0, nil); err != nil {
		t.Fatalf("XcodeVideoToVideo: %v", err)
	}

	atoms := topLevelAtoms(t, preview)
	moov, mdat := indexOfAtom(atoms, "moov"), indexOfAtom(atoms, "mdat")
	if moov == -1 || mdat == -1 {
		t.Fatalf("preview is not a usable mp4, atoms: %v", atoms)
	}
	if moov > mdat {
		t.Errorf("moov comes after mdat (atoms: %v); -movflags +faststart did not take effect", atoms)
	}

	// The preview is only useful if it's also the codec browsers play.
	if codec := videoCodecOf(t, ffprobeBin, preview); codec != "h264" {
		t.Errorf("preview video codec is %q, want h264", codec)
	}

	// The temp file XcodeVideoToVideo transcodes into must not be left behind.
	if _, err := os.Stat(filepath.Join(dir, ".src_480_0.mp4")); !os.IsNotExist(err) {
		t.Errorf("temp transcode file was left behind (stat err: %v)", err)
	}
}

// The case that sends a real user to the fallback player: an HEVC original, which many browsers
// cannot decode. The preview must come out H.264 and faststart, or the fallback has nothing
// playable to switch to.
func TestXcodeVideoToVideoFromHEVCSource(t *testing.T) {
	ffmpegBin, ffprobeBin := lookupFFmpeg(t)
	dir := t.TempDir()
	src := mkSourceVideo(t, ffmpegBin, dir, "src_hevc.mp4", "libx265")
	if codec := videoCodecOf(t, ffprobeBin, src); codec != "hevc" {
		t.Fatalf("source is %q, want hevc", codec)
	}
	preview := filepath.Join(dir, "src_hevc_480_0.mp4")

	engine := DefaultEngine(ffmpegBin, ffprobeBin)
	if err := engine.XcodeVideoToVideo(context.Background(), src, preview, 480, 0, nil); err != nil {
		t.Fatalf("XcodeVideoToVideo from hevc: %v", err)
	}

	if codec := videoCodecOf(t, ffprobeBin, preview); codec != "h264" {
		t.Errorf("preview video codec is %q, want h264", codec)
	}
	atoms := topLevelAtoms(t, preview)
	if moov, mdat := indexOfAtom(atoms, "moov"), indexOfAtom(atoms, "mdat"); moov == -1 || moov > mdat {
		t.Errorf("hevc-sourced preview is not faststart, atoms: %v", atoms)
	}
}
