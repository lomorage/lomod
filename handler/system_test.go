package handler

import (
	"archive/zip"
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"runtime"

	"bytes"
	"time"

	"os"
	"path/filepath"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/testutil"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/sirupsen/logrus"
	. "gopkg.in/check.v1"
)

func (ts *mainSuite) setbackup(c *C, username, dir string) {
	bak := &types.BackupCreateRequest{Username: username, DestDisk: dir}
	buf, err := json.Marshal(bak)
	c.Assert(err, IsNil)

	req, err := http.NewRequest("POST", "/system/backup?token="+ts.token, bytes.NewBuffer(buf))
	c.Assert(err, IsNil)
	req.Header.Set("Content-Type", "application/octet-stream")

	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	validateStatusCode(c, "/system/backup", rr, http.StatusOK)
}

func (ts *mainSuite) removeBackup(c *C, username string) {
	req, err := http.NewRequest("DELETE", "/system/backup?token="+ts.token, nil)
	c.Assert(err, IsNil)

	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	validateStatusCode(c, "/system/backup", rr, http.StatusOK)
}

// test not only backup, but also preview generation.
func (ts *mainSuite) TestMaintenanceBasic(c *C) {
	ts.resetDB(c, ts.h.conf.DbFilename, true)
	ts.h.conf.UseJpg = true
	defer func() {
		ts.h.conf.UseJpg = false
	}()

	category, err := loadCategory()
	c.Assert(err, IsNil)

	ts.createAssets(c, &category, ts.tokenBob)
	ts.waitPreviewComplete(c, true)

	c.Assert(os.RemoveAll(backupdir), IsNil)
	ts.setbackup(c, "alice", backupdir)
	ts.setbackup(c, "bob", backupdir)

	ts.h.lastBackup = make(map[string]types.BackupResult)
	ch := make(chan struct{})
	defer close(ch)
	go ts.h.maintenance(time.Now().Hour(), time.Now().Minute(), time.Now().Second()+1, 5*time.Second, ch, false)

	for {
		if ts.h.lastMaintStartTime.Before(ts.h.lastMaintEndTime) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// start validation
	ts.validateBackupDB(c, backupdir)
	ts.validateBackupDir(c, backupdir, true)
	ts.validateSystemInfoAPI(c, true)

	// remove some files from destination, then check if backup can recover
	// also remove preview directory and check preview assets
	c.Assert(os.RemoveAll(filepath.Join(backupdir, "alice/Photos/master/2003/01")), IsNil)
	c.Assert(os.RemoveAll(filepath.Join(backupdir, "alice/Photos/master/2004")), IsNil)
	c.Assert(os.RemoveAll(filepath.Join(backupdir, "bob/Photos/master/2013")), IsNil)

	c.Assert(os.RemoveAll(filepath.Join(photodir, "alice/Photos/preview")), IsNil)

	ts.h.lastMaintStartTime = time.Time{}
	ts.h.lastMaintEndTime = time.Time{}
	for {
		if ts.h.lastMaintStartTime.Before(ts.h.lastMaintEndTime) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	ts.validateBackupDB(c, backupdir)
	ts.validateBackupDir(c, backupdir, true)
	ts.validateTrashDir(c, backupdir, true)

	ts.waitPreviewComplete(c, true) // preview is async generated
	previewFileSizes := ts.getPreviewFileSize(c)

	ts.validateSystemInfoAPI(c, true)

	// remove some files from source, then check if backup can delete as well, and trashbox can have it or not
	// delete one file by delete api and another one by deleting folder
	// 2003-01-17
	ts.requestDelete(c, ts.token, &types.DeleteAssetItems{
		List: []types.DeleteAssetItem{{ID: "4"}},
	})

	// 2004-01-21
	c.Assert(os.RemoveAll(filepath.Join(photodir, "alice/Photos/master/2004")), IsNil)
	// 2013-11-23
	ts.requestDelete(c, ts.tokenBob, &types.DeleteAssetItems{
		List: []types.DeleteAssetItem{{ID: "9"}},
	})

	ts.requestDelete(c, ts.tokenBob, &types.DeleteAssetItems{
		List: []types.DeleteAssetItem{{ID: "10"}},
	})

	// truncated one preview file
	ts.truncatedPreviews(c)

	ts.h.lastMaintStartTime = time.Time{}
	ts.h.lastMaintEndTime = time.Time{}
	for {
		if ts.h.lastMaintStartTime.Before(ts.h.lastMaintEndTime) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	ts.validateBackupDir(c, backupdir, false)
	ts.validateTrashDir(c, backupdir, false)

	ts.waitTruncatedPreviews(c)

	previewFileSizesObtain := ts.getPreviewFileSize(c)
	c.Assert(previewFileSizesObtain, DeepEquals, previewFileSizes)

	ts.validateSystemInfoAPI(c, true)
}

func (ts *mainSuite) validateSystemInfoAPI(c *C, checkBackup bool) {
	result := ts.listSystemInfo(c, "/system?token="+ts.token)
	c.Assert(result.SystemStatus, Equals, common.SystemStatusInited)
	c.Assert(result.OS, Equals, runtime.GOOS)
	c.Assert(result.UUID, Equals, ts.h.uuid)
	c.Assert(result.APIVersion, Equals, "1.1")
	c.Assert(result.LomodVersion, Equals, "<replace me>")
	c.Assert(result.NetworkStatus, Equals, "ok")
	tz, tzOffset := time.Now().Zone()
	c.Assert(result.TimezoneName, Equals, tz, Commentf("%+v", result))
	c.Assert(result.TimezoneOffset, Equals, tzOffset/3600)
	c.Assert(len(result.UserDisks), Equals, 2)

	if !checkBackup {
		return
	}
	c.Assert(len(result.LastBackup), Equals, 2)

	backup, ok := result.LastBackup["alice"]
	c.Assert(ok, Equals, true)
	c.Assert(backup.DBRetCode, IsNil)
	c.Assert(backup.LastDBBackup, IsNil)
	c.Assert(backup.LastDBSuccess, IsNil)
	c.Assert(backup.AssetRetCode, Equals, "")
	c.Assert(backup.LastAssetBackupBegin.After(time.Now().Add(-10*time.Second)), Equals, true)
	c.Assert(backup.LastAssetBackupEnd.After(time.Now().Add(-10*time.Second)), Equals, true)

	backup, ok = result.LastBackup["bob"]
	c.Assert(ok, Equals, true)
	c.Assert(backup.DBRetCode, IsNil)
	c.Assert(backup.LastDBBackup, IsNil)
	c.Assert(backup.LastDBSuccess, IsNil)
	c.Assert(backup.AssetRetCode, Equals, "")
	c.Assert(backup.LastAssetBackupBegin.After(time.Now().Add(-10*time.Second)), Equals, true)
	c.Assert(backup.LastAssetBackupEnd.After(time.Now().Add(-10*time.Second)), Equals, true)
}

func (ts *mainSuite) validateBackupDB(c *C, backupDir string) {
	// db should be backed up too
	dstDirs := []string{ts.alice.HomeDir, ts.bob.HomeDir, filepath.Join(backupDir, "alice"),
		filepath.Join(backupDir, "bob")}

	f, err := os.Open(ts.h.conf.DbFilename)
	c.Assert(err, IsNil)
	defer f.Close()

	h := md5.New()
	_, err = io.Copy(h, f)
	c.Assert(err, IsNil)

	srcMD5 := fmt.Sprintf("%x", h.Sum(nil))

	for _, dstDir := range dstDirs {
		f2, err := os.Open(filepath.Join(dstDir, "assets.db"))
		c.Assert(err, IsNil)
		defer f2.Close()

		h2 := md5.New()
		_, err = io.Copy(h2, f2)
		c.Assert(err, IsNil)

		c.Assert(fmt.Sprintf("%x", h2.Sum(nil)), Equals, srcMD5)
	}
}

func (ts *mainSuite) validateBackupDir(c *C, photodir string, beforeDelete bool) {
	start := 1
	ts.validateAsset(c, fmt.Sprintf("%s/alice/Photos/master/2003/11/23/20031123_%d.jpg", photodir, start), 80603)
	ts.validateAsset(c, fmt.Sprintf("%s/bob/Photos/master/2003/11/01/20031101_%d.jpg", photodir, start+1), 70513)
	ts.validateAsset(c, fmt.Sprintf("%s/bob/Photos/master/2003/11/01/20031101_%d.jpg", photodir, start+2), 247759)
	if beforeDelete {
		ts.validateAsset(c, fmt.Sprintf("%s/alice/Photos/master/2003/01/17/20030117_%d.jpg", photodir, start+3), 231635)
		ts.validateAsset(c, fmt.Sprintf("%s/alice/Photos/master/2004/01/21/20040121_%d.jpg", photodir, start+4), 1125776)
		// DISCUSS: removed --delete from rsync to make sure no photos are deleted at backup drive
		//} else {
		//ts.validateAsset(c, fmt.Sprintf("%s/alice/Photos/master/2003/01/17/20030117_%d.jpg", photodir, start+3), -1)
		//ts.validateAsset(c, fmt.Sprintf("%s/alice/Photos/master/2004/01/21/20040121_%d.jpg", photodir, start+4), -1)
	}
	ts.validateAsset(c, fmt.Sprintf("%s/alice/Photos/master/2013/08/08/20130808_%d.mp4", photodir, start+5), 7360)
	ts.validateAsset(c, fmt.Sprintf("%s/alice/Photos/master/2013/07/28/20130728_%d.png", photodir, start+6), 44969)
	ts.validateAsset(c, fmt.Sprintf("%s/alice/Photos/master/2013/11/23/20131123_%d_image.jpg", photodir, start+7), 231635)
	ts.validateAsset(c, fmt.Sprintf("%s/alice/Photos/master/2013/11/23/20131123_%d.zip", photodir, start+7), 238254)
	//if beforeDelete {
	ts.validateAsset(c, fmt.Sprintf("%s/bob/Photos/master/2013/11/23/20131123_%d.heic", photodir, start+8), 1051433)
	ts.validateAsset(c, fmt.Sprintf("%s/bob/Photos/master/2013/11/23/20131123_%d_image.heic", photodir, start+9), 1051433)
	ts.validateAsset(c, fmt.Sprintf("%s/bob/Photos/master/2013/11/23/20131123_%d.zip", photodir, start+9), 1055380)
	//} else {
	//	ts.validateAsset(c, fmt.Sprintf("%s/bob/Photos/master/2013/11/23/20131123_%d.heic", photodir, start+8), -1)
	//	ts.validateAsset(c, fmt.Sprintf("%s/bob/Photos/master/2013/11/23/20131123_%d_image.heic", photodir, start+9), -1)
	//	ts.validateAsset(c, fmt.Sprintf("%s/bob/Photos/master/2013/11/23/20131123_%d.zip", photodir, start+9), -1)
	//}

	//if beforeDelete {
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/", photodir), 3, true)
	//} else {
	//ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/", photodir), 2, true)
	//}
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2003", photodir), 2, true)
	//if beforeDelete {
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2004", photodir), 1, true)
	//}
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2013", photodir), 3, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2003/11", photodir), 1, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2003/11/23", photodir), 0, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2003/11/23", photodir), 1, false)
	if beforeDelete {
		ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2003/01", photodir), 1, true)
		ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2003/01/17", photodir), 0, true)
		ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2003/01/17", photodir), 1, false)
		ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2004", photodir), 1, true)
		ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2004/01", photodir), 1, true)
		ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2004/01/21", photodir), 0, true)
		ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2004/01/21", photodir), 1, false)
	}
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2013", photodir), 3, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2013/08", photodir), 1, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2013/08/08", photodir), 0, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2013/08/08", photodir), 1, false)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2013/07", photodir), 1, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2013/07/28", photodir), 0, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2013/07/28", photodir), 1, false)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2013/11", photodir), 1, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2013/11/23", photodir), 0, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2013/11/23", photodir), 2, false)

	if beforeDelete {
		ts.validateDir(c, fmt.Sprintf("%s/bob/Photos/master/", photodir), 2, true)
		//} else {
		//	ts.validateDir(c, fmt.Sprintf("%s/bob/Photos/master/", photodir), 2, true)
	}

	ts.validateDir(c, fmt.Sprintf("%s/bob/Photos/master/", photodir), 0, false)
	ts.validateDir(c, fmt.Sprintf("%s/bob/Photos/master/2003", photodir), 1, true)
	ts.validateDir(c, fmt.Sprintf("%s/bob/Photos/master/2003", photodir), 0, false)
	ts.validateDir(c, fmt.Sprintf("%s/bob/Photos/master/2003/11", photodir), 1, true)
	ts.validateDir(c, fmt.Sprintf("%s/bob/Photos/master/2003/11", photodir), 0, false)
	ts.validateDir(c, fmt.Sprintf("%s/bob/Photos/master/2003/11/01", photodir), 0, true)
	ts.validateDir(c, fmt.Sprintf("%s/bob/Photos/master/2003/11/01", photodir), 2, false)
	if beforeDelete {
		ts.validateDir(c, fmt.Sprintf("%s/bob/Photos/master/2013", photodir), 1, true)
		ts.validateDir(c, fmt.Sprintf("%s/bob/Photos/master/2013", photodir), 0, false)
		ts.validateDir(c, fmt.Sprintf("%s/bob/Photos/master/2013/11", photodir), 1, true)
		ts.validateDir(c, fmt.Sprintf("%s/bob/Photos/master/2013/11", photodir), 0, false)
		ts.validateDir(c, fmt.Sprintf("%s/bob/Photos/master/2013/11/23", photodir), 0, true)
		ts.validateDir(c, fmt.Sprintf("%s/bob/Photos/master/2013/11/23", photodir), 3, false)
	}
}

