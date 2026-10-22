package avutil

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/cmd"
	"github.com/sirupsen/logrus"
)

// DefaultEngine create defaults transcode engine
func DefaultEngine(transcodeApp, probeApp string) Engine {
	return &ffmpeg{transcodeApp: transcodeApp, probeApp: probeApp}
}

type ffmpeg struct {
	transcodeApp string
	probeApp     string

	videoCodecMu      sync.Mutex
	videoCodecKnown   bool
	videoCodecArgs    []string
	videoCodecRetryAt time.Time
}

// videoCodecRetry: how long to wait before asking ffmpeg for its encoders again after that failed.
const videoCodecRetry = time.Minute

// videoCodec returns the encoder arguments for an H.264 video preview, the codec every browser
// and phone plays. ffmpeg only picks H.264 for .mp4 on its own when it's built with libx264
// (GPL); builds without it, like the Windows bundle, fall back to MPEG-4 Part 2, which browsers
// can't play -- so ask such a build for OpenH264 instead.
// Only a successful answer is kept: if ffmpeg couldn't be asked (e.g. mid-update), this preview
// uses ffmpeg's default and the question is asked again a minute later.
func (f *ffmpeg) videoCodec() []string {
	f.videoCodecMu.Lock()
	defer f.videoCodecMu.Unlock()
	if f.videoCodecKnown || time.Now().Before(f.videoCodecRetryAt) {
		return f.videoCodecArgs
	}
	out, err := cmd.Run(f.transcodeApp, "-hide_banner", "-encoders")
	if err != nil {
		logrus.Warnf("list %s encoders: %v", f.transcodeApp, err)
		f.videoCodecRetryAt = time.Now().Add(videoCodecRetry)
		return nil
	}
	f.videoCodecArgs = h264EncoderArgs(string(out))
	f.videoCodecKnown = true
	logrus.Infof("video preview encoder args: %v", f.videoCodecArgs)
	return f.videoCodecArgs
}

func h264EncoderArgs(encoders string) []string {
	has := map[string]bool{}
	for _, line := range strings.Split(encoders, "\n") {
		if fields := strings.Fields(line); len(fields) >= 2 && strings.HasPrefix(fields[0], "V") {
			has[fields[1]] = true
		}
	}
	switch {
	case has["libx264"]:
		return nil // ffmpeg's default for .mp4
	case has["libopenh264"]:
		return []string{"-c:v", "libopenh264", "-pix_fmt", "yuv420p", "-b:v", "1200k"}
	}
	return nil
}

// ProbeVideoInfo return video infromation
func (f *ffmpeg) ProbeVideoInfo(filename string) (*VideoProbeInfo, error) {
	var (
		info    VideoProbeInfo
		content []byte
		err     error
	)
	if err := common.RetryIfKnownError(fmt.Sprintf("while probe video %s", filename), func() error {
		content, err = cmd.Run(f.probeApp, "-v", "quiet", "-select_streams", "v:0", "-show_entries", "stream=width,height:stream_tags=rotate", "-of", "json", filename)
		return err
	}); err != nil {
		return nil, err
	}

	err = json.Unmarshal(content, &info)
	return &info, err

}

// XcodeVideoToVideo xcode video for video
func (f *ffmpeg) XcodeVideoToVideo(ctx context.Context, assetpath, previewpath string, width, height uint, stream *MediaStream) error {
	// generate to one temp file, then move to final one if it is success
	imgWidth, imgHeight, rotate := checkRotate(width, height, stream)
	filter := fmt.Sprintf("scale=%d:%d", imgWidth, imgHeight)
	if rotate != "" {
		filter += ", " + rotate
	}

	dir, file := filepath.Split(previewpath)
	tmpfile := filepath.Join(dir, "."+file)
	defer os.RemoveAll(tmpfile)
	logrus.Infof("generate video preview %s - %s", previewpath, filter)
	if err := common.RetryIfKnownError(fmt.Sprintf("while generating %s for video %s with size %dx%d", filter, previewpath, imgWidth, imgHeight), func() error {
		args := []string{"-i", assetpath, "-y", "-threads", "1", "-r", "24", "-max_muxing_queue_size", "99999", "-vf", filter}
		args = append(args, f.videoCodec()...)
		// Move moov ahead of mdat so a player knows the duration from the first bytes, rather
		// than range-requesting the tail first -- or failing outright where it cannot range
		// request at all. The preview is always .mp4 (ext.MkPreviewVideoAssetName), so the mp4
		// muxer always accepts this. The cost is a second pass over the finished file, i.e. the
		// preview is written roughly twice; at 480p that is cheap next to a round trip per play.
		args = append(args, "-movflags", "+faststart")
		return cmd.ExecLowPriorityContext(ctx, f.transcodeApp, append(args, tmpfile)...)
	}); err != nil {
		return err
	}

	return os.Rename(tmpfile, previewpath)
}

// XcodeVideoToVideo create image preview for video
func (f *ffmpeg) XcodeVideoToImage(assetpath, previewpath string, width, height uint, stream *MediaStream) error {
	imgWidth, imgHeight, rotate := checkRotate(width, height, stream)
	filter := fmt.Sprintf("scale=%d:%d", imgWidth, imgHeight)
	if rotate != "" {
		filter += ", " + rotate
	}
	logrus.Infof("generate video's image preview %s - %s", previewpath, filter)
	cmds := []string{"-i", assetpath, "-y", "-an", "-vframes", "1", "-vf", filter}
	cmds = append(cmds, previewpath)

	return common.RetryIfKnownError(fmt.Sprintf("while generating %s for video %s with size %dx%d", filter, previewpath, imgWidth, imgHeight), func() error {
		return cmd.ExecLowPriority(f.transcodeApp, cmds...)
	})
}

// DecodeImage decodes the primary image of assetpath into outpath. ffmpeg (7.1+) reads HEIF
// tile grids and applies irot/imir, so the result needs no further orientation handling.
func (f *ffmpeg) DecodeImage(assetpath, outpath string) error {
	if f.transcodeApp == "" {
		return fmt.Errorf("no ffmpeg to decode %s", assetpath)
	}
	return cmd.ExecLowPriority(f.transcodeApp, "-v", "error", "-i", assetpath, "-frames:v", "1", "-y", outpath)
}
