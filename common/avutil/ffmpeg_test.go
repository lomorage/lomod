package avutil

import (
	"reflect"
	"testing"
)

func TestH264EncoderArgs(t *testing.T) {
	const header = "Encoders:\n V..... = Video\n ------\n"
	for name, c := range map[string]struct {
		encoders string
		want     []string
	}{
		"libx264 is ffmpeg's own pick": {header + " V....D libx264              libx264 H.264\n V....D libopenh264 OpenH264\n", nil},
		"OpenH264 only (Windows bundle)": {header + " V....D libopenh264          OpenH264 H.264\n V.S..D mpeg4                MPEG-4 part 2\n",
			[]string{"-c:v", "libopenh264", "-pix_fmt", "yuv420p", "-b:v", "1200k"}},
		"neither: leave it to ffmpeg": {header + " V.S..D mpeg4                MPEG-4 part 2\n A....D aac AAC\n", nil},
		"an audio encoder of that name doesn't count": {header + " A....D libopenh264 not really\n", nil},
	} {
		if got := h264EncoderArgs(c.encoders); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v, want %v", name, got, c.want)
		}
	}
}
