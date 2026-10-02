package handler

import (
	"context"
	"database/sql"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	. "gopkg.in/check.v1"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/testutil"
	"github.com/sirupsen/logrus"
)

const bobCheck = `--------------- bob ---------------
General check is good
LivePhoto ZIP file check is good
LivePhoto Image file check is good
LivePhoto Image file duplication check is good
LivePhoto Image file zero size check is good
Asset file zero size check is good
Asset in file system check is good
Asset in database check is good
Asset create date in file system check is good
Asset extension check is good
Asset location check is good
Asset file hash duplication check is good
User directory in file system check is good
User in database check is good
Preview files in file system check is good
Master directory in file system check is good
Preview directory in file system check is good
`

func (ts *mainSuite) TestInconsistentCheckGeneral(c *C) {
	category, err := loadCategory()
	c.Assert(err, IsNil)
	ts.createAssets(c, &category, ts.token)

	ch := make(chan struct{})
	defer close(ch)
	go ts.h.maintenance(time.Now().Hour()+1, 0, 0, time.Hour, ch, true)

	// wait until all previews are generated
	ts.waitPreviewComplete(c, false)

	// simulate	AssetZeroSize
	filename := photodir + "/alice/Photos/master/2003/11/23/20031123_1.jpg"
	f, err := os.Create(filename)
	c.Assert(err, IsNil)
	c.Assert(f.Close(), IsNil)

	dt, err := time.Parse(common.TimeFormatLomod, "2003-11-23T20:00:00Z")
	c.Assert(err, IsNil)
	c.Assert(os.Chtimes(filename, dt, dt), IsNil)

	// simulate	AssetMissInDB
	c.Assert(dbx.InQuery(ts.h.db, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "delete from asset where id = 3")
		return err
	}), IsNil)
	// simulate	AssetDateDiff
	c.Assert(os.Mkdir(photodir+"/alice/Photos/master/2003/01/16", common.DefaultFolderPermission), IsNil)
	filename = photodir + "/alice/Photos/master/2003/01/16/20030116_4.jpg"
	c.Assert(os.Rename(photodir+"/alice/Photos/master/2003/01/17/20030117_4.jpg", filename), IsNil)

	dt, err = time.Parse(common.TimeFormatLomod, "2003-01-17T20:00:00Z")
	c.Assert(err, IsNil)
	c.Assert(os.Chtimes(filename, dt, dt), IsNil)

	// simulate	AssetExtDiff
	filename = photodir + "/alice/Photos/master/2004/01/21/20040121_5.jpeg"
	c.Assert(os.Rename(photodir+"/alice/Photos/master/2004/01/21/20040121_5.jpg", filename), IsNil)

	dt, err = time.Parse(common.TimeFormatLomod, "2004-01-21T20:00:00Z")
	c.Assert(err, IsNil)
	c.Assert(os.Chtimes(filename, dt, dt), IsNil)

	// simulate	AssetHashDuplicate
	filename = photodir + "/alice/Photos/master/2013/07/28/20130728_7.png"
	f, err = os.Create(filename)
	c.Assert(err, IsNil)
	data, err := ioutil.ReadFile(photodir + "/alice/Photos/master/2013/08/08/20130808_6.mp4")
	c.Assert(err, IsNil)
	_, err = f.Write(data)
	c.Assert(err, IsNil)
	c.Assert(f.Close(), IsNil)

	dt, err = time.Parse(common.TimeFormatLomod, "2013-07-28T20:00:00Z")
	c.Assert(err, IsNil)
	c.Assert(os.Chtimes(filename, dt, dt), IsNil)

	ts.resetDB(c, ts.h.conf.DbFilename, true)
	ts.startCCheck(c)
	result := ts.getCCheck(c)
	c.Assert(strings.Contains(result, bobCheck), Equals, true, Commentf(result))
	c.Assert(strings.Contains(result, `--------------- alice ---------------
General check is good
LivePhoto ZIP file check is good
LivePhoto Image file check is good
LivePhoto Image file duplication check is good
LivePhoto Image file zero size check is good
Asset file zero size check has 1 exception:
20031123_1.jpg : asset file size is 0
/tmp/usbdisk1/alice/Photos/master/2003/11/23/
20031123_1.jpg 0 2003-11-23T20:00:00Z
Asset in file system check has 1 exception:
20130728_7.png : asset in DB not found in file system or wrong SHA
/tmp/usbdisk1/alice/Photos/master/2013/07/28/
20130728_7.png 7360 2013-07-28T20:00:00Z
Asset in database check has 1 exception:
20031101_3.jpg : asset in file system not found in DB
/tmp/usbdisk1/alice/Photos/master/2003/11/01/
20031101_3.jpg 247759 2003-11-01T16:00:00Z
Asset create date in file system check has 1 exception:
20030117_4.jpg(17363532de7bc73e42823c1448bd52ffe45d4bfc) != 20030116_4.jpg(17363532de7bc73e42823c1448bd52ffe45d4bfc): expected creation date: 20030117
/tmp/usbdisk1/alice/Photos/master/2003/01/16/
20030116_4.jpg 231635 2003-01-17T20:00:00Z
Asset extension check has 1 exception:
20040121_5.jpg != 20040121_5.jpeg: expected extension: jpg
/tmp/usbdisk1/alice/Photos/master/2004/01/21/
20040121_5.jpeg 1125776 2004-01-21T20:00:00Z
Asset location check is good
Asset file hash duplication check has 1 exception:
20130808_6.mp4(good) == 20130728_7.png(wrong): hash duplicate: 5d14e3ce82d2101b3b8ece22487d81d824a5745a
/tmp/usbdisk1/alice/Photos/master/2013/08/08/
20130808_6.mp4 7360 2013-08-08T08:08:08Z
/tmp/usbdisk1/alice/Photos/master/2013/07/28/
20130728_7.png 7360 2013-07-28T20:00:00Z
User directory in file system check is good
User in database check is good
Preview files in file system check is good
Master directory in file system check is good
Preview directory in file system check is good`), Equals, true, Commentf(result))
} // nolint: lll

