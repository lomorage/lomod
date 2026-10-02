package ext

import (
	"fmt"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"bitbucket.org/lomoware/lomo-backend/common"
	"github.com/leslie-wang/filetype"
	"github.com/pkg/errors"
)

const (
	// ZIP is Apple live photo.
	ZIP = iota
	// JPG image format.
	JPG
	// JPEG image format.
	JPEG
	// PNG image format.
	PNG
	// MP4 video format.
	MP4
	// MOV video format.
	MOV
	// MPG video format.
	MPG
	// MPEG video format.
	MPEG
	// AVI video format.
	AVI
	// HEIF image format.
	HEIF
	// HEIC image format.
	HEIC
	// ThreeGP image format.
	ThreeGP
	// WebP image format.
	WebP
	// Dng image format.
	Dng
	// Bmp image format.
	Bmp
	// Tif image format.
	Tif
	// Enc is user encrypted format.
	Enc
	// Arw is ARW format.
	Arw
	// WebM video format
	WebM
	// Mkv video format
	Mkv
	// Gif video format
	Gif
	// NoXcode to disable transcoding.
	NoXcode = 100
)

const (
	// ZIPString is Apple live photo.
	ZIPString = "zip"
	// JPGString image format.
	JPGString = "jpg"
	// JPEGString image format.
	JPEGString = "jpeg"
	// PNGString image format.
	PNGString = "png"
	// MP4String video format.
	MP4String = "mp4"
	// MOVString video format.
	MOVString = "mov"
	// MPGString video format.
	MPGString = "mpg"
	// MPEGString video format.
	MPEGString = "mpeg"
	// AVIString video format.
	AVIString = "avi"
	// HEIFString image format.
	HEIFString = "heif"
	// HEICString image format.
	HEICString = "heic"
	// ThreeGPString image format.
	ThreeGPString = "3gp"
	// WebPString image format.
	WebPString = "webp"
	// DngString image format.
	DngString = "dng"
	// BmpString image format.
	BmpString = "bmp"
	// TifString image format.
	TifString  = "tif"
	TiffString = "tiff"
	// EncString image format.
	EncString = "enc"
	// ArwString image format.
	ArwString = "arw"
	// WebMString video format.
	WebMString = "webm"
	// MkvString video format.
	MkvString = "mkv"
	// GifString video format.
	GifString = "gif"
)

// GetExtString gets the extension string form by its ID.
func GetExtString(extID int) (string, error) {
	switch extID {
	case ZIP:
		return ZIPString, nil
	case JPG:
		return JPGString, nil
	case JPEG:
		return JPGString, nil
	case PNG:
		return PNGString, nil
	case MP4:
		return MP4String, nil
	case MOV:
		return MOVString, nil
	case MPG:
		return MPGString, nil
	case MPEG:
		return MPGString, nil
	case AVI:
		return AVIString, nil
	case HEIF:
		return HEICString, nil
	case HEIC:
		return HEICString, nil
	case ThreeGP:
		return ThreeGPString, nil
	case WebP:
		return WebPString, nil
	case WebM:
		return WebMString, nil
	case Dng:
		return DngString, nil
	case Bmp:
		return BmpString, nil
	case Tif:
		return TifString, nil
	case Enc:
		return EncString, nil
	case Arw:
		return ArwString, nil
	case Mkv:
		return MkvString, nil
	case Gif:
		return GifString, nil
	}
	return "", common.ErrNotImplementedFormat
}