func (ts *mainSuite) validateTrashDir(c *C, photodir string, beforeDelete bool) {
	start := 1
	if beforeDelete {
		_, err := os.Stat(fmt.Sprintf("%s/alice/Photos/%s/", photodir, common.AppPhotoTrashDir))
		c.Assert(os.IsNotExist(err), Equals, true)
		_, err = os.Stat(fmt.Sprintf("%s/bob/Photos/%s/", photodir, common.AppPhotoTrashDir))
		c.Assert(os.IsNotExist(err), Equals, true)
		return
	}
	ts.validateAsset(c, fmt.Sprintf("%s/alice/Photos/%s/2003/01/17/20030117_%d.jpg", photodir, common.AppPhotoTrashDir, start+3), 231635)
	ts.validateAsset(c, fmt.Sprintf("%s/bob/Photos/%s/2013/11/23/20131123_%d.heic", photodir, common.AppPhotoTrashDir, start+8), 1051433)
	ts.validateAsset(c, fmt.Sprintf("%s/bob/Photos/%s/2013/11/23/20131123_%d.zip", photodir, common.AppPhotoTrashDir, start+9), 1055380)

	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/%s/", photodir, common.AppPhotoTrashDir), 1, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/%s/2003", photodir, common.AppPhotoTrashDir), 1, true)

	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/%s/2003/01", photodir, common.AppPhotoTrashDir), 1, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/%s/2003/01/17", photodir, common.AppPhotoTrashDir), 0, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/%s/2003/01/17", photodir, common.AppPhotoTrashDir), 1, false)

	ts.validateDir(c, fmt.Sprintf("%s/bob/Photos/%s/", photodir, common.AppPhotoTrashDir), 1, true)
	ts.validateDir(c, fmt.Sprintf("%s/bob/Photos/%s/2013", photodir, common.AppPhotoTrashDir), 1, true)
	ts.validateDir(c, fmt.Sprintf("%s/bob/Photos/%s/2013", photodir, common.AppPhotoTrashDir), 0, false)
	ts.validateDir(c, fmt.Sprintf("%s/bob/Photos/%s/2013/11", photodir, common.AppPhotoTrashDir), 1, true)
	ts.validateDir(c, fmt.Sprintf("%s/bob/Photos/%s/2013/11", photodir, common.AppPhotoTrashDir), 0, false)
	ts.validateDir(c, fmt.Sprintf("%s/bob/Photos/%s/2013/11/23", photodir, common.AppPhotoTrashDir), 0, true)
	ts.validateDir(c, fmt.Sprintf("%s/bob/Photos/%s/2013/11/23", photodir, common.AppPhotoTrashDir), 2, false)
}

