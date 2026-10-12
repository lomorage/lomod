package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	. "gopkg.in/check.v1"
)

// TestListMountsBaseDirAsMountDir covers the macOS (and Windows) default install, where lomod
// runs with MountDir == BaseDir (e.g. ~/Pictures/Lomorage) instead of a "/media" USB staging
// dir. /mount must succeed and report the base dir itself as a usable, error-free location,
// otherwise signup shows "No storage locations found" (NoDisksFound).
func (ts *mainSuite) TestListMountsBaseDirAsMountDir(c *C) {
	orig := ts.h.conf.MountDir
	defer func() { ts.h.conf.MountDir = orig }()

	base := filepath.Join(c.MkDir(), "Lomorage")
	c.Assert(os.MkdirAll(base, 0755), IsNil)
	// lomod's own reserved subdirs under --base must not be offered as storage locations
	c.Assert(os.MkdirAll(filepath.Join(base, "var"), 0755), IsNil)
	c.Assert(os.MkdirAll(filepath.Join(base, "etc"), 0755), IsNil)
	ts.h.conf.MountDir = base

	dirs := ts.listMounts(c)
	found := false
	for _, d := range dirs {
		c.Assert(d.Dir, Not(Equals), filepath.Join("Lomorage", "var"))
		c.Assert(d.Dir, Not(Equals), filepath.Join("Lomorage", "etc"))
		if d.Dir == "Lomorage" {
			found = true
			c.Assert(d.Error, Equals, "")
			c.Assert(d.TotalSize > 0, Equals, true)
		}
	}
	c.Assert(found, Equals, true)
}

// TestListMountsMissingMountDir documents the pre-fix macOS behavior: a MountDir that does not
// exist (the "/media" default on a Mac) makes /mount fail outright.
func (ts *mainSuite) TestListMountsMissingMountDir(c *C) {
	orig := ts.h.conf.MountDir
	defer func() { ts.h.conf.MountDir = orig }()
	ts.h.conf.MountDir = filepath.Join(c.MkDir(), "media-does-not-exist")

	req, err := http.NewRequest("GET", "/mount?token="+ts.token, nil)
	c.Assert(err, IsNil)
	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	c.Assert(rr.Code, Equals, http.StatusInternalServerError)
}
