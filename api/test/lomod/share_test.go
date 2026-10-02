package atlomod

import (
	"context"
	"crypto/sha1"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/client"
	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/group"
	"bitbucket.org/lomoware/lomo-backend/common/testutil"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/pkg/errors"
	. "gopkg.in/check.v1"
)

const (
	bob        = "bob"
	bobPwd     = "bob123"
	charlie    = "charlie"
	charliePwd = "charlie123"
)

func (ts *lomodAPISuite) prepareShareAsset() (*types.Asset, AssetInfo, error) {
	// only test heic transcode
	var a types.Asset
	for _, y := range ts.category.Years {
		for _, m := range y.Months {
			for _, d := range m.Days {
				for _, asset := range d.Assets {
					if filepath.Ext(asset.Name) != ".heic" {
						continue
					}
					a = asset
					break
				}
			}
		}
	}

	ar, err := ts.insertAsset(a)
	return ar, ts.assetsInfo[a.Name], err
}

func (ts *lomodAPISuite) validateShareDownload(id uint64, expectSHA1 string, isPreview bool, q map[string]string, lomoc *client.Lomod) error {
	h := sha1.New()
	r, err := lomoc.ReceiveSharedAsset(id, isPreview, q)
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

func (ts *lomodAPISuite) validateShare(id uint64, info AssetInfo, lomoc *client.Lomod) error {
	q := map[string]string{common.QueryKeyICodec: "jpg"}
	err := ts.validateShareDownload(id, info.ImgXcode[runtime.GOARCH][ts.osDistro][ts.osRelease].SHA1, false, q, lomoc)
	if err != nil {
		return err
	}

	// reset query string
	for d, p := range info.Previews[runtime.GOARCH][ts.osDistro][ts.osRelease] {
		q = map[string]string{}
		parts := strings.Split(d, "x")
		if parts[0] != strconv.Itoa(common.DefaultPreviewWidth) {
			q = map[string]string{common.QueryKeyWidth: parts[0], common.QueryKeyHeight: parts[1]}
		}
		err := ts.validateShareDownload(id, p.SHA1, true, q, lomoc)
		if err != nil {
			return err
		}
	}
	return nil
}

// This only covers transcode part. Main testcases refer handler/share_test.go
func (ts *lomodAPISuite) TestShareToBob(c *C) {
	// create bob
	bobLomoc, err := ts.createBobUser()
	c.Assert(err, IsNil)

	// insert the picture
	ar, info, err := ts.prepareShareAsset()
	c.Assert(err, IsNil)

	_, previewDir := common.GetUserPhotoDir(getAliceHome())
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	c.Assert(testutil.WaitWithFileCounts(ctx, previewDir, 6), IsNil)

	aid, err := strconv.Atoi(strings.Split(ar.Name, ".")[0])
	c.Assert(err, IsNil)

	// alice share one heic picture to bob
	// FIXME: use list API to get bob's ID
	s, err := ts.lomoc.Share(2, aid, true)
	c.Assert(err, IsNil)

	// bob starts download the asset with specified codec
	c.Assert(ts.validateShare(s.ID, info, bobLomoc), IsNil)
}

// This only covers transcode part. Main testcases refer handler/share_test.go
func (ts *lomodAPISuite) TestShareToGroup(c *C) {
	// create bob
	bu := &user.User{ID: 2, Name: bob, Password: bobPwd, HomeDir: defaultHomeDir}
	c.Assert(ts.lomoc.AddUser(bu), IsNil)
	// create bobLomoc
	bobLomoc := client.NewLomod("127.0.0.1:" + strconv.Itoa(ts.listenPort))
	_, err := bobLomoc.Login(bob, bobPwd)
	c.Assert(err, IsNil)

	// create charlie
	cu := &user.User{ID: 3, Name: charlie, Password: charliePwd, HomeDir: defaultHomeDir}
	c.Assert(ts.lomoc.AddUser(cu), IsNil)
	// create charlieLomoc
	charlieLomoc := client.NewLomod("127.0.0.1:" + strconv.Itoa(ts.listenPort))
	_, err = charlieLomoc.Login(charlie, charliePwd)
	c.Assert(err, IsNil)

	// create one group and add bob and charlie
	c.Assert(ts.lomoc.AddGroup(&group.Group{Name: "test", Members: []*user.User{bu, cu}}), IsNil)

	// insert the picture
	ar, info, err := ts.prepareShareAsset()
	c.Assert(err, IsNil)

	_, previewDir := common.GetUserPhotoDir(getAliceHome())
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	c.Assert(testutil.WaitWithFileCounts(ctx, previewDir, 6), IsNil)

	aid, err := strconv.Atoi(strings.Split(ar.Name, ".")[0])
	c.Assert(err, IsNil)

	// alice share one heic picture to bob
	// FIXME: use list API to get group ID
	s, err := ts.lomoc.Share(1, aid, false)
	c.Assert(err, IsNil)

	c.Assert(ts.validateShare(s.ID, info, bobLomoc), IsNil)
	c.Assert(ts.validateShare(s.ID, info, charlieLomoc), IsNil)
}
