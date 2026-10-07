package preview

import (
	"context"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/avutil"
	lexif "bitbucket.org/lomoware/lomo-backend/common/exif"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"github.com/leslie-wang/govips/pkg/vips"
	"github.com/pkg/errors"
	"github.com/rwcarlsen/goexif/exif"
	"github.com/sirupsen/logrus"
)

// NotifyConfig is notification configuration
type NotifyConfig struct {
	ImageWidth uint
	VideoWidth uint
	Chan       chan types.PreviewRequest
}

// Runner is the engine to do real preview generation
type Runner struct {
	ctx        context.Context
	ch         chan types.PreviewRequest
	chClosed   bool
	pauseCount int
	chLock     *sync.Mutex
	chCount    int
	exiftool   string
	folderPerm os.FileMode
	ImageDims  []types.Dimension
	VideoDims  []types.Dimension
	avengine   avutil.Engine
	notify     *NotifyConfig
	// workers is how many background generation workers Start runs (see Start). Always >= 1.
	workers int
}

// NewRunner creates new preview runner. workers controls how many background previews Start
// generates concurrently (see Start); values less than 1 are treated as 1, the original
// single-worker behavior.
func NewRunner(ctx context.Context, transcodeApp, ffprobe, exiftool string, perm os.FileMode,
	imgDims, videoDims []types.Dimension, workers int, notify *NotifyConfig) *Runner {
	if workers < 1 {
		workers = 1
	}
	runner := &Runner{ctx: ctx, folderPerm: perm,
		avengine: avutil.DefaultEngine(transcodeApp, ffprobe), exiftool: exiftool,
		ImageDims: imgDims, VideoDims: videoDims, chLock: &sync.Mutex{},
		notify: notify, workers: workers,
	}
	runner.ch = make(chan types.PreviewRequest, workers)
	return runner
}

// Pause pauses next preview generation. Safe to call from multiple concurrent
// uploads: generation only resumes once every caller has called Resume.
func (r *Runner) Pause() {
	r.chLock.Lock()
	r.pauseCount++
	r.chLock.Unlock()
}

// Resume undoes one Pause call. Generation stays paused as long as any other
// concurrent upload still holds a pause.
func (r *Runner) Resume() {
	r.chLock.Lock()
	if r.pauseCount > 0 {
		r.pauseCount--
	}
	r.chLock.Unlock()
}

func (r *Runner) isPaused() bool {
	r.chLock.Lock()
	defer r.chLock.Unlock()
	return r.pauseCount > 0
}

// Start starts r.workers background generation workers, all consuming the same request queue.
// Was a single loop for a long time; a bulk import/scan can enqueue far more preview requests
// than one worker can drain (each is a real vips decode), leaving a backlog of assets whose
// default-size previews (see Generate) aren't ready yet -- any on-demand request for one of
// those in the meantime is a genuine cache miss that has to transcode inline. A small worker
// pool shrinks that backlog faster without saturating this hardware the way an unbounded one
// would (see acquirePreviewSlot's cap on the *on-demand* side for the matching concern there).
func (r *Runner) Start() {
	for i := 0; i < r.workers; i++ {
		go r.worker()
	}
}

func (r *Runner) worker() {
	lowerWorkerThreadPriority()
	for {
		select {
		case <-r.ctx.Done():
			logrus.Infof("preview generator done: %v", r.ctx.Err())
			return
		case req, open := <-r.ch:
			if !open {
				return
			}
			r.chLock.Lock()
			r.chCount--
			r.chLock.Unlock()
			if req.GenVideo {
				// video generation takes longer time, so check pause status
				retry := 0
				for {
					retry++
					if !r.isPaused() {
						break
					}
					time.Sleep(time.Second)
					if retry%30 == 0 {
						logrus.Infof("preview runner is still paused after %d second", retry)
					}
				}
			}
			if _, err := r.GeneratePreviewByPath(r.ctx, r.folderPerm, req); err != nil {
				logrus.Warnf("while generating %s's preview, fail: %v", req.MasterPath, err)
			}
		}
	}
}

// Stop stops generator
func (r *Runner) Stop() {
	r.chLock.Lock()
	r.chClosed = true
	r.chLock.Unlock()
	close(r.ch)
}

// PendingJobCount return count of pending jobs
func (r *Runner) PendingJobCount() int {
	return r.chCount
}