func (ts *mainSuite) getPreviewFileSize(c *C) map[string]int {
	files := map[string]int{
		fmt.Sprintf("%s/alice/Photos/preview/2003/11/23/20031123_1_75_75.jpg", photodir):   0,
		fmt.Sprintf("%s/bob/Photos/preview/2003/11/01/20031101_2_75_75.jpg", photodir):     0,
		fmt.Sprintf("%s/bob/Photos/preview/2003/11/01/20031101_3_75_75.jpg", photodir):     0,
		fmt.Sprintf("%s/alice/Photos/preview/2013/08/08/20130808_6_75_75.jpg", photodir):   0,
		fmt.Sprintf("%s/alice/Photos/preview/2013/07/28/20130728_7_75_75.png", photodir):   0,
		fmt.Sprintf("%s/alice/Photos/preview/2013/11/23/20131123_8_75_75.jpg", photodir):   0,
		fmt.Sprintf("%s/alice/Photos/preview/2003/11/23/20031123_1_480_320.jpg", photodir): 0,
		fmt.Sprintf("%s/bob/Photos/preview/2003/11/01/20031101_2_480_320.jpg", photodir):   0,
		fmt.Sprintf("%s/bob/Photos/preview/2003/11/01/20031101_3_480_320.jpg", photodir):   0,
		fmt.Sprintf("%s/alice/Photos/preview/2013/08/08/20130808_6_480_320.jpg", photodir): 0,
		fmt.Sprintf("%s/alice/Photos/preview/2013/07/28/20130728_7_480_320.png", photodir): 0,
		fmt.Sprintf("%s/alice/Photos/preview/2013/11/23/20131123_8_480_320.jpg", photodir): 0,
		fmt.Sprintf("%s/alice/Photos/preview/2013/08/08/20130808_6_320_0.mp4", photodir):   0,
	}

	for k := range files {
		info, err := os.Stat(k)
		c.Assert(err, IsNil)
		files[k] = int(info.Size())
		c.Assert(info.Size(), Not(Equals), 0, Commentf(k))
	}

	return files
}

