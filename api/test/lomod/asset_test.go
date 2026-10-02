package atlomod

import (
	"context"
	"crypto/sha1"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/client"
	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/testutil"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"github.com/pkg/errors"
	. "gopkg.in/check.v1"
)

func isDefaultPreviewWidth(filename, width string) bool {
	if ext.IsVideoFile(filename) {
		return width == strconv.Itoa(common.DefaultVideoPreviewWidth)
	}
	return width == strconv.Itoa(common.DefaultPreviewWidth)
}

func (ts *lomodAPISuite) validateCategoryEmpty(c *C) {
	yc, err := ts.lomoc.GetCategoryAll()
	c.Assert(err, IsNil)
	c.Assert(yc, DeepEquals, types.Years{Years: []types.Year{}})

	for _, y := range ts.category.Years {
		for _, m := range y.Months {
			for _, d := range m.Days {
				dc, err := ts.lomoc.GetCategoryDay(y.Year, m.Month, d.Day)
				c.Assert(err, IsNil)
				c.Assert(dc, DeepEquals, types.Day{Day: d.Day, Assets: []types.Asset{}})
			}
			mc, err := ts.lomoc.GetCategoryMonth(y.Year, m.Month)
			c.Assert(err, IsNil)
			c.Assert(mc, DeepEquals, types.Month{Month: m.Month, Days: []types.Day{}})
		}
		yc, err := ts.lomoc.GetCategoryYear(y.Year)
		c.Assert(err, IsNil)
		c.Assert(yc, DeepEquals, types.Year{Year: y.Year, Months: []types.Month{}})
	}
}

func (ts *lomodAPISuite) validateCategory(c *C, expectCategory types.Years) {
	// validate category
	for _, y := range expectCategory.Years {
		for _, m := range y.Months {
			for _, d := range m.Days {
				dc, err := ts.lomoc.GetCategoryDay(y.Year, m.Month, d.Day)
				c.Assert(err, IsNil)
				c.Assert(dc, DeepEquals, d)
			}
			mc, err := ts.lomoc.GetCategoryMonth(y.Year, m.Month)
			c.Assert(err, IsNil)
			c.Assert(mc, DeepEquals, m)
		}
		yc, err := ts.lomoc.GetCategoryYear(y.Year)
		c.Assert(err, IsNil)
		c.Assert(yc, DeepEquals, y)
	}

	for i, y := range expectCategory.Years {
		for m := range y.Months {
			expectCategory.Years[i].Months[m].Days = []types.Day{}
		}
	}

	yc, err := ts.lomoc.GetCategoryAll()
	c.Assert(err, IsNil)
	c.Assert(yc, DeepEquals, expectCategory)
}

func (ts *lomodAPISuite) validateAssetExist(a types.Asset) error {
	return ts.validateAssetExistLomoc(a, ts.lomoc)
}

func (ts *lomodAPISuite) validateAssetExistLomoc(a types.Asset, lomoc *client.Lomod) error {
	exist, part, err := lomoc.IsAssetExist(a.Hash)
	if err != nil {
		return err
	}
	if part != nil {
		return errors.Errorf("%s has partial download", a.Name)
	}
	if !exist {
		return errors.Errorf("%s is not exist", a.Name)
	}

	return nil
}

func (ts *lomodAPISuite) validateAssetDownload(id, expectSHA1 string, isPreview bool, q map[string]string) error {
	return ts.validateAssetDownloadLomoc(id, expectSHA1, isPreview, q, ts.lomoc)
}

func (ts *lomodAPISuite) validateAssetDownloadLomoc(id, expectSHA1 string, isPreview bool,
	q map[string]string, lomoc *client.Lomod) error {
	h := sha1.New()
	r, err := lomoc.DownloadAsset(id, isPreview, q)
	if err != nil {
		return err
	}
	defer r.Close()

	_, err = io.Copy(h, r)
	if err != nil {
		return err
	}

	obtain := fmt.Sprintf("%x", h.Sum(nil))
	if obtain == expectSHA1 {
		return nil
	}
	return errors.Errorf("obtained: %s, expect: %s", obtain, expectSHA1)
}

func (ts *lomodAPISuite) validateAssetMasterDownload(a types.Asset) error {
	if err := ts.validateAssetExist(a); err != nil {
		return err
	}

	var q map[string]string
	if ext.IsVideoFile(filepath.Ext(a.Name)) {
		q = map[string]string{common.QueryKeyOrig: "1"}
	}

	expectSHA1 := a.Hash
	file := ts.assetsInfo[a.Name]
	if file.FileSHA1 != nil {
		expectSHA1 = *file.FileSHA1
	}

	// try to download using the whole name
	parts := strings.Split(a.Name, ".")
	ids := []string{a.Name, parts[0]}
	for _, id := range ids {
		if err := ts.validateAssetDownload(id, expectSHA1, false, q); err != nil {
			return err
		}
	}

	return nil
}

