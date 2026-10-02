package lomoframe

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	. "testing"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/cmd"
	"bitbucket.org/lomoware/lomo-backend/common/security"
	. "gopkg.in/check.v1"
)

const testDataDir = "testdata"

type mainSuite struct {
}

var _ = Suite(&mainSuite{})

func TestMainSuite(t *T) {
	TestingT(t)
}

func (ts *mainSuite) SetUpTest(c *C) {
	os.Remove(filepath.Join(testDataDir, qrFile))
}

func (ts *mainSuite) decodeQRCode(dir string) (*qrinfo, error) {
	payload, err := cmd.Run("zbarimg", "-q", "--raw", "--nodbus", filepath.Join(dir, qrFile))
	if err != nil {
		return nil, err
	}

	info := &qrinfo{}
	return info, json.Unmarshal(payload, info)
}

func (ts *mainSuite) TestBootstrap(c *C) {
	tmpdir, err := ioutil.TempDir("", "")
	c.Assert(err, IsNil)
	defer os.RemoveAll(tmpdir)

	h := &Handler{conf: &Config{Port: 1234, ConfDir: tmpdir}}
	c.Assert(h.loadConf("state_test.json"), IsNil)
	c.Assert(h.conf.LomodURL, Equals, "")
	c.Assert(h.conf.Username, Not(Equals), "")
	c.Assert(h.conf.Password, Not(Equals), "")
	c.Assert(h.status, Equals, common.SystemStatusNew)

	info, err := ts.decodeQRCode(tmpdir)
	c.Assert(err, IsNil)
	c.Assert(info.Name, Equals, h.conf.Username)
	c.Assert(info.Port, Equals, h.conf.Port)
	c.Assert(info.Password, Equals, security.EncryptPassword(h.conf.Username, h.conf.Password))
}

func (ts *mainSuite) TestNewState(c *C) {
	h := &Handler{conf: &Config{ConfDir: testDataDir, Port: 10000}}
	c.Assert(h.loadConf("state_new.json"), IsNil)
	c.Assert(h.conf.LomodURL, Equals, "")
	c.Assert(h.conf.Username, Equals, "lomoframe-0xxx")
	c.Assert(h.conf.Password, Equals, "zbst5tdl")
	c.Assert(h.conf.Port, Equals, uint(10000))
	c.Assert(h.status, Equals, common.SystemStatusNew)

	info, err := ts.decodeQRCode(testDataDir)
	c.Assert(err, IsNil)
	c.Assert(info.Name, Equals, h.conf.Username)
	c.Assert(info.Port, Equals, h.conf.Port)
	c.Assert(info.Password, Equals, security.EncryptPassword(h.conf.Username, h.conf.Password))
}

func (ts *mainSuite) TestInitedState(c *C) {
	h := &Handler{conf: &Config{ConfDir: testDataDir}}
	c.Assert(h.loadConf("state_inited.json"), IsNil)
	c.Assert(h.conf.LomodURL, Equals, "127.0.0.1")
	c.Assert(h.conf.Username, Equals, "lomoframe-01uo")
	c.Assert(h.conf.Password, Equals, "zbst5tdl")
	c.Assert(h.conf.Port, Equals, uint(0))
	c.Assert(h.status, Equals, common.SystemStatusInited)

	info, err := ts.decodeQRCode(testDataDir)
	c.Assert(err, IsNil)
	c.Assert(info.Name, Equals, h.conf.Username)
	c.Assert(info.Port, Equals, h.conf.Port)
	c.Assert(info.Password, Equals, security.EncryptPassword(h.conf.Username, h.conf.Password))
}

func (ts *mainSuite) TestReset(c *C) {
	h := &Handler{conf: &Config{ConfDir: testDataDir}}
	c.Assert(h.loadConf("state_inited.json"), IsNil)

	c.Assert(cmd.Exec("cp", filepath.Join(h.conf.ConfDir, "state_inited.json"),
		filepath.Join(h.conf.ConfDir, confFile)), IsNil)

	h.recreate("", "")
	c.Assert(h.conf.LomodURL, Equals, "")
	c.Assert(h.conf.Username, Not(Equals), "lomoframe-01uo")
	c.Assert(h.conf.Password, Not(Equals), "zbst5tdl")
	c.Assert(h.status, Equals, common.SystemStatusNew)

	info, err := ts.decodeQRCode(testDataDir)
	c.Assert(err, IsNil)
	c.Assert(info.Name, Equals, h.conf.Username)
	c.Assert(info.Port, Equals, h.conf.Port)
	c.Assert(info.Password, Equals, security.EncryptPassword(h.conf.Username, h.conf.Password))
}