func (ts *mainSuite) TestInconsistentCheckAssetMiss(c *C) {
	category, err := loadCategory()
	c.Assert(err, IsNil)
	ts.createAssets(c, &category, ts.token)
	ts.waitPreviewComplete(c, false)

	ch := make(chan struct{})
	defer close(ch)
	go ts.h.maintenance(time.Now().Hour()+1, 0, 0, time.Hour, ch, true)

	// simulate	AssetMissInFS
	c.Assert(os.Remove(photodir+"/alice/Photos/master/2003/11/01/20031101_2.jpg"), IsNil)

	ts.resetDB(c, ts.h.conf.DbFilename, true)
	ts.startCCheck(c)
	result := ts.getCCheck(c)
	c.Assert(strings.Contains(result, bobCheck), Equals, true, Commentf(result))
	c.Assert(strings.Contains(result, `--------------- alice ---------------
General check is good
LivePhoto ZIP file check is good
LivePhoto Image file check is good
LivePhoto Image file duplication check is good
LivePhoto Image file zero size check is good
Asset file zero size check is good
Asset in file system check has 1 exception:
20031101_2.jpg : asset in DB not found in file system or wrong SHA
/tmp/usbdisk1/alice/Photos/master/2003/11/01/
!!! NOT FOUND
Asset in database check is good
Asset create date in file system check is good
Asset extension check is good
Asset location check is good
Asset file hash duplication check is good
User directory in file system check is good
User in database check is good
Preview files in file system check is good
Master directory in file system check is good
Preview directory in file system check is good`), Equals, true, Commentf(result))
}

func (ts *mainSuite) TestInconsistentCheckLivephoto1(c *C) {
	ts.createLivephotos(c)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	c.Assert(testutil.WaitWithFileCounts(ctx, photodir+"/alice/Photos/preview/2013/11/23", 4), IsNil)

	ch := make(chan struct{})
	defer close(ch)
	go ts.h.maintenance(time.Now().Hour()+1, 0, 0, time.Hour, ch, true)

	// simulate	LivePhotoZIPNotExist
	c.Assert(os.Remove(photodir+"/alice/Photos/master/2013/11/23/20131123_1.zip"), IsNil)
	// simulate	LivePhotoImageNotExist
	c.Assert(os.Remove(photodir+"/alice/Photos/master/2013/11/23/20131123_2_image.heic"), IsNil)

	ts.resetDB(c, ts.h.conf.DbFilename, true)
	ts.startCCheck(c)
	result := ts.getCCheck(c)
	c.Assert(strings.Contains(result, bobCheck), Equals, true, Commentf(result))
	c.Assert(strings.Contains(result, `--------------- alice ---------------
General check is good
LivePhoto ZIP file check has 1 exception:
20131123_1_image.jpg : live photo image file is exist, but zip file is not exist
/tmp/usbdisk1/alice/Photos/master/2013/11/23/
20131123_1_image.jpg 231635 2013-11-23T12:00:00Z
LivePhoto Image file check has 1 exception:
20131123_2.zip : live photo zip file is exist, but image file is not exist
/tmp/usbdisk1/alice/Photos/master/2013/11/23/
20131123_2.zip 1055380 2013-11-23T12:00:00Z
LivePhoto Image file duplication check is good
LivePhoto Image file zero size check is good
Asset file zero size check is good
Asset in file system check is good
Asset in database check is good
Asset create date in file system check is good
Asset extension check is good
Asset location check is good
Asset file hash duplication check is good
User directory in file system check is good
User in database check is good
Preview files in file system check is good
Master directory in file system check is good
Preview directory in file system check is good`), Equals, true, Commentf(result))
}