// GetExtID gets the extension ID.
func GetExtID(ext string) (int, error) {
	switch ext {
	case ZIPString:
		return ZIP, nil
	case JPGString:
		return JPG, nil
	case JPEGString:
		return JPG, nil
	case PNGString:
		return PNG, nil
	case MP4String:
		return MP4, nil
	case MOVString:
		return MOV, nil
	case MPGString:
		return MPG, nil
	case MPEGString:
		return MPG, nil
	case AVIString:
		return AVI, nil
	case HEICString:
		return HEIF, nil
	case HEIFString:
		return HEIF, nil
	case ThreeGPString:
		return ThreeGP, nil
	case WebPString:
		return WebP, nil
	case WebMString:
		return WebM, nil
	case DngString:
		return Dng, nil
	case BmpString:
		return Bmp, nil
	case TifString:
		return Tif, nil
	case TiffString:
		return Tif, nil
	case EncString:
		return Enc, nil
	case ArwString:
		return Arw, nil
	case MkvString:
		return Mkv, nil
	case GifString:
		return Gif, nil
	}
	return -1, common.ErrNotImplementedFormat
}

// ParseAssetName parse the given asset name to asset ID and ext ID.
func ParseAssetName(name string) (int, int, error) {
	e := path.Ext(name)
	aID, err := GetAssetIDByName(name)
	if err != nil {
		return -1, -1, err
	}
	eID, err := GetExtID(strings.TrimPrefix(e, "."))
	return aID, eID, err
}

// MkAssetName creates the asset name.
func MkAssetName(assetID, ext string) string {
	return fmt.Sprintf("%s.%s", assetID, ext)
}

// MkAssetNameByID creates the asset name by its ID.
func MkAssetNameByID(n, extID int) (string, error) {
	ext, err := GetExtString(extID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d.%s", n, ext), nil
}

// GetAssetIDByName extracts assetID from name.
func GetAssetIDByName(n string) (int, error) {
	n = strings.TrimSuffix(n, filepath.Ext(n))
	return strconv.Atoi(n)
}

// MkLivePhotoImageNameJPG creates the image name for live photo.
func MkLivePhotoImageNameJPG(assetName string) string {
	return fmt.Sprintf("%s_image.jpg", assetName)
}

// MkLivePhotoImageNameHEIC creates the image name for live photo.
func MkLivePhotoImageNameHEIC(assetName string) string {
	return fmt.Sprintf("%s_image.heic", assetName)
}

// IsLivePhoto checks if the give filename is live photo.
func IsLivePhoto(assetName string) bool {
	return filepath.Ext(assetName) == "."+ZIPString
}

// IsLivePhotoExtension checks if the give extension is live photo.
func IsLivePhotoExtension(extension string) bool {
	return extension == ZIPString
}

// IsLivePhotoImage checks if the give filename is live photo.
func IsLivePhotoImage(assetName string) bool {
	assetName = strings.TrimSuffix(assetName, filepath.Ext(assetName))
	return strings.HasSuffix(assetName, "_image")
}

// TrimLivePhotoImage trims suffix of live photo image.
func TrimLivePhotoImage(assetName string) string {
	assetName = strings.TrimSuffix(assetName, filepath.Ext(assetName))
	return strings.TrimSuffix(assetName, "_image")
}

// NormalizeAssetName normalize the asset name by its time.
func NormalizeAssetName(y, m, d, assetID int) string {
	return fmt.Sprintf("%d%02d%02d_%d", y, m, d, assetID)
}

// NormalizeAssetNameString normalize the asset name by its time.
func NormalizeAssetNameString(y, m, d int, name string) string {
	return fmt.Sprintf("%d%02d%02d_%s", y, m, d, name)
}

// ParseNormalizedAssetName splits the asset name by normalized struct.
func ParseNormalizedAssetName(name string) (int, int, int, string, error) {
	parts := strings.Split(name, "_")
	if len(parts) != 2 {
		return 0, 0, 0, "", errors.Errorf("non-normalized asset name: %s", name)
	}
	dt := []byte(parts[0])
	if len(dt) != 8 { // yyyymmdd
		return 0, 0, 0, "", errors.Errorf("invalid datetime: %s", name)
	}
	y, err := strconv.Atoi(string(dt[:4]))
	if err != nil {
		return 0, 0, 0, "", err
	}
	m, err := strconv.Atoi(string(dt[4:6]))
	if err != nil {
		return 0, 0, 0, "", err
	}
	d, err := strconv.Atoi(string(dt[6:8]))
	if err != nil {
		return 0, 0, 0, "", err
	}
	return y, m, d, parts[1], nil
}

// MkPreviewVideoAssetName creates the video preview asset name with width and height.
func MkPreviewVideoAssetName(assetName string, width, height uint) string {
	return fmt.Sprintf("%s_%d_%d.mp4", assetName, width, height)
}

// MkPreviewAssetName creates the preview asset name with width and height.
func MkPreviewAssetName(assetName, ext string, width, height uint, previewVideo, previewWebp bool) string {
	switch ext {
	case MPGString:
		fallthrough
	case MPEGString:
		fallthrough
	case AVIString:
		fallthrough
	case WebMString:
		fallthrough
	case MkvString:
		fallthrough
	case MOVString:
		fallthrough
	case MP4String:
		fallthrough
	case ThreeGPString:
		if previewVideo {
			return MkPreviewVideoAssetName(assetName, width, height)
		}
		fallthrough
	case BmpString:
		fallthrough
	case TifString:
		fallthrough
	case TiffString:
		fallthrough
	case WebPString:
		fallthrough
	case DngString:
		fallthrough
	case ZIPString:
		fallthrough
	case HEIFString:
		fallthrough
	case HEICString:
		fallthrough
	case ArwString:
		fallthrough
	case GifString:
		fallthrough
	case JPEGString:
		fallthrough
	case JPGString:
		assetName = strings.TrimSuffix(assetName, "_image")
		if previewWebp {
			return fmt.Sprintf("%s_%d_%d.webp", assetName, width, height)
		}
		return fmt.Sprintf("%s_%d_%d.jpg", assetName, width, height)
	case PNGString:
		if previewWebp {
			return fmt.Sprintf("%s_%d_%d.webp", assetName, width, height)
		}
		return fmt.Sprintf("%s_%d_%d.png", assetName, width, height)
	}
	return ""
}

// ExtractAssetIDByPreviewFile extracts asset id from preview filename
func ExtractAssetIDByPreviewFile(previewFile string) (int, error) {
	_, name := filepath.Split(previewFile)
	parts := strings.Split(name, "_")
	if len(parts) < 2 {
		return 0, errors.Errorf("invalid preview filename: %s", previewFile)
	}
	id, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, errors.Wrapf(err, "invalid preview filename: %s", previewFile)
	}
	return id, nil
}

