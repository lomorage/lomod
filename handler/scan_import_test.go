package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/asset"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/scan"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	. "gopkg.in/check.v1"
)

// Integration tests for the web "Local Import" flow: POST /assets/scan ->
// GET /assets/scan -> POST /assets/scan/import/{name}, with progress read
// from GET /assets/scan/status, all through the real router, db and
// filesystem.

// fixtures with a DateTimeOriginal EXIF tag, and the date it holds
var scanImportFixtures = []struct {
	src, rel string
	sha1     string
	exifDate time.Time
}{
	{"../cmd/lomod/test/img/5_2003_11_23.jpg", filepath.Join("trip", "a.jpg"),
		"4ebf54db04f335ff66bfc1fd982be62bf23fc967", time.Date(2003, 11, 23, 0, 0, 0, 0, time.UTC)},
	{"../cmd/lomod/test/img/3_2003_11_01.jpg", filepath.Join("trip", "day2", "b.jpg"),
		"", time.Date(2003, 11, 1, 0, 0, 0, 0, time.UTC)},
}

// file system time the fixtures get, far from their EXIF date so a test can
// tell which one the scan picked
var scanImportFSTime = time.Date(2020, 5, 6, 7, 8, 9, 0, time.UTC)

// setupScanImportSource copies the fixtures into a fresh folder outside every
// user's home dir (like a USB stick) and returns it. The folder is unique per
// call rather than wiped and reused: on Windows the test suite's vips file
// cache can still hold the previous test's scanned originals open.
func (ts *mainSuite) setupScanImportSource(c *C) string {
	src := filepath.Join(os.TempDir(), fmt.Sprintf("lomo-scan-import-src-%d", time.Now().UnixNano()))
	// start from an empty library: SetUpTest wipes photodir, but alice's
	// resolved home dir isn't under it on every platform
	c.Assert(os.RemoveAll(filepath.Join(ts.aliceHome(c), common.AppPhoto)), IsNil)
	for i, f := range scanImportFixtures {
		dst := filepath.Join(src, f.rel)
		c.Assert(os.MkdirAll(filepath.Dir(dst), 0755), IsNil)
		c.Assert(common.CopyFile(f.src, dst), IsNil)
		c.Assert(os.Chtimes(dst, scanImportFSTime, scanImportFSTime), IsNil)
		if f.sha1 == "" {
			sha, err := common.GetFileSHA(dst)
			c.Assert(err, IsNil)
			scanImportFixtures[i].sha1 = sha
		}
	}
	return src
}

func (ts *mainSuite) scanImportStatus(c *C) scanStatus {
	body, err := ts.request("/assets/scan/status?token="+ts.token, http.MethodGet, http.StatusOK, nil, nil)
	c.Assert(err, IsNil)
	defer body.Close()
	st := scanStatus{}
	c.Assert(json.NewDecoder(body).Decode(&st), IsNil)
	return st
}

// scanForImport runs a scan-only request the way the web page does and
// waits for its result tree.
func (ts *mainSuite) scanForImport(c *C, dir, params string) *scan.File {
	u := "/assets/scan?token=" + ts.token + "&path=" + url.QueryEscape(dir)
	if params != "" {
		u += "&" + params
	}
	body, err := ts.request(u, http.MethodPost, http.StatusOK, nil, nil)
	c.Assert(err, IsNil)
	body.Close()

	tree, err := ts.waitScanComplete(120)
	c.Assert(err, IsNil)
	c.Assert(tree, NotNil)
	scan.RecreateParent(tree)

	st := ts.scanImportStatus(c)
	c.Assert(st.Scan.Running, Equals, false)
	return tree
}

// importScanned posts tree for import and waits until the import finishes,
// returning the final progress.
func (ts *mainSuite) importScanned(c *C, name string, tree *scan.File, params string) importProgress {
	content, err := json.Marshal(tree)
	c.Assert(err, IsNil)
	u := "/assets/scan/import/" + name + "?token=" + ts.token + "&scan-video=1"
	if params != "" {
		u += "&" + params
	}
	body, err := ts.request(u, http.MethodPost, http.StatusOK, bytes.NewReader(content), nil)
	c.Assert(err, IsNil)
	body.Close()

	for i := 0; i < 240; i++ {
		st := ts.scanImportStatus(c)
		if !st.Import.Running {
			// the per-name result must be served once the import is over
			body, err := ts.request("/assets/scan/import/"+name+"?token="+ts.token, http.MethodGet,
				http.StatusOK, nil, nil)
			c.Assert(err, IsNil)
			body.Close()
			return st.Import
		}
		time.Sleep(250 * time.Millisecond)
	}
	c.Fatalf("import %s not finished in time", name)
	return importProgress{}
}