func (ts *lomodAPISuite) validateXcodeImageDownload(a types.Asset) error {
	file := ts.assetsInfo[a.Name]
	if file.ImgXcode == nil {
		return errors.Errorf("%s has no ImgXcode data", a.Name)
	}
	info := file.ImgXcode[runtime.GOARCH][ts.osDistro][ts.osRelease]
	qJpg := map[string]string{common.QueryKeyICodec: "jpg"}
	qWebp := map[string]string{common.QueryKeyICodec: "webp"}
	parts := strings.Split(a.Name, ".")
	ids := []string{a.Name, parts[0]}
	for _, id := range ids {
		if err := ts.validateAssetDownload(id, info.SHA1, false, qJpg); err != nil {
			return err
		}
		if err := ts.validateAssetDownload(id, info.WebpSHA1, false, qWebp); err != nil {
			return err
		}
	}
	return nil
}

func (ts *lomodAPISuite) validateXcodeVideoDownload(a types.Asset, useXcodeSHA1, videoPreviewMiss bool) error {
	if err := ts.validateAssetExist(a); err != nil {
		return err
	}
	expectSHA1 := a.Hash
	// if video preview is not exist, use original video sha
	if !videoPreviewMiss && useXcodeSHA1 {
		for _, p := range ts.assetsInfo[a.Name].Previews[runtime.GOARCH][ts.osDistro][ts.osRelease] {
			if p.IsVideo {
				expectSHA1 = p.SHA1
				break
			}
		}
		if expectSHA1 == "" {
			return errors.Errorf("%s-%s-%s %s: %s", runtime.GOARCH, ts.osDistro, ts.osRelease, a.Name, ts.assetsInfo[a.Name].Path)
		}
	}
	// try to download using the whole name
	parts := strings.Split(a.Name, ".")
	for _, id := range []string{a.Name, parts[0]} {
		if err := ts.validateAssetDownload(id, expectSHA1, false, nil); err != nil {
			return err
		}
	}
	return nil
}

func (ts *lomodAPISuite) validateAssetPreviewDownload(a types.Asset, videoPreviewMiss bool) error {
	if err := ts.validateAssetExist(a); err != nil {
		return err
	}

	info := ts.assetsInfo[a.Name]
	previews := info.Previews
	if previews == nil {
		if info.ImageFile == nil {
			return errors.Errorf("Asset %s has not mapping image file", a.Name)
		}
		previews = ts.assetsInfo[*info.ImageFile].Previews
	}
	for d, p := range previews[runtime.GOARCH][ts.osDistro][ts.osRelease] {
		parts := strings.Split(d, "x")
		if p.IsVideo {
			if parts[0] != strconv.Itoa(common.DefaultVideoPreviewWidth) {
				return errors.Errorf("Asset %s video preview size should only be default value, but got %s", a.Name, d)
			}
			parts = strings.Split(a.Name, ".")
			for _, id := range []string{a.Name, parts[0]} {
				// video preview is downloaded through regular /asset/{id} path
				// if video preview is not created yet, use original video file SHA1
				sha := p.SHA1
				if videoPreviewMiss {
					sha = a.Hash
				}
				if err := ts.validateAssetDownload(id, sha, false, nil); err != nil {
					return err
				}
			}
			continue
		}
		qJpg := map[string]string{common.QueryKeyICodec: "jpg"}
		qWebp := map[string]string{common.QueryKeyICodec: "webp"}
		if !isDefaultPreviewWidth(a.Name, parts[0]) {
			qJpg[common.QueryKeyWidth] = parts[0]
			qJpg[common.QueryKeyHeight] = parts[1]
			qWebp[common.QueryKeyWidth] = parts[0]
			qWebp[common.QueryKeyHeight] = parts[1]
		}

		// try to download using the whole name
		parts = strings.Split(a.Name, ".")
		for _, id := range []string{a.Name, parts[0]} {
			if err := ts.validateAssetDownload(id, p.SHA1, true, qJpg); err != nil {
				return err
			}
			if err := ts.validateAssetDownload(id, p.WebpSHA1, true, qWebp); err != nil {
				return err
			}
		}
	}
	return nil
}