func (ts *mainSuite) truncatedPreviews(c *C) {
	c.Assert(ioutil.WriteFile(fmt.Sprintf("%s/alice/Photos/preview/2003/11/23/20031123_1_75_75.jpg", photodir), []byte{}, 0666), IsNil)
	c.Assert(ioutil.WriteFile(fmt.Sprintf("%s/alice/Photos/preview/2013/08/08/20130808_6_75_75.jpg", photodir), []byte{}, 0666), IsNil)
	c.Assert(ioutil.WriteFile(fmt.Sprintf("%s/alice/Photos/preview/2013/07/28/20130728_7_75_75.png", photodir), []byte{}, 0666), IsNil)
	c.Assert(ioutil.WriteFile(fmt.Sprintf("%s/alice/Photos/preview/2013/11/23/20131123_8_75_75.jpg", photodir), []byte{}, 0666), IsNil)

	c.Assert(ioutil.WriteFile(fmt.Sprintf("%s/alice/Photos/preview/2003/11/23/20031123_1_480_320.jpg", photodir), []byte{}, 0666), IsNil)
	c.Assert(ioutil.WriteFile(fmt.Sprintf("%s/alice/Photos/preview/2013/08/08/20130808_6_480_320.jpg", photodir), []byte{}, 0666), IsNil)
	c.Assert(ioutil.WriteFile(fmt.Sprintf("%s/alice/Photos/preview/2013/07/28/20130728_7_480_320.png", photodir), []byte{}, 0666), IsNil)
	c.Assert(ioutil.WriteFile(fmt.Sprintf("%s/alice/Photos/preview/2013/11/23/20131123_8_480_320.jpg", photodir), []byte{}, 0666), IsNil)
	c.Assert(ioutil.WriteFile(fmt.Sprintf("%s/alice/Photos/preview/2013/08/08/20130808_6_320_0.mp4", photodir), []byte{}, 0666), IsNil)
}