func scannedFiles(tree *scan.File) []*scan.File {
	if !tree.IsDir() {
		return []*scan.File{tree}
	}
	var files []*scan.File
	for _, f := range tree.Children {
		files = append(files, scannedFiles(f)...)
	}
	return files
}

func (ts *mainSuite) assetByHash(c *C, hash string) *types.Asset {
	var a *types.Asset
	c.Assert(dbx.InQuery(ts.h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		a, err = asset.GetAssetByHash(ctx, tx, ts.alice.ID, hash)
		return err
	}), IsNil)
	return a
}

// aliceHome is alice's home dir as stored in the db, which can differ from
// ts.alice.HomeDir once resolved (e.g. on Windows)
func (ts *mainSuite) aliceHome(c *C) string {
	var home string
	c.Assert(dbx.InQuery(ts.h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		home, err = user.GetHomedir(ctx, tx, ts.alice.ID)
		return err
	}), IsNil)
	return home
}

// masterDayDir is alice's library folder for day t
func (ts *mainSuite) masterDayDir(c *C, t time.Time) string {
	master, _ := common.GetUserPhotoDir(ts.aliceHome(c))
	return common.NormalDatedDirName(master, t.Year(), int(t.Month()), t.Day())
}

func dirFileCount(c *C, dir string) int {
	entries, err := ioutil.ReadDir(dir)
	if os.IsNotExist(err) {
		return 0
	}
	c.Assert(err, IsNil)
	return len(entries)
}

func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

func (ts *mainSuite) TestScanImportCopyKeepsOriginals(c *C) {
	src := ts.setupScanImportSource(c)

	tree := ts.scanForImport(c, src, "scan-video=1&exif-time=1")
	files := scannedFiles(tree)
	c.Assert(files, HasLen, len(scanImportFixtures))
	// exif-time=1 must be honoured by a scan-only request
	for _, f := range files {
		c.Assert(f.CreateTime.Year(), Equals, 2003, Commentf("%s got %s", f.Name, f.CreateTime))
	}

	st := ts.scanImportStatus(c)
	c.Assert(st.Scan.Total, Equals, len(scanImportFixtures))
	c.Assert(st.Scan.Checked, Equals, len(scanImportFixtures))
	c.Assert(st.Scan.New, Equals, len(scanImportFixtures))
	c.Assert(st.Scan.Existing, Equals, 0)
	c.Assert(st.Scan.Counting, Equals, false)

	res := ts.importScanned(c, "copy-test", tree, "copy=1")
	c.Assert(res.Total, Equals, len(scanImportFixtures))
	c.Assert(res.Done, Equals, len(scanImportFixtures))
	c.Assert(res.Imported, Equals, len(scanImportFixtures))
	c.Assert(res.Skipped, Equals, 0)
	c.Assert(res.Failures, HasLen, 0)

	for _, f := range scanImportFixtures {
		// original left untouched
		_, err := os.Stat(filepath.Join(src, f.rel))
		c.Assert(err, IsNil, Commentf("original %s gone", f.rel))
		// a copy lands in the library, under its EXIF date
		c.Assert(dirFileCount(c, ts.masterDayDir(c, f.exifDate)), Equals, 1,
			Commentf("no library copy of %s", f.rel))
		a := ts.assetByHash(c, f.sha1)
		c.Assert(sameDay(a.Date.Time, f.exifDate), Equals, true, Commentf("%s dated %s", f.rel, a.Date.Time))
	}

	// the private copies made on the way in are cleaned up
	tmpEntries, err := ioutil.ReadDir(common.GetUserTmpDir(ts.aliceHome(c)))
	c.Assert(err, IsNil)
	for _, e := range tmpEntries {
		c.Assert(strings.HasPrefix(e.Name(), "import-"), Equals, false, Commentf("leftover %s", e.Name()))
	}

	// and the import log is written for this folder
	log := ts.getScanLog(c, src)
	c.Assert(strings.Contains(log, "FINISH: imported 2, skipped 0 already in library, failed 0"), Equals, true,
		Commentf("log: %s", log))

	// scanning again finds nothing new, and reports what it skipped
	tree = ts.scanForImport(c, src, "scan-video=1&exif-time=1")
	c.Assert(scannedFiles(tree), HasLen, 0)
	st = ts.scanImportStatus(c)
	c.Assert(st.Scan.Total, Equals, len(scanImportFixtures))
	c.Assert(st.Scan.New, Equals, 0)
	c.Assert(st.Scan.Existing, Equals, len(scanImportFixtures))
}