func (ts *lomodAPISuite) TestAssetBasic(c *C) { // nolint: gocyclo
	// category should be empty at the beginning
	ts.validateCategoryEmpty(c)

	// validate exist or not
	for _, y := range ts.category.Years {
		for _, m := range y.Months {
			for _, d := range m.Days {
				for _, a := range d.Assets {
					exist, part, err := ts.lomoc.IsAssetExist(a.Hash)
					c.Assert(err, IsNil)
					c.Assert(part, IsNil)
					c.Assert(exist, Equals, false)
				}
			}
		}
	}

	// insert assets
	c.Assert(ts.insertDefaultAssets(), IsNil)

	// validate download
	for _, y := range ts.category.Years {
		for _, m := range y.Months {
			for _, d := range m.Days {
				for _, a := range d.Assets {
					c.Assert(ts.validateAssetMasterDownload(a), IsNil, Commentf("Asset %s", a.Name))
				}
			}
		}
	}

	// compare preview tree to make sure preview is generated correctly
	_, previewDir := common.GetUserPhotoDir(getAliceHome())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c.Assert(testutil.WaitWithFileCounts(ctx, previewDir, defaultPreviewFilesCount), IsNil)
	c.Assert(testutil.CompareDirsByTreeCmd(previewDir, fmt.Sprintf(previewTree1, runtime.GOARCH, ts.osRelease)), IsNil)

	for _, y := range ts.category.Years {
		for _, m := range y.Months {
			for _, d := range m.Days {
				for _, a := range d.Assets {
					// validate video non-origin
					if ext.IsVideoFile(filepath.Ext(a.Name)) {
						c.Assert(ts.validateXcodeVideoDownload(a, true, false), IsNil, Commentf("Asset %s", a.Name))
					} else {
						// download image transcoding version
						switch filepath.Ext(a.Name) {
						case ".bmp":
							fallthrough
						case ".tif":
							fallthrough
						case ".dng":
							fallthrough
						case ".heic":
							fallthrough
						case ".webp":
							c.Assert(ts.validateXcodeImageDownload(a), IsNil, Commentf("Asset %s", a.Name))
						}
					}
					// validate preview download
					c.Assert(ts.validateAssetPreviewDownload(a, false), IsNil, Commentf("Preview %s: %s", a.Name, d))
				}
			}
		}
	}

	// validate preview directory again for JITT assets
	c.Assert(testutil.CompareDirsByTreeCmd(previewDir, fmt.Sprintf(previewTree2, runtime.GOARCH, ts.osRelease)), IsNil)

	// validate delete - delete all preview
	// trashbox should not be created now
	_, err := os.Stat(common.GetUserPhotoTrashRootDir(getAliceHome()))
	c.Assert(err, NotNil)
	c.Assert(os.IsNotExist(err), Equals, true)

	// delete previews should trigger JITT preview generation
	c.Assert(os.RemoveAll(previewDir), IsNil)
	for _, y := range ts.category.Years {
		for _, m := range y.Months {
			for _, d := range m.Days {
				for _, a := range d.Assets {
					c.Assert(ts.validateAssetPreviewDownload(a, true), IsNil, Commentf("Preview %s: %s", a.Name, d))

					// video asset download should return origin one for either w or w/ orig query
					if !ext.IsVideoFile(filepath.Ext(a.Name)) {
						continue
					}
					c.Assert(ts.validateXcodeVideoDownload(a, true, true), IsNil, Commentf("Asset %s", a.Name))
					c.Assert(ts.validateAssetMasterDownload(a), IsNil, Commentf("Asset %s", a.Name))
				}
			}
		}
	}

	// compare preview folder again and it should contain JITT assets now
	c.Assert(testutil.CompareDirsByTreeCmd(previewDir, fmt.Sprintf(previewTree3, runtime.GOARCH, ts.osRelease)), IsNil)

	// validate category finally because validateCategory will remove default category
	ts.validateCategory(c, ts.category)

	// delete the asset 8.dng because it is the year having one single asset
	c.Assert(ts.lomoc.DeleteAsset("8.dng"), IsNil)
	delCategory, err := loadCategory(categoryDelete1)
	c.Assert(err, IsNil)
	ts.validateCategory(c, delCategory) // validate category after the delete

	// delete the 1.jpg by its hash and 2.zip by its id
	c.Assert(ts.lomoc.DeleteAsset("17363532de7bc73e42823c1448bd52ffe45d4bfc"), IsNil)
	c.Assert(ts.lomoc.DeleteAsset("2"), IsNil)
	delCategory, err = loadCategory(categoryDelete2)
	c.Assert(err, IsNil)

	// get assets list before validate category because it may clean up asset list
	// double delete previous assets
	assetsDelete := []string{"8.dng", "17363532de7bc73e42823c1448bd52ffe45d4bfc", "2", "10000"}
	assetsDeleteReply := types.DeleteAssetItems{List: []types.DeleteAssetItem{
		{ID: "8.dng", Reason: common.ErrAssetNotExistForUser.Error()},
		{ID: "17363532de7bc73e42823c1448bd52ffe45d4bfc", Type: types.Hash, Reason: common.ErrAssetNotExistForUser.Error()},
		{ID: "2", Reason: common.ErrAssetNotExistForUser.Error()},
		{ID: "10000", Reason: common.ErrAssetNotExistForUser.Error()}}}
	idx := 0
	for _, y := range delCategory.Years {
		for _, m := range y.Months {
			for _, d := range m.Days {
				for _, a := range d.Assets {
					item := types.DeleteAssetItem{Result: true}
					switch idx % 3 {
					case 0:
						item.ID = a.Name
					case 1:
						item.ID = a.Hash
						item.Type = types.Hash
					case 2:
						item.ID = strings.Split(a.Name, ".")[0]
					}
					idx++
					assetsDelete = append(assetsDelete, item.ID)
					assetsDeleteReply.List = append(assetsDeleteReply.List, item)
				}
			}
		}
	}

	ts.validateCategory(c, delCategory) // validate category after the delete
	reply, err := ts.lomoc.DeleteAssets(assetsDelete)
	c.Assert(err, IsNil)
	c.Assert(*reply, DeepEquals, assetsDeleteReply)

	// category should be empty now
	ts.validateCategoryEmpty(c)

	// compare alice's top directory
	c.Assert(testutil.CompareDirsByTreeCmd(getAliceHome(), trashTree), IsNil)

	// reload category and reinsert assets
	ts.category, err = loadCategory(categoryFile)
	c.Assert(err, IsNil)

	// try download all master / preview assets and should be empty now
	for _, y := range ts.category.Years {
		for _, m := range y.Months {
			for _, d := range m.Days {
				for _, a := range d.Assets {
					exist, _, err := ts.lomoc.IsAssetExist(a.Hash)
					c.Assert(err, IsNil)
					c.Assert(exist, Equals, false, Commentf("Asset: %s", a.Name))

					for _, b := range []bool{true, false} {
						_, err = ts.lomoc.DownloadAsset(a.Name, b, nil)
						c.Assert(err, Equals, common.ErrAssetNotExistForUser)
					}
				}
			}
		}
	}

	si, err := ts.lomoc.System()
	c.Assert(err, IsNil)
	c.Assert(len(si.UserStatus), Equals, 1)
	status, ok := si.UserStatus[alice]
	c.Assert(ok, Equals, true)
	c.Assert(status.HomeDisk.Status, Equals, "")
	// all assets should have 10M at least
	c.Assert(status.HomeDisk.FreeSizeInMB < usbMockFreeSizeMB-10, Equals, true)
	c.Assert(status.BackupDisk, IsNil)

	c.Assert(len(si.UserDisks), Equals, 1)
	c.Assert(si.UserDisks[0].Username, Equals, alice)
	c.Assert(si.UserDisks[0].FreeSize < usbMockFreeSizeMB-10, Equals, true)
	c.Assert(si.UserDisks[0].Error, Equals, "")
}

