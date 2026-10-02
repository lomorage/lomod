package common

import (
	"io/ioutil"
	"net/http"
	"os"
	. "testing"

	. "gopkg.in/check.v1"
)

type commonSuite struct {
}

var _ = Suite(&commonSuite{})

func TestCommonSuite(t *T) {
	TestingT(t)
}

func (ts *commonSuite) TestIfMatch(c *C) {
	header := http.Header{}
	SetHeaderIfMatch(header, 123, "abc")
	size, hash, err := GetHeaderIfMatch(header)
	c.Assert(err, IsNil)
	c.Assert(size, Equals, 123)
	c.Assert(hash, Equals, "abc")
}

// test empty token for the 1st time
func (ts *commonSuite) TestAdminTokenEmpty(c *C) {
	tmpfile, err := ioutil.TempFile("", "")
	c.Assert(err, IsNil)
	defer tmpfile.Close()
	defer os.RemoveAll(tmpfile.Name())

	token, err := LoadAndSaveAdminToken(tmpfile.Name(), "", DefaultFilePermission)
	c.Assert(err, IsNil)
	c.Assert(token != "", Equals, true)

	// same token should return same
	token2, err := LoadAndSaveAdminToken(tmpfile.Name(), token, DefaultFilePermission)
	c.Assert(err, IsNil)
	c.Assert(token, Equals, token2)

	// empty token should return existing one
	token2, err = LoadAndSaveAdminToken(tmpfile.Name(), "", DefaultFilePermission)
	c.Assert(err, IsNil)
	c.Assert(token, Equals, token2)

	// different token should return different value
	token = token + "123"
	token2, err = LoadAndSaveAdminToken(tmpfile.Name(), token, DefaultFilePermission)
	c.Assert(err, IsNil)
	c.Assert(token, Equals, token2)
}

// test given token for the 1st time
func (ts *commonSuite) TestAdminTokenGiven(c *C) {
	tmpfile, err := ioutil.TempFile("", "")
	c.Assert(err, IsNil)
	defer tmpfile.Close()
	defer os.RemoveAll(tmpfile.Name())

	token := "123456"
	token2, err := LoadAndSaveAdminToken(tmpfile.Name(), token, DefaultFilePermission)
	c.Assert(err, IsNil)
	c.Assert(token, Equals, token2)

	token = "54321"
	token2, err = LoadAndSaveAdminToken(tmpfile.Name(), token, DefaultFilePermission)
	c.Assert(err, IsNil)
	c.Assert(token, Equals, token2)
}