func (ts *mainSuite) TestScanImportLinkLeavesFilesInPlace(c *C) {
	src := ts.setupScanImportSource(c)

	tree := ts.scanForImport(c, src, "scan-video=1&exif-time=1")
	res := ts.importScanned(c, "link-test", tree, "")
	c.Assert(res.Imported, Equals, len(scanImportFixtures))
	c.Assert(res.Failures, HasLen, 0)

	for _, f := range scanImportFixtures {
		_, err := os.Stat(filepath.Join(src, f.rel))
		c.Assert(err, IsNil)
		c.Assert(dirFileCount(c, ts.masterDayDir(c, f.exifDate)), Equals, 0,
			Commentf("linked %s should not be copied into the library", f.rel))
		ts.assetByHash(c, f.sha1)
	}
}

func (ts *mainSuite) TestScanImportTwiceSkipsExisting(c *C) {
	src := ts.setupScanImportSource(c)

	tree := ts.scanForImport(c, src, "scan-video=1&exif-time=1")
	res := ts.importScanned(c, "first", tree, "copy=1")
	c.Assert(res.Imported, Equals, len(scanImportFixtures))

	// re-posting the same (now stale) scan result must not duplicate anything
	res = ts.importScanned(c, "second", tree, "copy=1")
	c.Assert(res.Total, Equals, len(scanImportFixtures))
	c.Assert(res.Imported, Equals, 0)
	c.Assert(res.Skipped, Equals, len(scanImportFixtures))
	c.Assert(res.Failures, HasLen, 0)
	for _, f := range scanImportFixtures {
		c.Assert(dirFileCount(c, ts.masterDayDir(c, f.exifDate)), Equals, 1)
	}
}

func (ts *mainSuite) TestScanImportReportsFailures(c *C) {
	src := ts.setupScanImportSource(c)

	tree := ts.scanForImport(c, src, "scan-video=1&exif-time=1")
	// a file vanishing between scan and import (e.g. USB stick pulled):
	// point one scanned entry at a file that isn't there
	files := scannedFiles(tree)
	c.Assert(files, HasLen, len(scanImportFixtures))
	files[0].Name = "vanished.jpg"
	gone := files[0].Path()

	res := ts.importScanned(c, "fail-test", tree, "copy=1")
	c.Assert(res.Total, Equals, len(scanImportFixtures))
	c.Assert(res.Done, Equals, len(scanImportFixtures))
	c.Assert(res.Imported, Equals, 1)
	c.Assert(res.Failures, HasLen, 1)
	c.Assert(res.Failures[0].Path, Equals, gone)
	c.Assert(res.Failures[0].Reason, Not(Equals), "")

	log := ts.getScanLog(c, src)
	c.Assert(strings.Contains(log, "failed to import "+gone), Equals, true, Commentf("log: %s", log))
}

func (ts *mainSuite) TestScanImportRejectsConcurrentImport(c *C) {
	src := ts.setupScanImportSource(c)
	tree := ts.scanForImport(c, src, "scan-video=1&exif-time=1")

	ts.h.scanImportResult.start(ts.bob.ID, 1)
	defer ts.h.scanImportResult.finish()

	content, err := json.Marshal(tree)
	c.Assert(err, IsNil)
	body, err := ts.request("/assets/scan/import/busy?token="+ts.token, http.MethodPost,
		http.StatusServiceUnavailable, bytes.NewReader(content), nil)
	c.Assert(err, IsNil)
	body.Close()
	// alice sees that bob's import is running, but none of its details
	st := ts.scanImportStatus(c)
	c.Assert(st.Import.Running, Equals, true)
	c.Assert(st.Import.Total, Equals, 0)
}

func (ts *mainSuite) TestScanWithoutExifTimeUsesFileTime(c *C) {
	src := ts.setupScanImportSource(c)

	tree := ts.scanForImport(c, src, "scan-video=1")
	files := scannedFiles(tree)
	c.Assert(files, HasLen, len(scanImportFixtures))
	// falls back to the file's birth time where the filesystem has one (the
	// copy just made), else its mtime -- either way not the EXIF date
	birth := time.Now().UTC()
	for _, f := range files {
		t := f.CreateTime.UTC()
		c.Assert(sameDay(t, scanImportFSTime) || sameDay(t, birth), Equals, true,
			Commentf("%s got %s", f.Name, f.CreateTime))
		c.Assert(t.Year(), Not(Equals), 2003)
	}
}