func (ts *mainSuite) waitTruncatedPreviews(c *C) {
	for _, name := range []string{
		photodir + "/alice/Photos/preview/2003/11/23/20031123_1_75_75.jpg",
		photodir + "/alice/Photos/preview/2003/11/23/20031123_1_480_320.jpg",
		photodir + "/alice/Photos/preview/2013/07/28/20130728_7_75_75.png",
		photodir + "/alice/Photos/preview/2013/07/28/20130728_7_480_320.png",
		photodir + "/alice/Photos/preview/2013/08/08/20130808_6_480_320.jpg",
		photodir + "/alice/Photos/preview/2013/08/08/20130808_6_320_0.mp4",
		photodir + "/alice/Photos/preview/2013/08/08/20130808_6_75_75.jpg",
		photodir + "/alice/Photos/preview/2013/11/23/20131123_8_75_75.jpg",
		photodir + "/alice/Photos/preview/2013/11/23/20131123_8_480_320.jpg",
	} {
		done := false
		for i := 0; i < 60; i++ {
			stat, err := os.Stat(name)
			if err == nil && stat.Size() != 0 {
				done = true
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		c.Assert(done, Equals, true, Commentf("%s", name))
	}
}

func (ts *mainSuite) createLivePhotoWithComment(c *C) (string, string) {
	tmpfile, err := ioutil.TempFile("", "")
	c.Assert(err, IsNil)

	filename := "../cmd/lomod/test/liph/15_2017_09_13.zip"
	hash, err := types.GetLivePhotoFileSHAByContent(filename,
		func(name string, imgSHA hash.Hash) (io.Writer, error) { return imgSHA, nil })
	c.Assert(err, IsNil)

	r, err := zip.OpenReader(filename)
	c.Assert(err, IsNil)
	defer r.Close()

	w := zip.NewWriter(tmpfile)
	for _, f := range r.File {
		rc, err := f.Open()
		c.Assert(err, IsNil)

		wf, err := w.Create(f.Name)
		c.Assert(err, IsNil)
		_, err = io.Copy(wf, rc)
		c.Assert(err, IsNil)

		c.Assert(rc.Close(), IsNil)
	}

	data, err := json.Marshal(hash)
	c.Assert(err, IsNil)
	c.Assert(w.SetComment(string(data)), IsNil)
	c.Assert(w.Close(), IsNil)
	c.Assert(tmpfile.Close(), IsNil)

	return tmpfile.Name(), hash.TotalSHA1
}

func (ts *mainSuite) listMounts(c *C) []types.MountDir {
	req, err := http.NewRequest("GET", "/mount?token="+ts.token, nil)
	c.Assert(err, IsNil)

	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	validateStatusCode(c, "/mount", rr, http.StatusOK)

	res := rr.Result()
	defer res.Body.Close()

	dirs := []types.MountDir{}
	c.Assert(json.NewDecoder(res.Body).Decode(&dirs), IsNil)
	return dirs
}

func (ts *mainSuite) TestListMounts(c *C) {
	// remove backup dir firstly
	c.Assert(os.RemoveAll(backupdir), IsNil)

	dirs := ts.listMounts(c)
	mnts := map[string]types.MountDir{}
	for _, dir := range dirs {
		d := "/" + dir.Dir
		if d == mntdir || d == photodir || d == backupdir {
			mnts[d] = dir
		}
	}

	c.Assert(len(mnts), Equals, 2)
	dir, ok := mnts[mntdir]
	c.Assert(ok, Equals, true)
	c.Assert(dir.FreeSize > 0, Equals, true)
	c.Assert(dir.TotalSize > 0, Equals, true)

	dir, ok = mnts[photodir]
	c.Assert(ok, Equals, true)
	c.Assert(dir.FreeSize > 0, Equals, true)
	c.Assert(dir.TotalSize > 0, Equals, true)

	// create backup dir and validate again
	c.Assert(os.Mkdir(backupdir, 0755), IsNil)
	dirs = ts.listMounts(c)
	for _, dir := range dirs {
		d := "/" + dir.Dir
		if d == backupdir {
			mnts[d] = dir
		}
	}

	c.Assert(len(mnts), Equals, 3, Commentf("dirs: %v", dirs))
	dir, ok = mnts[mntdir]
	c.Assert(ok, Equals, true)
	c.Assert(dir.FreeSize > 0, Equals, true)
	c.Assert(dir.TotalSize > 0, Equals, true)

	dir, ok = mnts[photodir]
	c.Assert(ok, Equals, true)
	c.Assert(dir.FreeSize > 0, Equals, true)
	c.Assert(dir.TotalSize > 0, Equals, true)

	dir, ok = mnts[backupdir]
	c.Assert(ok, Equals, true)
	c.Assert(dir.FreeSize > 0, Equals, true)
	c.Assert(dir.TotalSize > 0, Equals, true)

	username := "David"
	c.Assert(ts.createUser(user.User{
		Name: username, HomeDir: filepath.Base(photodir), BackupDir: backupdir,
	}, ts.token, true), IsNil)

	_, err := os.Stat(filepath.Join(photodir, username))
	c.Assert(err, IsNil)
	_, err = os.Stat(filepath.Join(filepath.Join(photodir, username), common.AppPhoto))
	c.Assert(err, IsNil)
	_, err = os.Stat(filepath.Join(backupdir, username))
	c.Assert(err, IsNil)
	_, err = os.Stat(filepath.Join(filepath.Join(backupdir, username), common.AppPhoto))
	c.Assert(err, IsNil)

	username = "david"
	c.Assert(ts.createUser(user.User{
		Name: username, HomeDir: filepath.Base(photodir),
	}, ts.token, true), IsNil)

	_, err = os.Stat(filepath.Join(photodir, username))
	c.Assert(err, IsNil)
	_, err = os.Stat(filepath.Join(filepath.Join(photodir, username), common.AppPhoto))
	c.Assert(err, IsNil)
	_, err = os.Stat(filepath.Join(backupdir, username))
	c.Assert(err, NotNil)
	c.Assert(os.IsNotExist(err), Equals, true)

	ts.setbackup(c, username, mnts[backupdir].Dir)
	_, err = os.Stat(filepath.Join(backupdir, username))
	c.Assert(err, IsNil)
	_, err = os.Stat(filepath.Join(filepath.Join(backupdir, username), common.AppPhoto))
	c.Assert(err, IsNil)
}

func (ts *mainSuite) listSystemInfo(c *C, u string) types.SystemInfo {
	req, err := http.NewRequest("GET", u, nil)
	c.Assert(err, IsNil)

	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	validateStatusCode(c, u, rr, http.StatusOK)

	res := rr.Result()
	defer res.Body.Close()

	info := types.SystemInfo{}
	c.Assert(json.NewDecoder(res.Body).Decode(&info), IsNil)
	return info
}

func (ts *mainSuite) TestMaintenanceBackup(c *C) {
	ts.resetDB(c, ts.h.conf.DbFilename, true)

	category, err := loadCategory()
	c.Assert(err, IsNil)

	ts.createAssets(c, &category, ts.tokenBob)
	ts.waitPreviewComplete(c, true)

	c.Assert(os.RemoveAll(backupdir), IsNil)
	ts.setbackup(c, "alice", backupdir)
	ts.setbackup(c, "bob", backupdir)

	ts.h.conf.CheckLeftDays = 100
	ts.h.lastBackup = make(map[string]types.BackupResult)
	ch := make(chan struct{})
	defer close(ch)
	// use 1 hour to block regular maintenance
	go ts.h.maintenance(time.Now().Hour()+1, 0, 0, time.Hour, ch, true)

	// remove some files from destination, then check if backup can recover
	// also remove preview directory and check preview assets
	c.Assert(os.RemoveAll(filepath.Join(backupdir, "alice/Photos/master/2003/01")), IsNil)
	c.Assert(os.RemoveAll(filepath.Join(backupdir, "alice/Photos/master/2004")), IsNil)
	c.Assert(os.RemoveAll(filepath.Join(backupdir, "bob/Photos/master/2013")), IsNil)

	c.Assert(os.RemoveAll(filepath.Join(photodir, "alice/Photos/preview")), IsNil)

	ts.requestWithMethod(c, "/system/backup?token="+ts.token, "PUT", http.StatusOK, nil, nil, nil)

	time.Sleep(5 * time.Second)

	ts.validateBackupDB(c, backupdir)
	ts.validateBackupDir(c, backupdir, true)
	ts.validateTrashDir(c, backupdir, true)

	ts.validateSystemInfoAPI(c, false)

	// remove alice's backup directory, it should no backup anymore, but bob should not be impacted
	ts.removeBackup(c, "alice")
	c.Assert(os.RemoveAll(backupdir), IsNil)

	ts.requestWithMethod(c, "/system/backup?token="+ts.token, "PUT", http.StatusOK, nil, nil, nil)

	time.Sleep(5 * time.Second)

	ts.validateAsset(c, fmt.Sprintf("%s/bob/Photos/master/2003/11/01/20031101_%d.jpg", backupdir, 2), 70513)

	_, err = os.Stat(filepath.Join(backupdir, "alice"))
	c.Assert(err, NotNil)
	c.Assert(os.IsNotExist(err), Equals, true)
}

func (ts *mainSuite) removeAllAssets(dir string) error {
	return filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		fmt.Printf("---- remove: %s\n", p)
		return os.Remove(p)
	})
}

