package lomocloud

import (
	"io/ioutil"
	. "testing"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common/migrator"
	lsql "bitbucket.org/lomoware/lomo-backend/migrations/sqls/lomocloud"

	. "gopkg.in/check.v1"
)

type mainSuite struct {
	h *Handler
}

var _ = Suite(&mainSuite{})

func TestMainSuite(t *T) {
	TestingT(t)
}

func (ts *mainSuite) SetUpSuite(c *C) {
	ts.h = &Handler{conf: &Config{DeadTimeout: 3 * time.Second}}
}
func (ts *mainSuite) TearDownSuite(c *C) {
}

func (ts *mainSuite) SetUpTest(c *C) {
	if ts.h.db != nil {
		c.Assert(ts.h.db.Close(), IsNil)
	}
	dbfile, err := ioutil.TempFile("", "lomocloud-db")
	c.Assert(err, IsNil)
	c.Assert(dbfile.Close(), IsNil)

	c.Assert(migrator.StartLomocloud(dbfile.Name(), lsql.SchemaStatements), IsNil)
	ts.h.df = dbfile.Name()
	c.Assert(ts.h.openSqliteDB(), IsNil)
}

func (ts *mainSuite) TearDownTest(c *C) {
}
