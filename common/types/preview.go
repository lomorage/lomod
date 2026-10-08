package types

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"bitbucket.org/lomoware/lomo-backend/common/avutil"
	"bitbucket.org/lomoware/lomo-backend/common/exif"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
)

// PreviewRequest is the data structure for preview generation
type PreviewRequest struct {
	Width         uint
	Height        uint
	MasterPath    string
	PreviewPath   string
	PreviewPrefix string
	GenVideo      bool
	IsWebp        bool
	Stream        *avutil.MediaStream `json:",omitempty"`
	MasterTags    *exif.Tags          `json:",omitempty"`
}

// MkPreviewFileName makes preview file name
func (req PreviewRequest) MkPreviewFileName() (string, string) {
	return MkPreviewFileName(req.MasterPath, req.PreviewPath, req.PreviewPrefix, req.Width, req.Height,
		req.GenVideo, req.IsWebp)
}

// MkPreviewFileName makes preview file name
func MkPreviewFileName(masterPath, previewPath, previewPrefix string, width, height uint,
	genVideo, isWebp bool) (string, string) {
	_, name := filepath.Split(masterPath)
	e := filepath.Ext(name)
	prefix := previewPrefix
	if prefix == "" {
		prefix = strings.TrimSuffix(name, e)
	}
	e = strings.ToLower(strings.TrimPrefix(e, "."))
	return filepath.Join(previewPath, ext.MkPreviewAssetName(prefix, e, width, height, genVideo, isWebp)), e
}

// PreviewRunner is preview runner interface so as to avoid pi zero compile failure
type PreviewRunner interface {
	Start()
	Stop()
	Pause()
	Resume()
	PendingJobCount() int
	Copy(masterFilename, srcDir, srcPrefix, dstDir, dstPrefix, extension string, isWebp bool, m os.FileMode) error
	MkPreviewRequest(assetpath, previewdir, extension string, isWebp bool,
		tags *exif.Tags, previewImgDims, previewVideoDims []Dimension) []PreviewRequest
	Generate(assetPath, previewDir, previewPrefix, extension string, blocking, isWebp bool)
	GeneratePreviewByPath(ctx context.Context, folderPerm os.FileMode, req PreviewRequest) (string, error)
	DecodeImage(assetpath, outpath string) error
}
