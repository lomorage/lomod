// +build linux

package user

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"sort"

	"bitbucket.org/lomoware/lomo-backend/common"

	. "testing"

	. "gopkg.in/check.v1"
)

type userSuite struct {
}

var _ = Suite(&userSuite{})

func TestUserSuite(t *T) {
	TestingT(t)
}

func (ts *userSuite) TestUserBasic(c *C) {
	tmpdir, err := ioutil.TempDir("", "")
	c.Assert(err, IsNil)
	defer os.RemoveAll(tmpdir)

	dir := filepath.Join(tmpdir, common.RandomString(8))
	username := common.RandomString(8)
	password := common.RandomString(8)
	c.Assert(CreateOSUser(username, password, dir), IsNil, Commentf("%s - %s - %s.", username, password, dir))

	fmt.Printf("%s create\n", username)

	_, err = os.Stat(dir)
	c.Assert(err, NotNil)
	c.Assert(os.IsNotExist(err), Equals, true)

	_, err = user.Lookup(username)
	c.Assert(err, IsNil)

	// create directory and delete the user now
	c.Assert(os.Mkdir(dir, 0755), IsNil)

	c.Assert(DeleteOSUser(username), IsNil)
	_, err = os.Stat(dir)
	c.Assert(os.IsNotExist(err), Equals, true)
	_, err = user.Lookup(username)
	c.Assert(err, NotNil)
}

func (ts *userSuite) TestGenConf(c *C) {
	for _, conf := range []string{"", sambaGlobalTemplate} {
		data, err := generateConfig(conf, SambaUsersConf{
			{Name: "alice", Path: "/tmp/alice"},
			{Name: "BOB-b", Path: "/tmp/你好"},
			{Name: "Charlie_li", Path: "/tmp/Charlie李"},
		})
		c.Assert(err, IsNil)
		exp, err := ioutil.ReadFile("./testdata/sample1.conf")
		c.Assert(err, IsNil)
		c.Assert(data, Equals, string(exp))
	}
}

func (ts *userSuite) TestSort(c *C) {
	uc := SambaUsersConf{
		{Name: "Charlie_li", Path: "/tmp/Charlie李"},
		{Name: "BOB-b", Path: "/tmp/你好"},
		{Name: "alice", Path: "/tmp/alice"},
		{Name: "dann", Path: "/tmp/dann"},
	}
	exp := SambaUsersConf{
		{Name: "alice", Path: "/tmp/alice"},
		{Name: "BOB-b", Path: "/tmp/你好"},
		{Name: "Charlie_li", Path: "/tmp/Charlie李"},
		{Name: "dann", Path: "/tmp/dann"},
	}
	sort.Sort(uc)
	c.Assert(uc, DeepEquals, exp)
}

func (ts *userSuite) TestSambaCreateUser(c *C) {
	name := common.RandomString(8)
	pass := common.RandomString(8)
	home := "/tmp/test"

	// cleanup firstly
	DeleteOSUser(name)
	DeleteSambaUser(name)

	c.Assert(CreateSambaUser(name, pass), NotNil)
	c.Assert(CreateOSUser(name, pass, home), IsNil)
	defer DeleteOSUser(name)

	c.Assert(CreateSambaUser(name, pass), IsNil)
	c.Assert(DeleteSambaUser(name), IsNil)

	c.Assert(DeleteOSUser(name), IsNil)
}

func (ts *userSuite) TestGroupBasic(c *C) {
	name := common.RandomString(8)

	_, err := user.LookupGroup(name)
	c.Assert(err, NotNil)
	_, ok := err.(user.UnknownGroupError)
	c.Assert(ok, Equals, true, Commentf("%v", err))

	c.Assert(CheckAndCreateGroup(name), IsNil)

	_, err = user.LookupGroup(name)
	c.Assert(err, IsNil)

	c.Assert(deleteOSGroup(name), IsNil)

	_, err = user.LookupGroup(name)
	c.Assert(err, NotNil)
	_, ok = err.(user.UnknownGroupError)
	c.Assert(ok, Equals, true)
}