func checkFile(dir, filePath string, folderPerm os.FileMode) (bool, error) {
	if fi, err := os.Stat(filePath); err == nil {
		if fi.Size() != 0 {
			return true, nil
		}
		// truncated file, remove and recreate it
		if err := os.Remove(filePath); err != nil {
			logrus.Warnf("remove truncated file %s: %s", filePath, err)
			return false, err
		}
		return false, nil
	}
	return false, os.MkdirAll(dir, folderPerm)
}

func copyPreview(srcDir, srcFilename, dstDir, dstFilename string, folderPerm os.FileMode) error {
	exist, err := checkFile(dstDir, dstFilename, folderPerm)
	if err != nil {
		return err
	} else if exist {
		return nil
	}
	exist, err = checkFile(srcDir, srcFilename, folderPerm)
	if err != nil {
		return err
	} else if !exist {
		return errors.Errorf("source file %s is not exist", srcFilename)
	}

	src, err := os.Open(srcFilename)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(dstFilename)
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = io.Copy(dst, src)
	if err != nil {
		return err
	}
	return dst.Sync()
}

// Copy copies preview from one source folder to destination preview folder
func (r *Runner) Copy(masterFilename, srcDir, srcPrefix, dstDir, dstPrefix, extension string,
	isWebp bool, folderPerm os.FileMode) error {
	requests := []types.PreviewRequest{}
	for _, dim := range r.ImageDims {
		requests = append(requests, types.PreviewRequest{
			Width:  dim.Width,
			Height: dim.Height,
		})
	}
	if ext.IsVideoFile(extension) {
		for _, dim := range r.VideoDims {
			requests = append(requests, types.PreviewRequest{
				Width:    dim.Width,
				Height:   dim.Height,
				GenVideo: true,
			})
		}
	}

	var retErr error
	for _, r := range requests {
		dst, _ := types.MkPreviewFileName(masterFilename, dstDir, dstPrefix, r.Width, r.Height, r.GenVideo,
			isWebp)
		src, _ := types.MkPreviewFileName(masterFilename, srcDir, srcPrefix, r.Width, r.Height, r.GenVideo,
			isWebp)
		if err := copyPreview(srcDir, src, dstDir, dst, folderPerm); err != nil {
			if retErr == nil {
				retErr = errors.Wrapf(err, "%s -> %s [%d,%d]", src, dst, r.Width, r.Height)
			} else {
				retErr = errors.Wrapf(retErr, "[%d,%d]: %s", r.Width, r.Height, err.Error())
			}
		}
	}
	return retErr
}

// MkPreviewRequest analysis asset and generate preview request
func (r *Runner) MkPreviewRequest(assetpath, previewdir, extension string, isWebp bool,
	tags *lexif.Tags, previewImgDims, previewVideoDims []types.Dimension) []types.PreviewRequest {
	requests := []types.PreviewRequest{}
	for _, dim := range previewImgDims {
		requests = append(requests, types.PreviewRequest{
			Width:       dim.Width,
			Height:      dim.Height,
			MasterPath:  assetpath,
			PreviewPath: previewdir,
			MasterTags:  tags,
			IsWebp:      isWebp,
		})
	}
	if !ext.IsVideoFile(extension) {
		return requests
	}
	info, err := r.avengine.ProbeVideoInfo(assetpath)
	if err != nil {
		logrus.Warnf("probe %s width height got %v", assetpath, err)
	}

	for _, dim := range previewVideoDims {
		var stream *avutil.MediaStream
		if info != nil && len(info.Streams) != 0 {
			stream = &info.Streams[0]
		}
		requests = append(requests, types.PreviewRequest{
			Width:       dim.Width,
			Height:      dim.Height,
			MasterPath:  assetpath,
			PreviewPath: previewdir,
			MasterTags:  tags,
			GenVideo:    true,
			Stream:      stream,
		})
	}
	return requests
}