func (ts *mainSuite) TestMaintenanceMasterAssetDeletedFromFSDirWithClean(c *C) {
	ts.h.conf.DbClean = true
	defer func() {
		ts.h.conf.DbClean = false
	}()

	// this is to test all assets are deleted from file system, and maintenance should remove from DB
	ts.resetDB(c, ts.h.conf.DbFilename, true)

	ts.insertAssetsWithGPS(c)

	dir := photodir + "/alice/Photos"
	c.Assert(testutil.WaitWithFileCounts(context.Background(), dir, 6), IsNil)

	c.Assert(ts.removeAllAssets(dir), IsNil)

	// disable ccheck for now
	ts.h.conf.CheckLeftDays = 100
	ch := make(chan struct{})
	defer func() {
		close(ch)
		ts.h.conf.CheckLeftDays = 0
	}()

	go ts.h.maintenance(time.Now().Hour(), time.Now().Minute(), time.Now().Second()+1, 2*time.Second, ch, true)

	for {
		if ts.h.lastMaintStartTime.Before(ts.h.lastMaintEndTime) {
			break
		}
		logrus.Info("------ maintenance not finish yet")
		time.Sleep(50 * time.Millisecond)
	}
	logrus.Info("------ maintenance finish")

	// category should return empty
	category := types.Years{}
	ts.validateBasicCategory(c, category)

	for _, a := range gpsAssets {
		ts.requestWithMethod(c, fmt.Sprintf("/asset/%s?token=%s", a.hash, ts.token), http.MethodHead,
			http.StatusNotFound, nil, nil, nil)
	}
}