// IsMediaFile checks if the file is supported media file.
func IsMediaFile(filename string) bool {
	switch strings.ToLower(strings.TrimPrefix(filepath.Ext(filename), ".")) {
	case JPGString:
		fallthrough
	case JPEGString:
		fallthrough
	case PNGString:
		fallthrough
	case GifString:
		fallthrough
	case MP4String:
		fallthrough
	case MOVString:
		fallthrough
	case MPGString:
		fallthrough
	case MPEGString:
		fallthrough
	case AVIString:
		fallthrough
	case HEICString:
		fallthrough
	case HEIFString:
		fallthrough
	case WebPString:
		fallthrough
	case WebMString:
		fallthrough
	case DngString:
		fallthrough
	case BmpString:
		fallthrough
	case TifString:
		fallthrough
	case TiffString:
		fallthrough
	case ArwString:
		fallthrough
	case MkvString:
		fallthrough
	case ThreeGPString:
		return true
	}
	return false
}

// IsVideoFile checks if the file is video file.
func IsVideoFile(extension string) bool {
	if strings.HasPrefix(extension, ".") {
		extension = strings.TrimPrefix(extension, ".")
	}
	switch extension {
	case MP4String:
		fallthrough
	case MOVString:
		fallthrough
	case MPGString:
		fallthrough
	case MPEGString:
		fallthrough
	case AVIString:
		fallthrough
	case WebMString:
		fallthrough
	case MkvString:
		fallthrough
	case ThreeGPString:
		return true
	default:
		return false
	}
}