// Generate analysis asset and generate preview request
func (r *Runner) Generate(assetPath, previewDir, previewPrefix, extension string, blocking, isWebp bool) {
	requests := []types.PreviewRequest{}
	for _, dim := range r.ImageDims {
		previewFile, _ := types.MkPreviewFileName(assetPath, previewDir, previewPrefix, dim.Width, dim.Height, false, isWebp)
		stat, err := os.Stat(previewFile)
		if err == nil && stat.Size() != 0 {
			// skip generation if it is exist
			continue
		}
		requests = append(requests,
			types.PreviewRequest{
				Width:         dim.Width,
				Height:        dim.Height,
				MasterPath:    assetPath,
				PreviewPath:   previewDir,
				PreviewPrefix: previewPrefix,
				IsWebp:        isWebp,
			},
		)
	}
	if !ext.IsVideoFile(extension) {
		r.generate(requests, blocking)
		return
	}

	// only generate image for video
	if len(r.VideoDims) == 0 {
		r.generate(requests, blocking)
		return
	}

	// only generate image for video
	if len(r.VideoDims) == 0 {
		r.generate(requests, blocking)
		return
	}

	var probed bool
	for _, dim := range r.VideoDims {
		previewFile, _ := types.MkPreviewFileName(assetPath, previewDir, previewPrefix, dim.Width, dim.Height, true, false)
		stat, err := os.Stat(previewFile)
		if err == nil && stat.Size() != 0 {
			// skip generation if it is exist
			continue
		}
		var stream *avutil.MediaStream
		if !probed {
			probed = true
			info, err := r.avengine.ProbeVideoInfo(assetPath)
			if err != nil {
				logrus.Warnf("probe %s width and height: %v", assetPath, err)
			} else if info != nil && len(info.Streams) != 0 {
				stream = &info.Streams[0]
			}
		}
		requests = append(requests, types.PreviewRequest{
			Width:         dim.Width,
			Height:        dim.Height,
			MasterPath:    assetPath,
			PreviewPath:   previewDir,
			PreviewPrefix: previewPrefix,
			GenVideo:      true,
			Stream:        stream,
		})
	}

	r.generate(requests, blocking)
}

func (r *Runner) generate(requests []types.PreviewRequest, blocking bool) {
	for _, req := range requests {
		if blocking {
			if _, err := r.GeneratePreviewByPath(r.ctx, r.folderPerm, req); err != nil {
				logrus.Warnf("while generating %s's preview, fail: %v", req.MasterPath, err)
			}
			continue
		}
		go func(req types.PreviewRequest) {
			defer func() {
				if recover() == nil {
					return
				}

				logrus.Warnf("channel is closed for preview request: %v", req)
			}()

			r.chLock.Lock()
			if r.chClosed {
				r.chLock.Unlock()
				logrus.Infof("preview channel closed")
				return
			}
			r.chCount++
			r.chLock.Unlock()
			r.ch <- req
		}(req)
	}
}

/*
func resetOrientation(assetpath, previewpath, exiftool string, tags *lexif.Tags) error {
	var (
		orienName  string
		orienValue int
		err        error
	)
	if tags == nil {
		logrus.Debugf("JITT create preview tag from %s", assetpath)
		tags, err = lexif.NewTags(assetpath, exiftool, "")
		if err != nil {
			return errors.Wrapf(err, "Use %s extract EXIF tag from %s", exiftool, assetpath)
		}
	}

	if tags != nil {
		orienName, orienValue = tags.GetOrientation()
	}
	if orienName == "" {
		return nil
	}
	return lexif.SetOrientation(previewpath, exiftool, orienName, orienValue)
}
*/
func (r *Runner) generateDefaultPreviewJPG(assetpath, previewpath string, tags *lexif.Tags) error {
	file, err := os.Open(assetpath)
	if err != nil {
		return err
	}
	defer file.Close()

	// try to read thumbnail from EXIF. if not exist, then resize by ourselves
	x, err := exif.Decode(file)
	if err != nil {
		logrus.Warnf("decode EXIF from %s: %v", assetpath, err)
		return err
	}

	buf, err := x.JpegThumbnail()
	if err != nil || len(buf) == 0 {
		logrus.Warnf("extract EXIF thumbnail from %s: %v", assetpath, err)
		return err
	}
	return ioutil.WriteFile(previewpath, buf, 0644)
}

// vips will preserve Orientation metadata, so only set orientation if taking it from exif thumbnail.
func (r *Runner) generatePreviewJPG(assetpath, previewpath string, width, height uint, tags *lexif.Tags) error {
	if width == 0 {
		err := r.generateDefaultPreviewJPG(assetpath, previewpath, tags)
		if err != nil {
			// probably not able to find thumbnail in exif tag, and use vips to recreate
			logrus.Warnf("extract EXIF thumbnail from %s: %v", assetpath, err)
			return r.generatePreviewJPGByResize(assetpath, previewpath, common.DefaultPreviewWidth, 0)
		}
		logrus.Debugf("Success extract EXIF thumbnail from %s, and create %s", assetpath, previewpath)
		return nil
	}
	return r.generatePreviewJPGByResize(assetpath, previewpath, width, height)

	/* vips has auto rotate, so remove it for now
	if err := resetOrientation(assetpath, previewpath, exiftool, tags); err != nil {
		logrus.Warnf("reset orientation: %s", err)
	}
	*/
	// GPS should be removed already, comment out to avoid chinese support by exiftool
	// error: `Wildcards don't work in the directory specification`
	// return lexif.RemoveGPS(previewpath, r.exiftool)
}