func (ts *mainSuite) TestMaintenanceMasterAssetDeletedFromFSDirWithoutCleanRemoveAssetOnly(c *C) {
	// this is to test all assets are deleted from file system, and maintenance should remove from DB
	ts.resetDB(c, ts.h.conf.DbFilename, true)

	ts.insertAssetsWithGPS(c)

	dir := photodir + "/alice/Photos"
	c.Assert(testutil.WaitWithFileCounts(context.Background(), dir, 6), IsNil)

	c.Assert(ts.removeAllAssets(dir), IsNil)

	// disable ccheck for now
	ts.h.conf.CheckLeftDays = 100
	ch := make(chan struct{})
	defer func() {
		close(ch)
		ts.h.conf.CheckLeftDays = 0
	}()

	go ts.h.maintenance(time.Now().Hour(), time.Now().Minute(), time.Now().Second()+1, 2*time.Second, ch, true)

	for {
		if ts.h.lastMaintStartTime.Before(ts.h.lastMaintEndTime) {
			break
		}
		logrus.Info("------ maintenance not finish yet")
		time.Sleep(50 * time.Millisecond)
	}
	logrus.Info("------ maintenance finish")

	// category should return empty
	ts.validateBasicCategory(c, gpsCategory)

	for _, a := range gpsAssets {
		ts.requestWithMethod(c, fmt.Sprintf("/asset/%s?token=%s", a.hash, ts.token), http.MethodHead,
			http.StatusOK, nil, nil, nil)
	}
}