// IsVideoFileByID checks if the file is video file by extenstion ID.
func IsVideoFileByID(id int) bool {
	switch id {
	case MP4:
		fallthrough
	case MOV:
		fallthrough
	case MPG:
		fallthrough
	case MPEG:
		fallthrough
	case AVI:
		fallthrough
	case WebM:
		fallthrough
	case Mkv:
		fallthrough
	case ThreeGP:
		return true
	default:
		return false
	}
}

// IsImageFile checks if the file is supported image file.
func IsImageFile(filename string) bool {
	switch strings.ToLower(strings.TrimPrefix(filepath.Ext(filename), ".")) {
	case WebPString:
		fallthrough
	case DngString:
		fallthrough
	case JPGString:
		fallthrough
	case JPEGString:
		fallthrough
	case PNGString:
		fallthrough
	case BmpString:
		fallthrough
	case TifString:
		fallthrough
	case TiffString:
		fallthrough
	case HEICString:
		fallthrough
	case ArwString:
		fallthrough
	case GifString:
		fallthrough
	case HEIFString:
		return true
	}
	return false
}

// IsMediaFileMatch checks if media file matches extension or not
func IsMediaFileMatch(filename string) error {
	e := strings.ToLower(strings.TrimPrefix(filepath.Ext(filename), "."))
	// skip unsupported format firstly
	if e == DngString || e == EncString {
		return nil
	}

	typ, err := filetype.MatchFile(filename)
	if err != nil {
		return err
	}
	switch e {
	case JPEGString:
		if typ.Extension == JPGString {
			return nil
		}
	case MPEGString:
		if typ.Extension == JPGString {
			return nil
		}
	case HEICString:
		if typ.Extension == HEIFString {
			return nil
		}
	case JPGString:
		fallthrough
	case PNGString:
		fallthrough
	case MP4String:
		fallthrough
	case MOVString:
		fallthrough
	case MPGString:
		fallthrough
	case AVIString:
		fallthrough
	case HEIFString:
		fallthrough
	case ThreeGPString:
		fallthrough
	case BmpString:
		fallthrough
	case TifString:
		fallthrough
	case TiffString:
		fallthrough
	case ArwString:
		fallthrough
	case MkvString:
		fallthrough
	case GifString:
		fallthrough
	case WebMString:
		fallthrough
	case WebPString:
		if typ.Extension == e {
			return nil
		}
	default:
		return errors.Errorf("unknown media file: %s", filename)
	}
	return errors.Errorf("detected extension %s: %s", typ.Extension, filename)
}

// IsSkipPreview checks if it can skip preview generation
func IsSkipPreview(extension string) bool {
	if strings.HasPrefix(extension, ".") {
		extension = strings.TrimPrefix(extension, ".")
	}
	switch extension {
	case EncString:
		return true
	}
	return false
}

// IsSkipPreviewByID checks if it can skip preview generation
func IsSkipPreviewByID(extension int) bool {
	switch extension {
	case Enc:
		return true
	}
	return false
}

// ChromecastMIMEType returns supported MIME type.
func ChromecastMIMEType(extid int) (string, string, error) {
	switch extid {
	case Gif:
		return "image/gif", "", nil
	case Dng:
		return "image/DNG", "", nil
	case WebP:
		return "image/webp", "", nil
	case WebM:
		return "video/webm", "", nil
	case JPG:
		fallthrough
	case JPEG:
		return "image/jpeg", "", nil
	case Bmp:
		return "image/bmp", "", nil
	case Tif:
		return "image/tiff", "", nil
	case HEIF:
		fallthrough
	case HEIC:
		return "image/jpeg", "icodec=jpg", nil
	case PNG:
		return "image/png", "", nil
	case MOV:
		fallthrough
	case Mkv:
		fallthrough
	case MP4:
		return "video/mp4", "", nil
	case Arw:
		return "image/x-sony-arw", "", nil
	}
	return "", "", common.ErrNotImplementedFormat
}