func (ts *mainSuite) TestInconsistentCheckLivephoto2(c *C) {
	ts.createLivephotos(c)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	c.Assert(testutil.WaitWithFileCounts(ctx, photodir+"/alice/Photos/preview/2013/11/23", 4), IsNil)

	ch := make(chan struct{})
	defer close(ch)
	go ts.h.maintenance(time.Now().Hour()+1, 0, 0, time.Hour, ch, true)

	// simulate	LivePhotoImageDuplicate
	filename := photodir + "/alice/Photos/master/2013/11/23/20131123_1_image.jpg"
	f, err := os.Create(filename)
	c.Assert(err, IsNil)
	data, err := ioutil.ReadFile(photodir + "/alice/Photos/master/2013/11/23/20131123_2_image.heic")
	c.Assert(err, IsNil)
	_, err = f.Write(data)
	c.Assert(err, IsNil)
	c.Assert(f.Close(), IsNil)

	dt, err := time.Parse(common.TimeFormatLomod, "2013-11-23T20:00:00Z")
	c.Assert(err, IsNil)
	c.Assert(os.Chtimes(filename, dt, dt), IsNil)

	ts.resetDB(c, ts.h.conf.DbFilename, true)
	ts.startCCheck(c)
	result := ts.getCCheck(c)
	c.Assert(strings.Contains(result, bobCheck), Equals, true, Commentf(result))
	res1 := `--------------- alice ---------------
General check is good
LivePhoto ZIP file check is good
LivePhoto Image file check is good
LivePhoto Image file duplication check has 1 exception:
20131123_2_image.heic = 20131123_1_image.jpg: live photo image has same hash 2a8210982e4cfbeb56d283f43fea9c118a53a839
/tmp/usbdisk1/alice/Photos/master/2013/11/23/
20131123_2.zip 1055380 2013-11-23T12:00:00Z
20131123_2_image.heic 1051433 2013-11-23T12:00:00Z
/tmp/usbdisk1/alice/Photos/master/2013/11/23/
20131123_1.zip 238254 2013-11-23T12:00:00Z
20131123_1_image.jpg 1051433 2013-11-23T20:00:00Z
LivePhoto Image file zero size check is good
Asset file zero size check is good
Asset in file system check is good
Asset in database check is good
Asset create date in file system check is good
Asset extension check is good
Asset location check is good
Asset file hash duplication check is good
User directory in file system check is good
User in database check is good
Preview files in file system check is good
Master directory in file system check is good
Preview directory in file system check is good`

	res2 := `--------------- alice ---------------
General check is good
LivePhoto ZIP file check is good
LivePhoto Image file check is good
LivePhoto Image file duplication check has 1 exception:
20131123_1_image.jpg = 20131123_2_image.heic: live photo image has same hash 2a8210982e4cfbeb56d283f43fea9c118a53a839
/tmp/usbdisk1/alice/Photos/master/2013/11/23/
20131123_1.zip 238254 2013-11-23T12:00:00Z
20131123_1_image.jpg 1051433 2013-11-23T20:00:00Z
/tmp/usbdisk1/alice/Photos/master/2013/11/23/
20131123_2.zip 1055380 2013-11-23T12:00:00Z
20131123_2_image.heic 1051433 2013-11-23T12:00:00Z
LivePhoto Image file zero size check is good
Asset file zero size check is good
Asset in file system check is good
Asset in database check is good
Asset create date in file system check is good
Asset extension check is good
Asset location check is good
Asset file hash duplication check is good
User directory in file system check is good
User in database check is good
Preview files in file system check is good
Master directory in file system check is good
Preview directory in file system check is good`
	c.Assert(strings.Contains(result, res1) || strings.Contains(result, res2), Equals, true, Commentf(result))
}

