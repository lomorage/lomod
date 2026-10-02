package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	testdata "bitbucket.org/lomoware/lomo-backend/cmd/lomod/test"
	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/scan"
	"bitbucket.org/lomoware/lomo-backend/common/testutil"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"github.com/leslie-wang/times"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	. "gopkg.in/check.v1"
)

func (ts *mainSuite) setupScan(c *C, root string) {
	// copy files to the scan directory and test Chinese
	content, err := exec.Command("cp", "-r", "/go/src/bitbucket.org/lomoware/lomo-backend/cmd/lomod/test/img", root+"/图片").CombinedOutput()
	c.Assert(err, IsNil, Commentf(string(content)))

	content, err = exec.Command("cp", "-r", "/go/src/bitbucket.org/lomoware/lomo-backend/cmd/lomod/test/video", root+"/video").CombinedOutput()
	c.Assert(err, IsNil, Commentf(string(content)))

	// setup timestamp
	for p, t := range testdata.NoExifFilesTime {
		dt, err := time.Parse(common.TimeFormatLomod, t)
		c.Assert(err, IsNil)
		if strings.Contains(p, "/go/src/bitbucket.org/lomoware/lomo-backend/cmd/lomod/test/img") {
			p = strings.Replace(p, "/go/src/bitbucket.org/lomoware/lomo-backend/cmd/lomod/test/img", root+"/图片", -1)
		} else if strings.Contains(p, "/go/src/bitbucket.org/lomoware/lomo-backend/cmd/lomod/test") {
			p = strings.Replace(p, "/go/src/bitbucket.org/lomoware/lomo-backend/cmd/lomod/test", root, -1)
		} else {
			p, _ = filepath.Split(root)
		}
		c.Assert(os.Chtimes(p, dt, dt), IsNil)
	}
}

func (ts *mainSuite) setupScanShort(c *C, root string) {
	// only copy 2 files for simplicity
	for idx, f := range gpsAssets {
		content, err := exec.Command("cp", "-r", f.file,
			filepath.Join(root, strconv.Itoa(idx)+filepath.Ext(f.file))).CombinedOutput()
		c.Assert(err, IsNil, Commentf(string(content)))
	}
}