func (ts *mainSuite) TestScanStatusRequiresLogin(c *C) {
	body, err := ts.request("/assets/scan/status", http.MethodGet, http.StatusUnauthorized, nil, nil)
	c.Assert(err, IsNil)
	body.Close()
}

func (ts *mainSuite) TestScanRejectsMissingFolder(c *C) {
	missing := filepath.Join(os.TempDir(), "lomo-scan-import-does-not-exist")
	c.Assert(os.RemoveAll(missing), IsNil)
	body, err := ts.request("/assets/scan?token="+ts.token+"&path="+url.QueryEscape(missing), http.MethodPost,
		http.StatusInternalServerError, nil, nil)
	c.Assert(err, IsNil)
	body.Close()
	c.Assert(ts.scanImportStatus(c).Scan.Running, Equals, false)
}

func (ts *mainSuite) TestScanBrowseListsRoots(c *C) {
	src := ts.setupScanImportSource(c)
	body, err := ts.request("/assets/scan/browse?token="+ts.token+"&path="+url.QueryEscape(src), http.MethodGet,
		http.StatusOK, nil, nil)
	c.Assert(err, IsNil)
	defer body.Close()
	res := browseDirResult{}
	c.Assert(json.NewDecoder(body).Decode(&res), IsNil)
	c.Assert(res.Dirs, DeepEquals, []string{"trip"})
	// the root src itself lives under must be offered, so a picker can always get back to it
	c.Assert(len(res.Roots) > 0, Equals, true)
	found := false
	for _, r := range res.Roots {
		if strings.HasPrefix(strings.ToLower(src), strings.ToLower(r)) {
			found = true
		}
	}
	c.Assert(found, Equals, true, Commentf("roots %v don't cover %s", res.Roots, src))
}

func (ts *mainSuite) TestScanImportRefusesOtherUsersFiles(c *C) {
	src := ts.setupScanImportSource(c)
	tree := ts.scanForImport(c, src, "scan-video=1&exif-time=1")

	// the same files, but inside bob's library: a client posting a tree that
	// points there must not get them linked or copied into alice's library
	var bobHome string
	c.Assert(dbx.InQuery(ts.h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		bobHome, err = user.GetHomedir(ctx, tx, ts.bob.ID)
		return err
	}), IsNil)
	bobSrc := filepath.Join(bobHome, "private")
	c.Assert(os.RemoveAll(bobSrc), IsNil)
	for _, f := range scanImportFixtures {
		dst := filepath.Join(bobSrc, f.rel)
		c.Assert(os.MkdirAll(filepath.Dir(dst), 0755), IsNil)
		c.Assert(common.CopyFile(f.src, dst), IsNil)
	}
	tree.Name = bobSrc

	for _, params := range []string{"copy=1", ""} {
		res := ts.importScanned(c, "other-user-"+params, tree, params)
		c.Assert(res.Imported, Equals, 0, Commentf("params %q", params))
		c.Assert(res.Failures, HasLen, len(scanImportFixtures))
		for _, f := range res.Failures {
			c.Assert(f.Reason, Equals, errNotImportable.Error())
		}
	}
	for _, f := range scanImportFixtures {
		c.Assert(dirFileCount(c, ts.masterDayDir(c, f.exifDate)), Equals, 0)
	}
}

func (ts *mainSuite) TestIsUnderFolder(c *C) {
	root := filepath.Join(os.TempDir(), "lib")
	c.Assert(isUnderFolder(root, root), Equals, true)
	c.Assert(isUnderFolder(filepath.Join(root, "a", "b.jpg"), root), Equals, true)
	c.Assert(isUnderFolder(filepath.Join(root, "a", "..", "..", "x.jpg"), root), Equals, false)
	c.Assert(isUnderFolder(root+"2", root), Equals, false)
	c.Assert(isUnderFolder(filepath.Join(root+"2", "x.jpg"), root), Equals, false)
	c.Assert(isUnderFolder(filepath.Join(os.TempDir(), "..x"), root), Equals, false)
	c.Assert(isUnderFolder(root, ""), Equals, false)
}

func (ts *mainSuite) TestImportResultTryStart(c *C) {
	ir := &importResult{}
	c.Assert(ir.tryStart(1, 3), Equals, true)
	c.Assert(ir.tryStart(2, 5), Equals, false)
	c.Assert(ir.total, Equals, 3)
	c.Assert(ir.userID, Equals, 1)
	ir.finish()
	c.Assert(ir.tryStart(2, 5), Equals, true)
	c.Assert(ir.total, Equals, 5)
	c.Assert(ir.userID, Equals, 2)
}