func (ts *mainSuite) TestInconsistentCheckLivephoto3(c *C) {
	ts.createLivephotos(c)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	c.Assert(testutil.WaitWithFileCounts(ctx, photodir+"/alice/Photos/preview/2013/11/23", 4), IsNil)

	ch := make(chan struct{})
	defer close(ch)
	go ts.h.maintenance(time.Now().Hour()+1, 0, 0, time.Hour, ch, true)

	// simulate	LivePhotoImageZeroSize
	filename := photodir + "/alice/Photos/master/2013/11/23/20131123_1_image.jpg"
	f, err := os.Create(filename)
	c.Assert(err, IsNil)
	c.Assert(f.Close(), IsNil)

	dt, err := time.Parse(common.TimeFormatLomod, "2013-11-23T20:00:00Z")
	c.Assert(err, IsNil)
	c.Assert(os.Chtimes(filename, dt, dt), IsNil)

	ts.resetDB(c, ts.h.conf.DbFilename, true)
	ts.startCCheck(c)
	result := ts.getCCheck(c)
	c.Assert(strings.Contains(result, bobCheck), Equals, true, Commentf(result))
	c.Assert(strings.Contains(result, `--------------- alice ---------------
General check has 1 exception:
while get file sha for asset Path: /tmp/usbdisk1/alice/Photos/master/2013/11/23/20131123_1_image.jpg: zip: not a valid zip file
LivePhoto ZIP file check is good
LivePhoto Image file check has 1 exception:
20131123_1.zip : live photo zip file is exist, but image file is not exist
/tmp/usbdisk1/alice/Photos/master/2013/11/23/
20131123_1.zip 238254 2013-11-23T12:00:00Z
20131123_1_image.jpg 0 2013-11-23T20:00:00Z
LivePhoto Image file duplication check is good
LivePhoto Image file zero size check has 1 exception:
20131123_1_image.jpg : live photo image file size is 0
/tmp/usbdisk1/alice/Photos/master/2013/11/23/
20131123_1.zip 238254 2013-11-23T12:00:00Z
20131123_1_image.jpg 0 2013-11-23T20:00:00Z
Asset file zero size check is good
Asset in file system check is good
Asset in database check is good
Asset create date in file system check is good
Asset extension check is good
Asset location check is good
Asset file hash duplication check is good
User directory in file system check is good
User in database check is good
Preview files in file system check is good
Master directory in file system check is good
Preview directory in file system check is good`), Equals, true, Commentf(result))
	fmt.Println(result)
} // nolint: lll

/*
 moved to api/test/lomod
func (ts *mainSuite) TestInconsistentCheckMissPreviewFile(c *C) {
}
*/

func (ts *mainSuite) startCCheck(c *C) {
	url := fmt.Sprintf("/system/ccheck?token=%s", ts.token)
	req, err := http.NewRequest("POST", url, nil)
	c.Assert(err, IsNil)

	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)

	// Check the status code is what we expect.
	res := rr.Result()
	defer res.Body.Close()
	content, _ := ioutil.ReadAll(res.Body)
	c.Assert(res.StatusCode, Equals, http.StatusOK, Commentf("%s", string(content)))
}

func (ts *mainSuite) getCCheck(c *C) string {
	// retry 1 minute
	for i := 0; i < 120; i++ {
		logrus.Info("get consistent check result")
		url := fmt.Sprintf("/system/ccheck?token=%s&%s=1", ts.token, common.QueryKeyPlainOutput)
		req, err := http.NewRequest("GET", url, nil)
		c.Assert(err, IsNil)

		rr := httptest.NewRecorder()
		ts.h.CreateRouter().ServeHTTP(rr, req)

		res := rr.Result()
		defer res.Body.Close()
		// Check the status code is what we expect.
		if res.StatusCode == http.StatusOK {
			data, err := ioutil.ReadAll(res.Body)
			c.Assert(err, IsNil)
			return string(data)
		}
		logrus.Warnf("consistent check expect get 200, but get %d", res.StatusCode)
		time.Sleep(500 * time.Millisecond)
	}
	return "Unable to get ok"
}
