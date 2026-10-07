package avutil

import "context"

const (
	// Transpose0 Rotate by 90 degrees counter-clockwise and flip vertically
	Transpose0 = "transpose=0"
	// Transpose12 Rotate by 90 degrees clockwise.
	Transpose12 = "transpose=1,transpose=2"
	// Transpose2 Rotate by 90 degrees counter-clockwise.
	Transpose2 = "transpose=2"
	// Transpose3 Rotate by 90 degrees clockwise and flip vertically.
	Transpose3 = "transpose=3"
	// Transpose22 Rotate by 180
	Transpose22 = "transpose=2,transpose=2"
)

// MediaTags is the structure of each media asset
type MediaTags struct {
	Rotate string `json:"rotate"`
}

// MediaStream is the structure of each media asset
type MediaStream struct {
	Width  uint      `json:"width"`
	Height uint      `json:"height"`
	Tags   MediaTags `json:"tags"`
}

// VideoProbeInfo is the structure to probe video aspect ratio
type VideoProbeInfo struct {
	Streams []MediaStream `json:"streams"`
}

// Engine is interface for av processing
type Engine interface {
	ProbeVideoInfo(filename string) (*VideoProbeInfo, error)
	XcodeVideoToVideo(ctx context.Context, assetpath, previewpath string, width, height uint, stream *MediaStream) error
	XcodeVideoToImage(assetpath, previewpath string, width, height uint, stream *MediaStream) error
	// DecodeImage decodes a still image into an uncompressed file (format from outpath's
	// extension) with HEIF rotation and tiling applied, for formats vips cannot read itself.
	DecodeImage(assetpath, outpath string) error
}

func checkRotate(width, height uint, stream *MediaStream) (int, int, string) {
	imgWidth := -2
	imgHeight := -2
	if width != 0 {
		imgWidth = int(width)
	}
	if height != 0 {
		imgHeight = int(height)
	}

	if stream != nil {
		// for 90, 270 rotation, if width is larger than height, need change w and h
		// for 180, and others, if width is smaller than height, change w and h
		switch stream.Tags.Rotate {
		case "90":
			if stream.Width > stream.Height {
				return imgHeight, imgWidth, Transpose12
			}
			return imgWidth, imgHeight, Transpose12
		case "180":
			if stream.Width < stream.Height {
				return imgHeight, imgWidth, Transpose12
			}
			return imgWidth, imgHeight, Transpose12
		case "270":
			if stream.Width > stream.Height {
				return imgHeight, imgWidth, Transpose12
			}
			return imgWidth, imgHeight, Transpose12
		default:
			if stream.Width < stream.Height {
				return imgHeight, imgWidth, ""
			}
		}
	}
	return imgWidth, imgHeight, ""
}