func vipsHeightOptions(height uint) []*vips.Option {
	options := []*vips.Option{}
	if height != 0 {
		options = append(options,
			vips.InputInt("height", int(height)),
			vips.InputString("size", "force"),
		)
	}
	return options
}

// NeedsDecodeFallback reports whether a file vips failed to read should be retried through an
// external decoder: libvips' Windows builds ship libheif with only the AV1 decoder (no HEVC,
// for patent reasons), so iPhone HEIC photos can't be decoded by vips there.
func NeedsDecodeFallback(path string) bool {
	switch strings.ToLower(strings.TrimPrefix(filepath.Ext(path), ".")) {
	case ext.HEICString, ext.HEIFString:
		return true
	}
	return false
}

// FirstLine returns err's first line; govips appends a goroutine stack to its errors.
func FirstLine(err error) string {
	return strings.SplitN(err.Error(), "\n", 2)[0]
}

// DecodeToTemp decodes src with decode into an uncompressed temp file next to dst, for vips to
// read instead of src. The caller must call cleanup once vips is done with the image.
func DecodeToTemp(decode func(src, dst string) error, src, dst string) (string, func(), error) {
	// unique per call: a background worker and an on-demand request can decode the same
	// asset for the same preview at once
	dir, file := filepath.Split(dst)
	f, err := ioutil.TempFile(dir, "."+file+".decoded.*.ppm")
	if err != nil {
		return "", nil, err
	}
	tmp := f.Name()
	f.Close()
	cleanup := func() {
		if err := os.Remove(tmp); err != nil && !os.IsNotExist(err) {
			logrus.Warnf("remove decoded temp file %s: %v", tmp, err)
		}
	}
	if err := decode(src, tmp); err != nil {
		cleanup()
		return "", nil, errors.Wrapf(err, "decode %s", src)
	}
	return tmp, cleanup, nil
}

type heifDecoder int

const (
	heifUntried heifDecoder = iota
	heifByVips
	heifByFallback
)

// heifSupport remembers whether this libvips build decodes HEIF. Until the first HEIF file
// settles it, vips attempts are serialized: concurrent failed loads of one file leak its handle
// in libvips on Windows, which then can't be moved or deleted until lomod restarts.
var heifSupport struct {
	sync.Mutex
	decoder heifDecoder
}

// WithDecodeFallback runs attempt on src, and for HEIF files vips can't decode (see
// NeedsDecodeFallback) runs it again on a copy decoded by decode, which may be nil.
// attempt must do all the vips work (load and save): vips reads HEIF lazily, so a missing
// decoder may only show up when saving.
func WithDecodeFallback(src, dst string, decode func(src, dst string) error, attempt func(path string) error) error {
	if decode == nil || !NeedsDecodeFallback(src) {
		return attempt(src)
	}

	heifSupport.Lock()
	decoder := heifSupport.decoder
	untried := decoder == heifUntried
	if untried {
		defer heifSupport.Unlock()
	} else {
		heifSupport.Unlock()
	}

	var err error
	if decoder == heifByFallback {
		err = errors.New("libvips can't decode HEIF here")
	} else {
		if err = attempt(src); err == nil {
			if untried {
				heifSupport.decoder = heifByVips
			}
			return nil
		}
		logrus.Infof("vips can't read %s (%s), decoding it with ffmpeg", src, FirstLine(err))
	}

	decoded, cleanup, derr := DecodeToTemp(decode, src, dst)
	if derr != nil {
		return errors.Wrapf(err, "fallback: %v", derr)
	}
	defer cleanup()
	if err := attempt(decoded); err != nil {
		return err
	}
	if untried {
		heifSupport.decoder = heifByFallback
		logrus.Warnf("libvips can't decode HEIF (%s), using ffmpeg for HEIC/HEIF from now on", src)
	}
	return nil
}