func (ts *mainSuite) waitImportComplete() {
	for {
		if !ts.h.scanImportResult.End.IsZero() {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func (ts *mainSuite) waitScanComplete(retryCount int) (*scan.File, error) {
	for i := 0; i < retryCount; i++ {
		sc, f, err := ts.getScanResult()
		if err != nil {
			return nil, err
		}
		if sc == 200 {
			return f, nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil, errors.Errorf("scan not complete after %d retry", retryCount)
}

func (ts *mainSuite) getScanLog(c *C, root string) string {
	u := "/assets/scan/log?token=" + ts.token + "&path=" +
		url.QueryEscape(root)
	req, err := http.NewRequest("GET", u, nil)
	c.Assert(err, IsNil)

	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	c.Assert(rr.Result().StatusCode, Equals, 200)

	defer rr.Result().Body.Close()

	content, err := ioutil.ReadAll(rr.Result().Body)
	c.Assert(err, IsNil)
	return string(content)
}

func (ts *mainSuite) getScanResult() (int, *scan.File, error) {
	u := "/assets/scan?token=" + ts.token
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return 0, nil, err
	}

	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	if rr.Result().StatusCode != 200 {
		return rr.Result().StatusCode, nil, nil
	}

	defer rr.Result().Body.Close()

	ret := &scan.File{}
	return 200, ret, json.NewDecoder(rr.Result().Body).Decode(ret)
}

func (ts *mainSuite) TestScanBasic(c *C) {
	c.Skip("skip scan test until it really finish")
	ts.h.conf.MountDir = photodir

	scanTmpDir := filepath.Join(photodir, "scan_test")
	c.Assert(os.RemoveAll(scanTmpDir), IsNil)
	c.Assert(os.Mkdir(scanTmpDir, 0755), IsNil)

	ts.setupScan(c, scanTmpDir)

	url := "/assets/scan?token=" + ts.token
	ts.requestWithMethodBody(c, url, "POST", http.StatusOK, nil, nil, nil, nil)

	scanResult, err := ts.waitScanComplete(100)
	c.Assert(err, IsNil)

	// validate scan preview
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	previewRoot, err := ts.h.getScanPreviewRoot()
	c.Assert(err, IsNil)
	c.Assert(testutil.WaitWithFileCounts(ctx, previewRoot, 73), IsNil)

	expectSHA := map[string]string{
		"17363532de7bc73e42823c1448bd52ffe45d4bfc": "eb9019bfb361c7d4878d0f302945417f407c402c",
		"921196cc106f070668dbb2ff3eafc01017540a52": "f1514725a46cd53e248e81120d69b646092f25f4",
	}
	ts.validateScanPreview(c, scanResult, 75, 75, expectSHA)

	expectSHA = map[string]string{
		"17363532de7bc73e42823c1448bd52ffe45d4bfc": "97e352a2538cd01c16cd8d9b6f331bf90bfeb7d5",
		"921196cc106f070668dbb2ff3eafc01017540a52": "e3f6cb62499c063960e57153798a86acdd13177b",
	}
	ts.validateScanPreview(c, scanResult, 480, 320, expectSHA)

	expectSHA = map[string]string{
		"014cb0467a127fbda28c372fdee091a2c6cc8876": "b1025bec483693d16e35a1d3efab32deea5d75e7",
		"5d14e3ce82d2101b3b8ece22487d81d824a5745a": "44f44f7587a2e95db4fc95d0d4db8f953bb432ba",
	}
	ts.validateScanPreview(c, scanResult, 320, 0, expectSHA)

	// start compare scan result file
	f := filepath.Join(common.GetVarDir(ts.h.conf.BaseDir), scanResultFile)
	ts.validateScanResult(c, scanResult, f)

	ts.validateScanResult(c, scanResult, "./testdata/scan_all.json")

	// start import, then validate import
	// zero folders before import
	c.Assert(testutil.ValidateFilesInDir(photodir+"/alice/Photos/master/", 0, true), IsNil)
	c.Assert(testutil.ValidateFilesInDir(photodir+"/alice/Photos/master/", 0, false), IsNil)
	c.Assert(testutil.ValidateFilesInDir(photodir+"/alice/Photos/preview/", 0, true), IsNil)
	c.Assert(testutil.ValidateFilesInDir(photodir+"/alice/Photos/preview/", 0, false), IsNil)

	// import the whole path
	url = "/assets/scan/import/complete_import?token=" + ts.token
	buf := &bytes.Buffer{}
	c.Assert(json.NewEncoder(buf).Encode(scanResult), IsNil)
	ts.requestWithMethodBody(c, url, "POST", 200, buf, nil, nil, nil)

	// files should have import flag now
	ts.waitImportComplete()
	ts.validateScanResult(c, scanResult, "./testdata/scan_import_link.json")

	// create new context
	ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel2()

	// start validate master, preview and tree structure
	c.Assert(testutil.WaitWithFileCounts(ctx2, photodir+"/alice/Photos/master", 28), IsNil)
	c.Assert(testutil.WaitWithFileCounts(ctx2, photodir+"/alice/Photos/preview", 71), IsNil)
	c.Assert(testutil.CompareDirsByTreeCmd(photodir+"/alice/Photos/", "./testdata/tree_scan_import_link.txt"), IsNil)

	start := 1
	ts.validateImportDownload(c, scanResult, &start)

	// post again without move condition
	url = "/assets/scan/import/complete_import?token=" + ts.token
	ts.requestWithMethod(c, url, "POST", 200, nil, nil, nil)

	// resource should have same import flag
	ts.waitImportComplete()
	ts.validateScanResult(c, scanResult, "./testdata/scan_import_link.json")

	c.Assert(testutil.WaitWithFileCounts(ctx2, photodir+"/alice/Photos/master", 28), IsNil)
	c.Assert(testutil.WaitWithFileCounts(ctx2, photodir+"/alice/Photos/preview", 71), IsNil)
	c.Assert(testutil.CompareDirsByTreeCmd(photodir+"/alice/Photos/", "./testdata/tree_scan_import_link.txt"), IsNil)

	start = 1
	ts.validateImportDownload(c, scanResult, &start)

	// post again with move condition
	url = "/assets/scan/import/complete_import?move=1&token=" + ts.token
	ts.requestWithMethod(c, url, "POST", 200, nil, nil, nil)

	// resource should have move flag now
	ts.waitImportComplete()
	ts.validateScanResult(c, scanResult, "./testdata/scan_import_move.json")

	c.Assert(testutil.WaitWithFileCounts(ctx2, photodir+"/alice/Photos/master", 28), IsNil)
	c.Assert(testutil.WaitWithFileCounts(ctx2, photodir+"/alice/Photos/preview", 71), IsNil)
	c.Assert(testutil.CompareDirsByTreeCmd(photodir+"/alice/Photos/", "./testdata/tree_scan_import_move.txt"), IsNil)

	start = 1
	ts.validateImportDownload(c, scanResult, &start)
}

// only import subfolders in one scan
func (ts *mainSuite) TestScanImportSubfolders(c *C) {
}

func (ts *mainSuite) validateScanResult(c *C, f *scan.File, compareFileName string) {
	content, err := json.MarshalIndent(f, "", " ")
	c.Assert(err, IsNil)
	c.Assert(ioutil.WriteFile("/tmp/lomo.json", content, 0755), IsNil)

	expectContent, err := ioutil.ReadFile(compareFileName)
	c.Assert(err, IsNil)
	c.Assert(strings.TrimSpace(string(content)), DeepEquals,
		strings.TrimSpace(string(expectContent)))
}

func (ts *mainSuite) validateScanPreview(c *C, f *scan.File, width, height int, expectSHA map[string]string) {
	if f.IsDir() {
		for _, child := range f.Children {
			ts.validateScanPreview(c, child, width, height, expectSHA)
		}
		return
	}
	c.Assert(f.SHA1, NotNil, Commentf("%s doesn't have SHA", f.Path()))
	sha, ok := expectSHA[*f.SHA1]
	if !ok {
		return
	}
	url := fmt.Sprintf("/assets/scan/preview/%d/%d/%d/%s%s?token=%s&width=%d&height=%d",
		f.CreateTime.Year(), int(f.CreateTime.Month()), f.CreateTime.Day(),
		*f.SHA1, filepath.Ext(f.Name), ts.token, width, height)

	ts.assetGet(c, url, sha)
}

func (ts *mainSuite) validateImportDownload(c *C, file *scan.File, index *int) {
	// download imported photos
	if file.IsDir() {
		for _, f := range file.Children {
			ts.validateImportDownload(c, f, index)
		}
		return
	}
	url1 := fmt.Sprintf("/asset/%d?token=%s", *index, ts.token)
	if ext.IsVideoFile(filepath.Ext(file.Name)) {
		url1 += "&orig=1"
	}
	//fmt.Printf("----- download: %s\n", file.Path())
	ts.assetGet(c, url1, *file.SHA1)

	// download by hash

	*index = *index + 1
}

func (ts *mainSuite) TestScanAndImportNotMove(c *C) {
	c.Skip("skip scan test until it really finish")
	ts.testScanAndImportMove(c, false)
}

func (ts *mainSuite) TestScanAndImportMove(c *C) {
	ts.testScanAndImportMove(c, true)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// still 5_2003_11_23.jpg.part1 and 5_2003_11_23.jpg.part2 are left
	c.Assert(testutil.WaitWithFileCounts(ctx, photodir+"/scan_test", 2), IsNil)
}

func (ts *mainSuite) scanAndImport(c *C, move, scanVideo, useExifTime bool) types.Years {
	scanTmpDir := filepath.Join(photodir, "scan_test")
	c.Assert(os.RemoveAll(scanTmpDir), IsNil)
	c.Assert(os.Mkdir(scanTmpDir, 0755), IsNil)

	category := types.Years{}
	contents, err := ioutil.ReadFile("./testdata/category_scan.json")
	c.Assert(err, IsNil)
	c.Assert(json.Unmarshal(contents, &category), IsNil)

	ts.setupScan(c, scanTmpDir)

	// always scan video
	url := "/assets/scan?token=" + ts.token + "&" + common.QueryKeyScanAndImport + "=1&path=" +
		url.QueryEscape(scanTmpDir)
	if move {
		url += "&" + common.QueryKeyMove + "=1"
	}
	if scanVideo {
		url += "&" + common.QueryKeyScanVideo + "=1"
	}
	if useExifTime {
		url += "&" + common.QueryKeyUseExifTime + "=1"
	}
	ts.requestWithMethodBody(c, url, "POST", http.StatusOK, nil, nil, nil, nil)

	ts.waitImportComplete()

	return category
}

func (ts *mainSuite) testScanAndImportMove(c *C, move bool) {
	category := ts.scanAndImport(c, move, true, true)
	ts.validateBasicCategory(c, category)
	ts.validateAssetsDownload(c, category)

	// test asset download using short asset category to validate
	category, err := loadCategory()
	c.Assert(err, IsNil)
	ts.validateAssetsDownload(c, category)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c.Assert(testutil.WaitWithFileCounts(ctx, photodir+"/alice/Photos/preview", 79), IsNil)

	if move {
		// move may have file system sync issue, retry 10 times
		done := false
		for i := 0; i < 10; i++ {
			time.Sleep(time.Second)
			err := testutil.WaitWithFileCounts(ctx, photodir+"/alice/Photos/master", 32)
			if err != nil {
				logrus.Info(err)
				continue
			}
			err = testutil.CompareDirsByTreeCmd(photodir+"/alice/Photos/", "./testdata/tree_scan_import_move.txt")
			if err != nil {
				logrus.Info(err)
				continue
			}
			done = true
			break
		}
		c.Assert(done, Equals, true)
	} else {
		c.Assert(testutil.CompareDirsByTreeCmd(photodir+"/alice/Photos/", "./testdata/tree_scan_import_link.txt"), IsNil)
	}
	for _, a := range gpsAssets {
		for j, dim := range ts.h.previewDims {
			u := fmt.Sprintf("/asset/preview/%s?token=%s&width=%d&height=%d", a.hash, ts.token, dim.Width, dim.Height)
			ts.assetGet(c, u, a.previewHash[j])
		}
	}
}

func (ts *mainSuite) TestScanAndImportDeletion(c *C) {
	// delete one file, and scan again, and deleted files should be in the list of scan result
	scanTmpDir := filepath.Join(photodir, "scan_test")
	c.Assert(os.RemoveAll(scanTmpDir), IsNil)
	c.Assert(os.Mkdir(scanTmpDir, 0755), IsNil)

	ts.setupScanShort(c, scanTmpDir)

	u := "/assets/scan?token=" + ts.token + "&" + common.QueryKeyScanAndImport + "=1&path=" +
		url.QueryEscape(scanTmpDir) + "&" + common.QueryKeyUseExifTime + "=1"
	ts.requestWithMethodBody(c, u, "POST", http.StatusOK, nil, nil, nil, nil)

	ts.waitImportComplete()

	scanCategory := types.Years{Hash: gpsCategory.Hash}
	scanCategory.Years = make([]types.Year, len(gpsCategory.Years))
	copy(scanCategory.Years, gpsCategory.Years)
	for i, y := range scanCategory.Years {
		y.Hash = gpsCategory.Years[i].Hash
		y.Months = make([]types.Month, len(gpsCategory.Years[i].Months))
		copy(y.Months, gpsCategory.Years[i].Months)
		for j, m := range y.Months {
			m.Hash = gpsCategory.Years[i].Months[j].Hash
			m.Days = make([]types.Day, len(gpsCategory.Years[i].Months[j].Days))
			copy(m.Days, gpsCategory.Years[i].Months[j].Days)
			for k, d := range m.Days {
				d.Hash = gpsCategory.Years[i].Months[j].Days[k].Hash
				d.Assets = make([]types.Asset, len(gpsCategory.Years[i].Months[j].Days[j].Assets))
				copy(d.Assets, gpsCategory.Years[i].Months[j].Days[j].Assets)
				for m, a := range d.Assets {
					a.Status = 1
					d.Assets[m] = a
				}
				m.Days[k] = d
			}
			y.Months[j] = m
		}
		scanCategory.Years[i] = y
	}
	ts.validateBasicCategory(c, scanCategory)

	// due to not move, the directory is not created yet
	_, err := os.Stat(photodir + "/alice/Photos/master/2003/11/23/20031123_1.jpg")
	c.Assert(err, NotNil)
	c.Assert(os.IsNotExist(err), Equals, true, Commentf("%v", err))

	ts.requestDelete(c, ts.token, &types.DeleteAssetItems{
		List: []types.DeleteAssetItem{{ID: "1"}},
	})

	_, err = os.Stat(photodir + "/alice/Photos/master/2003/11/23/20031123_1.jpg")
	c.Assert(err, NotNil)
	c.Assert(os.IsNotExist(err), Equals, true, Commentf(err.Error()))

	ts.requestGet(c, fmt.Sprintf("/category/2003?token=%s", ts.token),
		&types.Year{}, &types.Year{Year: 2003, Months: []types.Month{}})

	ts.requestWithMethod(c, fmt.Sprintf("/asset/1.jpg?token=%s", ts.token), http.MethodHead, http.StatusNotFound,
		nil, nil, nil)
	ts.requestWithMethod(c, fmt.Sprintf("/asset/1.jpg?token=%s", ts.token), http.MethodGet, http.StatusNotFound,
		nil, nil, nil)

	u = "/assets/scan?token=" + ts.token + "&" + common.QueryKeyScanAndImport + "=1&path=" +
		url.QueryEscape(scanTmpDir) + "&move=1&" + common.QueryKeyUseExifTime + "=1"
	ts.requestWithMethodBody(c, u, "POST", http.StatusOK, nil, nil, nil, nil)

	ts.waitImportComplete()

	scanCategory.Years[0].Months[0].Days[0].Assets[0].Name = "3.jpg"
	scanCategory.Years[0].Months[0].Days[0].Assets[0].Status = 0
	ts.validateBasicCategory(c, scanCategory)

	_, err = os.Stat(photodir + "/alice/Photos/master/2003/11/23/20031123_3.jpg")
	c.Assert(err, IsNil)
	ts.assetGet(c, fmt.Sprintf("/asset/3.jpg?token=%s", ts.token), gpsAssets[0].hash)
}

func (ts *mainSuite) TestScanAndImportMultipleTime(c *C) {
	// scan and import same folders multiple time
	ts.h.conf.UseMemdb = true
	c.Assert(ts.h.openMemDB(), IsNil)

	scanTmpDir := filepath.Join(photodir, "scan_test")
	for i := 0; i < 10; i++ {
		c.Assert(os.RemoveAll(scanTmpDir), IsNil)
		c.Assert(os.Mkdir(scanTmpDir, 0755), IsNil)

		ts.setupScanShort(c, scanTmpDir)

		u := "/assets/scan?token=" + ts.token + "&" + common.QueryKeyScanAndImport + "=1&path=" +
			url.QueryEscape(scanTmpDir) + "&move=1&" + common.QueryKeyUseExifTime + "=1"
		ts.requestWithMethodBody(c, u, "POST", http.StatusOK, nil, nil, nil, nil)

		ts.waitImportComplete()

		ts.validateBasicCategory(c, gpsCategory)
		ts.validateAssetsDownload(c, gpsCategory)

		c.Assert(ts.h.scanRunner.Stats.TotalMediaFiles(), Equals, 2, Commentf("#%d iteration", i+1))
		if i == 0 {
			c.Assert(ts.h.scanRunner.Stats.TotalDuplicateFiles(), Equals, 0, Commentf("#%d iteration", i+1))
		} else {
			c.Assert(ts.h.scanRunner.Stats.TotalDuplicateFiles(), Equals, 2, Commentf("#%d iteration", i+1))
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		c.Assert(testutil.WaitWithFileCounts(ctx, photodir+"/alice/Photos/master", 2), IsNil)
		c.Assert(testutil.WaitWithFileCounts(ctx, photodir+"/alice/Photos/preview", 4), IsNil)
	}

	// only 1 albums should be exist
	albums := ts.listAlbums(c)
	c.Assert(len(albums.Albums), Equals, 1, Commentf("%+v", albums.Albums))
	c.Assert(albums.Albums[0].Title, Equals, scanTmpDir)

	assets := ts.listAlbumAssets(c, albums.Albums[0].ID, 0)
	c.Assert(assets, DeepEquals, []types.AssetName{
		{Name: "2.heic", Hash: gpsAssets[1].hash},
		{Name: "1.jpg", Hash: gpsAssets[0].hash},
	})
}

func (ts *mainSuite) TestScanAndImportTwoFolder(c *C) {
	dir1 := filepath.Join(photodir, "scan_test", "dir1")
	dir2 := filepath.Join(photodir, "scan_test", "dir2")
	c.Assert(os.RemoveAll(dir1), IsNil)
	c.Assert(os.RemoveAll(dir2), IsNil)
	c.Assert(os.MkdirAll(dir1, 0755), IsNil)
	c.Assert(os.MkdirAll(dir2, 0755), IsNil)

	// scan and import two folders one by one and make sure both scan and import work
	content, err := exec.Command("cp", "-r", gpsAssets[0].file, dir1).CombinedOutput()
	c.Assert(err, IsNil, Commentf(string(content)))

	content, err = exec.Command("cp", "-r", gpsAssets[1].file, dir2).CombinedOutput()
	c.Assert(err, IsNil, Commentf(string(content)))

	u := "/assets/scan?token=" + ts.token + "&" + common.QueryKeyScanAndImport + "=1&path=" +
		url.QueryEscape(dir1) + "&move=1&" + common.QueryKeyUseExifTime + "=1"
	ts.requestWithMethodBody(c, u, "POST", http.StatusOK, nil, nil, nil, nil)

	ts.waitImportComplete()

	// get log should be success
	log := ts.getScanLog(c, dir1)
	c.Assert(log, Equals, `1st PASS START: root folder (/tmp/usbdisk1/scan_test/dir1), ignore video (true), ignore EXIF analysis (false)
1st PASS FINISH: scanned 1 directories, found 1/0 image/video files, 0 malform media files, 0 other files
start import /tmp/usbdisk1/scan_test/dir1/5_2003_11_23.jpg
finish import /tmp/usbdisk1/scan_test/dir1/5_2003_11_23.jpg to /tmp/usbdisk1/alice/Photos/master/2003/11/23/20031123_1.jpg
FINISH: scanned 1 directories, imported 1/0 image/video files, ignore 0 video and 0 duplicated files
`)

	u = "/assets/scan?token=" + ts.token + "&" + common.QueryKeyScanAndImport + "=1&path=" +
		url.QueryEscape(dir2) + "&move=1&" + common.QueryKeyUseExifTime + "=1"
	ts.requestWithMethodBody(c, u, "POST", http.StatusOK, nil, nil, nil, nil)
	ts.waitImportComplete()

	// get log should be success
	log = ts.getScanLog(c, dir2)
	c.Assert(log, Equals, `1st PASS START: root folder (/tmp/usbdisk1/scan_test/dir2), ignore video (true), ignore EXIF analysis (false)
1st PASS FINISH: scanned 1 directories, found 1/0 image/video files, 0 malform media files, 0 other files
start import /tmp/usbdisk1/scan_test/dir2/14_2017_09_13.heic
finish import /tmp/usbdisk1/scan_test/dir2/14_2017_09_13.heic to /tmp/usbdisk1/alice/Photos/master/2017/09/13/20170913_2.heic
FINISH: scanned 1 directories, imported 1/0 image/video files, ignore 0 video and 0 duplicated files
`)

	// client should still be able to get dir1 log
	log = ts.getScanLog(c, dir1)
	c.Assert(log, Equals, `1st PASS START: root folder (/tmp/usbdisk1/scan_test/dir1), ignore video (true), ignore EXIF analysis (false)
1st PASS FINISH: scanned 1 directories, found 1/0 image/video files, 0 malform media files, 0 other files
start import /tmp/usbdisk1/scan_test/dir1/5_2003_11_23.jpg
finish import /tmp/usbdisk1/scan_test/dir1/5_2003_11_23.jpg to /tmp/usbdisk1/alice/Photos/master/2003/11/23/20031123_1.jpg
FINISH: scanned 1 directories, imported 1/0 image/video files, ignore 0 video and 0 duplicated files
`)

	ts.validateBasicCategory(c, gpsCategory)
	ts.validateAssetsDownload(c, gpsCategory)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c.Assert(testutil.WaitWithFileCounts(ctx, photodir+"/alice/Photos/master", 2), IsNil)
	c.Assert(testutil.WaitWithFileCounts(ctx, photodir+"/alice/Photos/preview", 4), IsNil)

	// validate albums
	albums := ts.listAlbums(c)
	c.Assert(len(albums.Albums), Equals, 2, Commentf("%+v", albums.Albums))
	c.Assert(albums.Albums[0].Title, Equals, dir1)
	c.Assert(albums.Albums[1].Title, Equals, dir2)

	assets := ts.listAlbumAssets(c, albums.Albums[0].ID, 0)
	c.Assert(len(assets), Equals, 1)
	c.Assert(assets[0].Name, Equals, "1.jpg")
	c.Assert(assets[0].Hash, Equals, gpsAssets[0].hash)
	assets = ts.listAlbumAssets(c, albums.Albums[1].ID, 0)
	c.Assert(len(assets), Equals, 1)
	c.Assert(assets[0].Name, Equals, "2.heic")
	c.Assert(assets[0].Hash, Equals, gpsAssets[1].hash)
}

func (ts *mainSuite) TestScanAndImportDuplicatedFiles(c *C) {
	// files are duplicate at different folders, scan and import should have only one copy
	// use this test to validate album as well
	ts.h.conf.UseMemdb = true
	c.Assert(ts.h.openMemDB(), IsNil)
	root := filepath.Join(photodir, "scan_test")
	for i := 0; i < 10; i++ {
		dir := filepath.Join(root, "dir"+strconv.Itoa(i))
		c.Assert(os.RemoveAll(dir), IsNil)
		c.Assert(os.MkdirAll(dir, 0755), IsNil)

		content, err := exec.Command("cp", "-r", gpsAssets[i%2].file,
			filepath.Join(dir, strconv.Itoa(i)+filepath.Ext(gpsAssets[i%2].file))).CombinedOutput()
		c.Assert(err, IsNil, Commentf(string(content)))
	}

	u := "/assets/scan?token=" + ts.token + "&" + common.QueryKeyScanAndImport + "=1&path=" +
		url.QueryEscape(root) + "&move=1&" + common.QueryKeyUseExifTime + "=1"
	ts.requestWithMethodBody(c, u, "POST", http.StatusOK, nil, nil, nil, nil)

	ts.waitImportComplete()

	ts.validateBasicCategory(c, gpsCategory)
	ts.validateAssetsDownload(c, gpsCategory)

	// validate album
	albums := ts.listAlbums(c)
	c.Assert(len(albums.Albums), Equals, 2, Commentf("%+v", albums.Albums))
	c.Assert(albums.Albums[0].Title, Equals, filepath.Join(root, "dir0"))
	c.Assert(albums.Albums[1].Title, Equals, filepath.Join(root, "dir1"))

	assets := ts.listAlbumAssets(c, albums.Albums[0].ID, 0)
	c.Assert(len(assets), Equals, 1)
	c.Assert(assets[0].Name, Equals, "1.jpg")
	c.Assert(assets[0].Hash, Equals, gpsAssets[0].hash)
	assets = ts.listAlbumAssets(c, albums.Albums[1].ID, 0)
	c.Assert(len(assets), Equals, 1)
	c.Assert(assets[0].Name, Equals, "2.heic")
	c.Assert(assets[0].Hash, Equals, gpsAssets[1].hash)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c.Assert(testutil.WaitWithFileCounts(ctx, photodir+"/alice/Photos/master", 2), IsNil)
	c.Assert(testutil.WaitWithFileCounts(ctx, photodir+"/alice/Photos/preview", 4), IsNil)
}

func (ts *mainSuite) TestScanAndImportFilename(c *C) {
	// test different types of file name
	// 1. chinese character
	// 2. multiple dot: "a.b.jpg"
	// 3. has space: "hello world.jpg"
	// 4. hidden file: ".2045.jpg"  --- this should fail
	// 5. Capitalized filename: "IMG_3456.JPG"
	root := filepath.Join(photodir, "scan_test")
	idx := 0
	year2003 := gpsCategory.Years[0]
	defer func() {
		// reset original value to avoid shadow copy
		year2003.Months[0].Days[0].Assets[0].Name = "1.jpg"
	}()

	for fn, ok := range map[string]bool{"你好.jpg": true, "a.b.jpg": true, "hello world.jpg": true,
		".2045.jpg": false, "IMG_3456.JPG": true} {
		if ok {
			idx++
		}
		c.Assert(os.RemoveAll(root), IsNil)
		c.Assert(os.MkdirAll(root, 0755), IsNil)
		content, err := exec.Command("cp", "-r", gpsAssets[0].file, filepath.Join(root, fn)).CombinedOutput()
		c.Assert(err, IsNil, Commentf(string(content)))

		u := "/assets/scan?token=" + ts.token + "&" + common.QueryKeyScanAndImport + "=1&path=" +
			url.QueryEscape(root) + "&move=1&" + common.QueryKeyUseExifTime + "=1"
		ts.requestWithMethodBody(c, u, "POST", http.StatusOK, nil, nil, nil, nil)

		ts.waitImportComplete()

		name := ""
		category := types.Years{}
		if ok {
			category.Hash = "5efb0d57d8168342b664f6128dfa359acde96c81"
			category.Years = []types.Year{year2003}
			name = strconv.Itoa(idx) + ".jpg"
			category.Years[0].Months[0].Days[0].Assets[0].Name = name
		} else {
			category.Years = []types.Year{}
		}
		ts.validateBasicCategory(c, category)
		ts.validateAssetsDownload(c, category)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if !ok {
			c.Assert(testutil.WaitWithFileCounts(ctx, photodir+"/alice/Photos/master", 0), IsNil)
			c.Assert(testutil.WaitWithFileCounts(ctx, photodir+"/alice/Photos/preview", 0), IsNil)
			continue
		}
		c.Assert(testutil.WaitWithFileCounts(ctx, photodir+"/alice/Photos/master", 1), IsNil)
		c.Assert(testutil.WaitWithFileCounts(ctx, photodir+"/alice/Photos/preview", 2), IsNil)

		// validate album
		albums := ts.listAlbums(c)
		c.Assert(len(albums.Albums), Equals, 1, Commentf("%+v", albums.Albums))
		c.Assert(albums.Albums[0].Title, Equals, root)

		assets := ts.listAlbumAssets(c, albums.Albums[0].ID, 0)
		c.Assert(len(assets), Equals, 1)
		c.Assert(assets[0].Name, Equals, name)
		c.Assert(assets[0].Hash, Equals, gpsAssets[0].hash)

		ts.requestDelete(c, ts.token, &types.DeleteAssetItems{
			List: []types.DeleteAssetItem{{ID: name}},
		})

		assets = ts.listAlbumAssets(c, albums.Albums[0].ID, 0)
		c.Assert(len(assets), Equals, 0)
	}
}

func (ts *mainSuite) TestScanAndImportSkipVideo(c *C) {
	ts.scanAndImport(c, true, false, true)

	var category types.Years
	ts.requestGet(c, fmt.Sprintf("/category?token=%s&%s=1", ts.token, common.QueryKeyAll), &category, nil)

	expCategory := types.Years{}
	contents, err := ioutil.ReadFile("./testdata/category_scan_novideo.json")
	c.Assert(err, IsNil)
	c.Assert(json.Unmarshal(contents, &expCategory), IsNil)

	c.Assert(category, DeepEquals, expCategory)
}

func getCreateTime(filename string) (t time.Time, err error) {
	ts, err := times.Stat(filename)
	if err != nil {
		return
	} else if ts.HasBirthTime() {
		t = ts.BirthTime()
	} else {
		t = ts.ModTime()
	}
	return t.UTC().Truncate(time.Second), err
}

func (ts *mainSuite) TestImportNoExiftime(c *C) {
	dir1 := filepath.Join(photodir, "scan_test", "dir1")
	c.Assert(os.RemoveAll(dir1), IsNil)
	c.Assert(os.MkdirAll(dir1, 0755), IsNil)

	// scan and import two folders one by one and make sure both scan and import work
	content, err := exec.Command("cp", "-r", gpsAssets[0].file, dir1).CombinedOutput()
	c.Assert(err, IsNil, Commentf(string(content)))

	createTime, err := getCreateTime(filepath.Join(dir1, filepath.Base(gpsAssets[0].file)))
	c.Assert(err, IsNil)

	u := "/assets/scan?token=" + ts.token + "&" + common.QueryKeyScanAndImport + "=1&path=" +
		url.QueryEscape(dir1) + "&move=1"
	ts.requestWithMethodBody(c, u, "POST", http.StatusOK, nil, nil, nil, nil)

	ts.waitImportComplete()

	newCategory := types.Year{
		Year: createTime.Year(),
		Hash: gpsCategory.Years[0].Hash,
		Months: []types.Month{
			{
				Days: []types.Day{
					{
						Assets: []types.Asset{{}},
					},
				},
			},
		},
	}
	newCategory.Months[0].Month = int(createTime.Month())
	newCategory.Months[0].Hash = gpsCategory.Years[0].Months[0].Hash
	newCategory.Months[0].Days[0].Day = createTime.Day()
	newCategory.Months[0].Days[0].Hash = gpsCategory.Years[0].Months[0].Days[0].Hash
	newCategory.Months[0].Days[0].Assets[0].Name = gpsCategory.Years[0].Months[0].Days[0].Assets[0].Name
	newCategory.Months[0].Days[0].Assets[0].Hash = gpsCategory.Years[0].Months[0].Days[0].Assets[0].Hash
	newCategory.Months[0].Days[0].Assets[0].Date.Time = createTime

	var year types.Year
	ts.requestGet(c, fmt.Sprintf("/category/%d?token=%s", createTime.Year(), ts.token), &year, &newCategory)

	u = "/asset/" + newCategory.Months[0].Days[0].Assets[0].Hash + `?token=` + ts.token
	ts.assetGet(c, u, newCategory.Months[0].Days[0].Assets[0].Hash)
}
