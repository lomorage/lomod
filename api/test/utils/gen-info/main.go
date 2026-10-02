package main

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	atlomod "bitbucket.org/lomoware/lomo-backend/api/test/lomod"
	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/asset"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/preview"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"github.com/leslie-wang/govips/pkg/vips"
	"github.com/pkg/errors"
)

const (
	previewRoot  = "/media/home/alice/Photos/preview"
	assetBaseDir = "../../../../cmd/lomod/test"
	infoFile     = "../../lomod/testdata/assets_info.json"
	previewTree1 = "../../lomod/testdata/tree_preview_insert_%s_%s.txt"
	previewTree2 = "../../lomod/testdata/tree_preview_all_%s_%s.txt"
	previewTree3 = "../../lomod/testdata/tree_preview_jitt_%s_%s.txt"
)

var (
	tmpDir               string
	imageDims, videoDims []types.Dimension
)

type fakePreviewFile struct {
	createTime time.Time
	size       int64
	name       string
}

func main() {
	if len(os.Args) != 3 {
		fmt.Println("gen-info [distribution] [release]")
		return
	}

	vips.Startup(&vips.Config{ReportLeaks: true, MaxCacheFiles: 0, MaxCacheMem: 0, MaxCacheSize: 0})
	defer vips.Shutdown()

	info := map[string]*atlomod.AssetInfo{}
	content, err := ioutil.ReadFile("../../lomod/testdata/assets_info.json")
	if err != nil {
		panic(err)
	}
	if err := json.Unmarshal(content, &info); err != nil {
		panic(err)
	}

	imageDims, err = types.NewDimensions(common.DefaultImagePreviewDims)
	if err != nil {
		panic(err)
	}
	imageDims = append(imageDims, types.Dimension{Width: common.DefaultPreviewWidth})
	videoDims, err = types.NewDimensions(common.DefaultVideoPreviewDims)
	if err != nil {
		panic(err)
	}

	tmpDir, err = ioutil.TempDir("", "")
	if err != nil {
		panic(err)
	}
	log.Printf("------ tmp directory: %s", tmpDir)
	//defer os.RemoveAll(tmpDir)

	// arch - distribution - release - filename - preview hash (480x320, 75x75, 320x0)
	dist := os.Args[1]
	release := os.Args[2]
	for k, a := range info {
		if filepath.Ext(k) == ".zip" {
			continue
		}
		if err := analysisAsset(dist, release, a); err != nil {
			panic(err)
		}
	}

	content, err = json.MarshalIndent(info, "", "  ")
	if err != nil {
		panic(err)
	}
	if err := ioutil.WriteFile(infoFile, content, 0644); err != nil {
		panic(err)
	}

	if err := generatePreviewTree(dist, release, info); err != nil {
		panic(err)
	}
}

func analysisAsset(dist, release string, a *atlomod.AssetInfo) error {
	if err := analysisAssetPreviews(dist, release, a); err != nil {
		return err
	}
	return analysisAssetXcode(dist, release, a)
}

func analysisAssetXcode(dist, release string, a *atlomod.AssetInfo) error {
	switch filepath.Ext(a.Path) {
	case ".dng":
	case ".heic":
	case ".webp":
	case ".bmp":
	default:
		return nil
	}
	if a.ImgXcode == nil {
		a.ImgXcode = map[string]map[string]map[string]atlomod.XcodeInfo{}
	}
	archInfo, ok := a.ImgXcode[runtime.GOARCH]
	if !ok {
		archInfo = map[string]map[string]atlomod.XcodeInfo{}
	}
	distInfo, ok := archInfo[dist]
	if !ok {
		distInfo = map[string]atlomod.XcodeInfo{}
	}
	releaseXcode, err := xcodeImage(filepath.Join(assetBaseDir, a.Path))
	if err != nil {
		return err
	}
	distInfo[release] = releaseXcode
	archInfo[dist] = distInfo
	a.ImgXcode[runtime.GOARCH] = archInfo
	return nil
}