// DecodeImage decodes a still image vips can't read into outpath -- see NeedsDecodeFallback.
func (r *Runner) DecodeImage(assetpath, outpath string) error {
	return r.avengine.DecodeImage(assetpath, outpath)
}

func (r *Runner) generatePreviewWebp(assetpath, previewpath string, width, height uint) error {
	return r.generateThumbnail(assetpath, previewpath, width, height, ext.WebPString)
}

func (r *Runner) generatePreviewJPGByResize(assetpath, previewpath string, width, height uint) error {
	return r.generateThumbnail(assetpath, previewpath, width, height, ext.JPGString)
}

func (r *Runner) generatePreviewPNG(assetpath, previewpath string, width, height uint) error {
	return r.generateThumbnail(assetpath, previewpath, width, height, ext.PNGString)
}

func (r *Runner) generateThumbnail(assetpath, previewpath string, width, height uint, format string) error {
	return WithDecodeFallback(assetpath, previewpath, r.DecodeImage, func(src string) error {
		return thumbnailTo(src, previewpath, width, height, format)
	})
}

func thumbnailTo(src, previewpath string, width, height uint, format string) error {
	outImage, err := vips.Thumbnail(src, int(width), vipsHeightOptions(height)...)
	if err != nil {
		if outImage != nil {
			vips.FreeImage(outImage)
		}
		return errors.Wrapf(err, "while generating %s thumbnail", format)
	}
	defer vips.FreeImage(outImage)

	if format != ext.PNGString {
		vips.RemoveImageMetadata(outImage, "jpeg-thumbnail-data")
		vips.RemoveImageMetadata(outImage, "exif-data")
	}
	vips.RemoveImageMetadata(outImage, "xmp-data")
	vips.RemoveImageMetadata(outImage, "iptc-data")
	vips.RemoveImageMetadata(outImage, "icc-profile-data")

	switch format {
	case ext.JPGString:
		err = vips.Jpegsave(outImage, previewpath)
	case ext.PNGString:
		err = vips.Pngsave(outImage, previewpath)
	default:
		err = vips.Webpsave(outImage, previewpath)
	}
	if err != nil {
		// a failed save can leave a partial file that checkFile would later serve as cached
		os.Remove(previewpath)
		return errors.Wrapf(err, "while saving %s preview", format)
	}
	return nil
}

// GeneratePreviewByPath generates preview for asset
func (r *Runner) GeneratePreviewByPath(ctx context.Context, folderPerm os.FileMode,
	req types.PreviewRequest) (string, error) {
	previewFile, e := req.MkPreviewFileName()

	exist, err := checkFile(req.PreviewPath, previewFile, folderPerm)
	if err != nil {
		return "", err
	} else if exist {
		return previewFile, nil
	}

	err = r.generatePreview(ctx, req.MasterPath, previewFile, e,
		req.Width, req.Height, req.GenVideo, req.Stream, req.MasterTags)
	if err != nil {
		return "", err
	}
	go func() {
		if r.notify == nil {
			return
		}
		if req.IsWebp && req.Width == r.notify.ImageWidth {
			r.notify.Chan <- req
		}
	}()
	return previewFile, nil
}

func (r *Runner) generatePreview(ctx context.Context, assetpath, previewFile, extension string,
	width, height uint, videoPreview bool, stream *avutil.MediaStream, tags *lexif.Tags) error {
	logrus.Infof("generate %s's preview file %s", assetpath, previewFile)

	defer func() {
		if err := recover(); err != nil {
			logrus.Warnf("panic occurred: %s", err)
		}
	}()

	if fi, err := os.Stat(assetpath); err != nil {
		return err
	} else if fi.Size() == 0 {
		return errors.Errorf("%s size 0", assetpath)
	}

	// only take context for video preview generation
	if ext.IsVideoFile(extension) {
		if videoPreview {
			return r.avengine.XcodeVideoToVideo(ctx, assetpath, previewFile, width, height, stream)
		}
		return r.avengine.XcodeVideoToImage(assetpath, previewFile, width, height, stream)
	}
	switch strings.TrimPrefix(filepath.Ext(previewFile), ".") {
	case ext.PNGString:
		return r.generatePreviewPNG(assetpath, previewFile, width, height)
	case ext.JPGString:
		return r.generatePreviewJPG(assetpath, previewFile, width, height, tags)
	default:
		return r.generatePreviewWebp(assetpath, previewFile, width, height)
	}
}
