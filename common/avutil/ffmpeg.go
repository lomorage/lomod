package avutil

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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
		return cmd.ExecLowPriorityContext(ctx, f.transcodeApp, "-i", assetpath, "-y", "-threads", "1", "-r", "24", "-max_muxing_queue_size", "99999", "-vf", filter, tmpfile)
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