func (ts *lomodAPISuite) TestAssetExiftime(c *C) {
	// this test case let backend figure out create time, client should provide last modified time
	// if no create time in media file and also no last modified time. Backend should return failure
	baseMediaDir := filepath.Join(baseWorkdir, testAssetDir)
	for _, y := range ts.category.Years {
		for _, m := range y.Months {
			for _, d := range m.Days {
				for _, a := range d.Assets {
					file, ok := ts.assetsInfo[a.Name]
					c.Assert(ok, Equals, true, Commentf("not found %s in asset map", a.Name))
					f, err := os.Open(filepath.Join(baseMediaDir, file.Path))
					c.Assert(err, IsNil)
					defer f.Close()
					if file.ModifiedTime == nil {
						_, err := ts.lomoc.UploadAsset(f, nil, a.Hash, time.Time{}, false, nil)
						c.Assert(err, IsNil, Commentf(a.Name))
						continue
					}
					// the file doesn't have time, so without any time flag should return fail
					_, err = ts.lomoc.UploadAsset(f, nil, a.Hash, time.Time{}, false, nil)
					c.Assert(strings.Contains(err.Error(), common.ErrNoCreateTime.Error()),
						Equals, true, Commentf(a.Name))

					// open again since last one has been closed
					f2, err := os.Open(filepath.Join(baseMediaDir, file.Path))
					c.Assert(err, IsNil)
					defer f2.Close()
					_, err = ts.lomoc.UploadAsset(f2, nil, a.Hash, file.ModifiedTime.Time, false, nil)
					c.Assert(err, IsNil, Commentf(a.Name))
				}
			}
		}
	}

	// compare tree
	masterDir, previewDir := common.GetUserPhotoDir(getAliceHome())
	c.Assert(testutil.CompareDirsByTreeCmd(masterDir, masterTree), IsNil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c.Assert(testutil.WaitWithFileCounts(ctx, previewDir, defaultPreviewFilesCount), IsNil)
	c.Assert(testutil.CompareDirsByTreeCmd(previewDir, fmt.Sprintf(previewTree1, runtime.GOARCH, ts.osRelease)), IsNil)
}
