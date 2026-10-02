package atlomod

import (
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"

	"bitbucket.org/lomoware/lomo-backend/client"
	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	. "gopkg.in/check.v1"
)

func (ts *lomodAPISuite) TestSetBackup(c *C) {
	/*
		without backup dir, then user added backup dir
		steps:
		2. add bob without backup directory, system API should not display status
		3. add backup directory, system API should display status
	*/
	c.Assert(ts.lomoc.AddUser(&user.User{Name: bob, Password: bobPwd, HomeDir: defaultMntDir}), IsNil)
	bobLomoc := client.NewLomod("127.0.0.1:" + strconv.Itoa(ts.listenPort))
	_, err := bobLomoc.Login(bob, bobPwd)
	c.Assert(err, IsNil)

	si, err := ts.lomoc.System()
	c.Assert(err, IsNil)
	c.Assert(len(si.UserStatus), Equals, 2)
	us, ok := si.UserStatus[bob]
	c.Assert(ok, Equals, true)
	c.Assert(us.HomeDisk.Status, Equals, "")
	c.Assert(us.BackupDisk, IsNil)

	c.Assert(ts.lomoc.SetBackupDisk(bob, strings.TrimPrefix(defaultHomeDir, "/")), IsNil)
	si, err = ts.lomoc.System()
	us = si.UserStatus[bob]
	c.Assert(us.BackupDisk, NotNil)
	c.Assert(us.BackupDisk.Status, Equals, "")

	// validate UUID read from conf folder
	confDir := common.GetUserConfDir(defaultHomeDir, bob)
	contents, err := ioutil.ReadFile(filepath.Join(confDir, types.UUIDFilename))
	c.Assert(err, IsNil)

	uuid := strings.TrimSpace(string(contents))
	c.Assert(strings.HasPrefix(uuid, types.MountUSB+types.UUIDDelimiter), Equals, true, Commentf(uuid))
	// it should be equal to alice's home UUID
	exp, err := getAliceUUID()
	c.Assert(err, IsNil)
	c.Assert(uuid, Equals, exp)

	// import one asset and start backup
	_, err = ts.insertAssetWithLomoc(testAsset1, bobLomoc)
	c.Assert(err, IsNil)

	// validate before preview is created
	c.Assert(ts.validateBackup(bobLomoc, 6), IsNil)
}