func xcodeImage(assetPath string) (atlomod.XcodeInfo, error) {
	pinfo := atlomod.XcodeInfo{}
	_, base := filepath.Split(assetPath)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	previewFile := filepath.Join(tmpDir, base+".jpg")
	log.Printf("transcode image %s to %s", assetPath, previewFile)
	err := asset.XcodeImage(context.Background(), assetPath, previewFile, 0755)
	if err != nil {
		return pinfo, err
	}
	pinfo.Size, pinfo.SHA1, err = getSizeSHA(previewFile)
	if err != nil {
		return pinfo, err
	}

	// webp preview
	previewFile = filepath.Join(tmpDir, base+".webp")
	log.Printf("transcode image %s to %s", assetPath, previewFile)
	err = asset.XcodeImage(context.Background(), assetPath, previewFile, 0755)
	if err != nil {
		return pinfo, err
	}
	pinfo.WebpSize, pinfo.WebpSHA1, err = getSizeSHA(previewFile)
	return pinfo, nil
}
func analysisAssetPreviews(dist, release string, a *atlomod.AssetInfo) error {
	if a.Previews == nil {
		a.Previews = map[string]map[string]map[string]map[string]atlomod.XcodeInfo{}
	}
	archInfo, ok := a.Previews[runtime.GOARCH]
	if !ok {
		archInfo = map[string]map[string]map[string]atlomod.XcodeInfo{}
	}
	distInfo, ok := archInfo[dist]
	if !ok {
		distInfo = map[string]map[string]atlomod.XcodeInfo{}
	}
	imageDims, err := types.NewDimensions(common.DefaultImagePreviewDims)
	if err != nil {
		return err
	}
	imageDims = append(imageDims, types.Dimension{Width: common.DefaultPreviewWidth})
	videoDims, err = types.NewDimensions(common.DefaultVideoPreviewDims)
	if err != nil {
		return err
	}

	runner := preview.NewRunner(context.Background(), "ffmpeg", "ffprobe", "exiftool",
		common.DefaultFolderPermission, imageDims, videoDims, 1, nil)
	releasePreviews, err := generatePreviews(runner, filepath.Join(assetBaseDir, a.Path))
	if err != nil {
		return err
	}
	distInfo[release] = releasePreviews
	archInfo[dist] = distInfo
	a.Previews[runtime.GOARCH] = archInfo
	return nil
}

func generatePreviews(runner *preview.Runner, assetPath string) (map[string]atlomod.XcodeInfo, error) {
	pinfos := map[string]atlomod.XcodeInfo{}
	for _, d := range imageDims {
		pi, err := generatePreview(runner, assetPath, d.Width, d.Height, false)
		if err != nil {
			return nil, err
		}
		pinfos[fmt.Sprintf("%dx%d", d.Width, d.Height)] = pi
	}

	if ext.IsImageFile(assetPath) {
		return pinfos, nil
	}

	for _, d := range videoDims {
		pi, err := generatePreview(runner, assetPath, d.Width, d.Height, true)
		if err != nil {
			return nil, err
		}
		pinfos[fmt.Sprintf("%dx%d", d.Width, d.Height)] = pi
	}
	return pinfos, nil
}

func generatePreview(runner *preview.Runner, assetPath string, width, height uint, isVideo bool) (atlomod.XcodeInfo, error) {
	log.Printf("generate preview %s: %dx%d", assetPath, width, height)
	pinfo := atlomod.XcodeInfo{IsVideo: isVideo}
	previewFile, err := runner.GeneratePreviewByPath(context.Background(), 0755, types.PreviewRequest{
		Width: width, Height: height, MasterPath: assetPath, PreviewPath: tmpDir, GenVideo: isVideo})
	if err != nil {
		return pinfo, err
	}
	pinfo.Size, pinfo.SHA1, err = getSizeSHA(previewFile)
	if err != nil {
		return pinfo, err
	}
	previewFile, err = runner.GeneratePreviewByPath(context.Background(), 0755, types.PreviewRequest{
		Width: width, Height: height, MasterPath: assetPath, PreviewPath: tmpDir, GenVideo: isVideo, IsWebp: true})
	if err != nil {
		return pinfo, err
	}
	pinfo.WebpSize, pinfo.WebpSHA1, err = getSizeSHA(previewFile)
	return pinfo, err
}