func (ts *mainSuite) TestMaintenanceMasterAssetDeletedFromFSDirWithoutCleanRemoveAll(c *C) {
	ts.h.conf.DbClean = true
	defer func() {
		ts.h.conf.DbClean = false
	}()

	// this is to test all assets are deleted from file system, and maintenance should remove from DB
	ts.resetDB(c, ts.h.conf.DbFilename, true)

	ts.insertAssetsWithGPS(c)

	dir := photodir + "/alice/Photos"
	c.Assert(testutil.WaitWithFileCounts(context.Background(), dir, 6), IsNil)

	// remove directory should not do db clean up
	c.Assert(os.RemoveAll(dir), IsNil)

	// disable ccheck for now
	ts.h.conf.CheckLeftDays = 100
	ch := make(chan struct{})
	defer func() {
		close(ch)
		ts.h.conf.CheckLeftDays = 0
	}()

	go ts.h.maintenance(time.Now().Hour(), time.Now().Minute(), time.Now().Second()+1, 2*time.Second, ch, true)

	for {
		if ts.h.lastMaintStartTime.Before(ts.h.lastMaintEndTime) {
			break
		}
		logrus.Info("------ maintenance not finish yet")
		time.Sleep(50 * time.Millisecond)
	}
	logrus.Info("------ maintenance finish")

	// category should not return empty
	ts.validateBasicCategory(c, gpsCategory)

	for _, a := range gpsAssets {
		ts.requestWithMethod(c, fmt.Sprintf("/asset/%s?token=%s", a.hash, ts.token), http.MethodHead,
			http.StatusOK, nil, nil, nil)
	}
}

func (ts *mainSuite) TestMaintenanceMasterAssetDeletedFromFSDirWithCleanLivePhoto(c *C) {
	ts.h.conf.DbClean = true
	defer func() {
		ts.h.conf.DbClean = false
	}()

	// this is to test all assets are deleted from file system, and maintenance should remove from DB
	ts.resetDB(c, ts.h.conf.DbFilename, true)

	ts.createLivephotos(c)

	dir := photodir + "/alice/Photos"
	c.Assert(testutil.WaitWithFileCounts(context.Background(), dir, 8), IsNil)

	c.Assert(ts.removeAllAssets(dir), IsNil)

	// disable ccheck for now
	ts.h.conf.CheckLeftDays = 100
	ch := make(chan struct{})
	defer func() {
		close(ch)
		ts.h.conf.CheckLeftDays = 0
	}()

	go ts.h.maintenance(time.Now().Hour(), time.Now().Minute(), time.Now().Second()+1, 2*time.Second, ch, true)

	for {
		if ts.h.lastMaintStartTime.Before(ts.h.lastMaintEndTime) {
			break
		}
		logrus.Info("------ maintenance not finish yet")
		time.Sleep(50 * time.Millisecond)
	}
	logrus.Info("------ maintenance finish")

	// category should return empty
	category := types.Years{}
	ts.validateBasicCategory(c, category)

	for _, h := range livePhotoHashes {
		ts.requestWithMethod(c, fmt.Sprintf("/asset/%s?token=%s", h, ts.token), http.MethodHead,
			http.StatusNotFound, nil, nil, nil)
	}
}
