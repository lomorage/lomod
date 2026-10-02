package handler

import (
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	. "gopkg.in/check.v1"
)

func (ts *mainSuite) TestDBReplicate(c *C) {
	dbFilename := filepath.Join(ts.userdir, "assets.db")
	ts.h.conf.ReplicaDuration = time.Second
	defer func() {
		ts.h.conf.ReplicaDuration = 0
	}()

	// start replicate loop
	go ts.h.replicateLoop(ts.h.gCtx, dbFilename, time.Second)

	// insert any 2 assets
	ts.insertAssetsWithGPS(c)

	// wait until replicate generation finished
	time.Sleep(2 * time.Second)

	// validate backup directory is created or not
	for _, n := range []string{ts.alice.Name, ts.bob.Name} {
		_, err := os.Stat(common.GetUserBackupDBDir(filepath.Join(photodir, n)))
		c.Assert(err, IsNil)
	}

	// truncate original dbfile and restore from alice or bob
	for _, name := range []string{ts.alice.Name, ts.bob.Name} {
		c.Assert(ioutil.WriteFile(dbFilename, []byte{}, common.DefaultFilePermission), IsNil)

		c.Assert(ts.restore(name), IsNil)

		result := ts.listUsers(c, ts.token)
		c.Assert(len(result), Equals, 2)
		c.Assert(result[0].Name, Equals, ts.alice.Name)
		c.Assert(result[1].Name, Equals, ts.bob.Name)
	}

	// delete one asset and repeat delete db + restore, and test syncup delete operation too
	sha := "4ebf54db04f335ff66bfc1fd982be62bf23fc967"
	ts.assetGet(c, "/asset/1?token="+ts.token, sha)
	ts.requestDelete(c, ts.token, &types.DeleteAssetItems{
		List: []types.DeleteAssetItem{{ID: "1"}},
	})

	_, err := ts.request("/asset/1?token="+ts.token, http.MethodGet, 404, nil, nil)
	c.Assert(err, IsNil)

	// wait 1 second to ensure sync are done. Assume sync is pretty fast
	time.Sleep(time.Second)
	for _, name := range []string{ts.alice.Name, ts.bob.Name} {
		c.Assert(ioutil.WriteFile(dbFilename, []byte{}, common.DefaultFilePermission), IsNil)

		c.Assert(ts.restore(name), IsNil)

		_, err := ts.request("/asset/1?token="+ts.token, http.MethodGet, 404, nil, nil)
		c.Assert(err, IsNil)
	}

	// import the asset again, and restore from backup
	u := fmt.Sprintf("/asset?token=%s&ext=jpg&createtime=2003-11-23T12:00:00Z&sha1=%s", ts.token, sha)
	ts.createAsset(c, "../cmd/lomod/test/img/5_2003_11_23.jpg", u, "3.jpg", sha)
	ts.assetGet(c, "/asset/3?token="+ts.token, sha)

	// wait 1 second to ensure sync are done. Assume sync is pretty fast
	time.Sleep(time.Second)
	for _, name := range []string{ts.alice.Name, ts.bob.Name} {
		c.Assert(ioutil.WriteFile(dbFilename, []byte{}, common.DefaultFilePermission), IsNil)

		c.Assert(ts.restore(name), IsNil)
		ts.assetGet(c, "/asset/3?token="+ts.token, sha)
	}

	// cancel earlier to avoid replication still write
	ts.cancel()
}

func (ts *mainSuite) TestDBReplicateUserAddDelete(c *C) {
	dbFilename := filepath.Join(ts.userdir, "assets.db")
	ts.h.conf.ReplicaDuration = time.Second
	defer func() {
		ts.h.conf.ReplicaDuration = 0
	}()

	// start replicate loop
	go ts.h.replicateLoop(ts.h.gCtx, dbFilename, time.Second)

	// insert any 2 assets
	ts.insertAssetsWithGPS(c)

	// add new user, replicate should support
	u := user.User{
		Name:     "charlie",
		Password: "charlie123",
		Phone:    "charlie_phone",
		Email:    "charlie_email",
		NickName: "charlie_nick",
		HomeDir:  photodir + "/charlie",
	}
	err := ts.createUser(u, ts.token, true)
	c.Assert(err, IsNil)

	// wait until replicate finished
	time.Sleep(2 * time.Second)

	_, err = os.Stat(common.GetUserBackupDBDir(filepath.Join(photodir, u.Name)))
	c.Assert(err, IsNil)

	// cancel earlier to avoid replication still write
	ts.cancel()
}

func (ts *mainSuite) TestDBRestoreFail(c *C) {
	// Restore from not exist user should fail
	dbFilename := filepath.Join(ts.userdir, "assets.db")

	ts.h.conf.ReplicaDuration = time.Second
	defer func() {
		ts.h.conf.ReplicaDuration = 0
	}()

	// start replicate loop
	go ts.h.replicateLoop(ts.h.gCtx, dbFilename, time.Second)

	// insert any 2 assets
	ts.insertAssetsWithGPS(c)

	time.Sleep(2 * time.Second)

	fmt.Println("should fail to restore from unknown user")
	c.Assert(ts.restore("12345"), NotNil)

	fmt.Println("should be success to restore from alice's backup")
	c.Assert(ts.restore(ts.alice.Name), IsNil)

	// Restore failure, such as removing wal folder should not impact current DB
	name := ts.alice.Name
	genDir := filepath.Join(common.GetUserBackupDBDir(filepath.Join(photodir, name)), "generations")
	fis, err := ioutil.ReadDir(genDir)
	c.Assert(err, IsNil)
	c.Assert(len(fis), Equals, 1, Commentf("%v", fis))
	walDir := filepath.Join(filepath.Join(genDir, fis[0].Name()), "wal")
	wals, err := ioutil.ReadDir(walDir)
	c.Assert(err, IsNil)
	c.Assert(len(wals) > 0, Equals, true, Commentf("%v", wals))

	walFile := filepath.Join(walDir, wals[0].Name())
	fmt.Printf("remove alice's wal file (%s) and validate should fail\n", walFile)
	c.Assert(os.Remove(walFile), IsNil)

	fmt.Println("should fail to restore from corrupted backup")
	c.Assert(ts.restore(name), NotNil)

	fmt.Println("normal operation should still be success")
	result := ts.listUsers(c, ts.token)
	c.Assert(len(result), Equals, 2)
	c.Assert(result[0].Name, Equals, ts.alice.Name)
	c.Assert(result[1].Name, Equals, ts.bob.Name)

	sha := "4ebf54db04f335ff66bfc1fd982be62bf23fc967"
	ts.assetGet(c, "/asset/1?token="+ts.token, sha)
	sha = "2a8210982e4cfbeb56d283f43fea9c118a53a839"
	ts.assetGet(c, "/asset/2?token="+ts.token, sha)

	// cancel earlier to avoid replication still write
	ts.cancel()
}