func getSizeSHA(previewFile string) (int64, string, error) {
	f, err := os.Open(previewFile)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()
	hash := sha1.New()
	size, err := io.Copy(hash, f)
	if err != nil {
		return 0, "", err
	}
	return size, fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func generatePreviewTree(dist, release string, info map[string]*atlomod.AssetInfo) error {
	if err := generatePreviewTreeInsert(dist, release, info); err != nil {
		return err
	}

	if err := generatePreviewTreeJITT(dist, release, info); err != nil {
		return err
	}

	return generatePreviewTreeAll(dist, release, info)
}

func generatePreviewTreeInsert(dist, release string, info map[string]*atlomod.AssetInfo) error {
	files := []fakePreviewFile{}
	for f, i := range info {
		if i.ImageFile != nil && *i.ImageFile != "" {
			i = info[*i.ImageFile]
		}
		for dim, ai := range i.Previews[runtime.GOARCH][dist][release] {
			wh := strings.Split(dim, "x")
			if wh[0] == strconv.Itoa(common.DefaultPreviewWidth) {
				continue
			}
			dt := extractTime(i.Path)
			files = append(files,
				fakePreviewFile{
					size: ai.Size, createTime: dt, name: mkPreviewFilename(f, dim, wh[0], wh[1], dt, false),
				},
				fakePreviewFile{
					size: ai.WebpSize, createTime: dt, name: mkPreviewFilename(f, dim, wh[0], wh[1], dt, true),
				},
			)
		}
	}

	return mkFakePreviewFiles(fmt.Sprintf(previewTree1, runtime.GOARCH, release), files)
}

func generatePreviewTreeJITT(dist, release string, info map[string]*atlomod.AssetInfo) error {
	files := []fakePreviewFile{}
	for f, i := range info {
		if i.ImageFile != nil && *i.ImageFile != "" {
			i = info[*i.ImageFile]
		}
		for dim, ai := range i.Previews[runtime.GOARCH][dist][release] {
			wh := strings.Split(dim, "x")
			dt := extractTime(i.Path)
			files = append(files,
				fakePreviewFile{
					size: ai.Size, createTime: dt, name: mkPreviewFilename(f, dim, wh[0], wh[1], dt, false),
				},
				fakePreviewFile{
					size: ai.WebpSize, createTime: dt, name: mkPreviewFilename(f, dim, wh[0], wh[1], dt, true),
				},
			)
		}
	}

	return mkFakePreviewFiles(fmt.Sprintf(previewTree3, runtime.GOARCH, release), files)
}

func generatePreviewTreeAll(dist, release string, info map[string]*atlomod.AssetInfo) error {
	files := []fakePreviewFile{}
	for f, i := range info {
		if i.ImageFile != nil && *i.ImageFile != "" {
			i = info[*i.ImageFile]
		}
		dt := extractTime(i.Path)
		for dim, ai := range i.Previews[runtime.GOARCH][dist][release] {
			wh := strings.Split(dim, "x")
			files = append(files,
				fakePreviewFile{
					size: ai.Size, createTime: dt, name: mkPreviewFilename(f, dim, wh[0], wh[1], dt, false),
				},
				fakePreviewFile{
					size: ai.WebpSize, createTime: dt, name: mkPreviewFilename(f, dim, wh[0], wh[1], dt, true),
				},
			)
		}

		// try xcode image
		parts := strings.Split(f, ".")
		if i.ImgXcode == nil || parts[1] == "zip" {
			// not support live photo image transcoding
			continue
		}
		ai := i.ImgXcode[runtime.GOARCH][dist][release]
		files = append(files,
			fakePreviewFile{size: ai.Size, createTime: dt,
				name: fmt.Sprintf("%d%02d%02d_%s.jpg", dt.Year(), int(dt.Month()), dt.Day(), parts[0]),
			},
			fakePreviewFile{size: ai.WebpSize, createTime: dt,
				name: fmt.Sprintf("%d%02d%02d_%s.webp", dt.Year(), int(dt.Month()), dt.Day(), parts[0]),
			},
		)
	}

	return mkFakePreviewFiles(fmt.Sprintf(previewTree2, runtime.GOARCH, release), files)
}

func mkPreviewFilename(f, dim, width, height string, dt time.Time, isWebp bool) string {
	parts := strings.Split(f, ".")
	e := "mp4"
	if ext.IsImageFile(f) {
		if isWebp {
			e = "webp"
		} else if parts[1] == "png" {
			e = "png"
		} else {
			e = "jpg"
		}
	} else if parts[1] == "zip" {
		if isWebp {
			e = "webp"
		} else {
			e = "jpg"
		}
	} else if dim != common.DefaultVideoPreviewDims {
		if isWebp {
			e = "webp"
		} else {
			e = "jpg"
		}
	}
	return fmt.Sprintf("%d%02d%02d_%s_%s_%s.%s", dt.Year(), int(dt.Month()), dt.Day(), parts[0],
		width, height, e)
}

func extractTime(file string) time.Time {
	_, b := filepath.Split(file)
	parts := strings.Split(strings.Split(b, ".")[0], "_")
	t, err := time.Parse("2006-01-02_15:04:05", fmt.Sprintf("%s-%s-%s_12:00:00", parts[1], parts[2], parts[3]))
	if err != nil {
		panic(errors.Wrap(err, file))
	}
	return t
}

func mkFakePreviewFiles(treeFilename string, files []fakePreviewFile) error {
	if err := os.RemoveAll(previewRoot); err != nil {
		panic(err)
	}
	for _, f := range files {
		dir := fmt.Sprintf("%s/%d/%02d/%02d", previewRoot, f.createTime.Year(), int(f.createTime.Month()), f.createTime.Day())
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}

		fp, err := os.Create(filepath.Join(dir, f.name))
		if err != nil {
			return err
		}
		defer fp.Close()
		data := make([]byte, f.size)
		size, err := fp.Write(data)
		if err != nil {
			return err
		}
		if size != int(f.size) {
			return errors.Errorf("write fake preview file %s, expect size %d, got %d", f.name, f.size, size)
		}
	}

	content, err := exec.Command("tree", "-Ns", previewRoot).CombinedOutput()
	if err != nil {
		return err
	}
	return ioutil.WriteFile(treeFilename, content, 0644)
}
