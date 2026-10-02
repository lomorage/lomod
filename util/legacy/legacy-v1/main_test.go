package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	. "testing"

	"bytes"

	. "gopkg.in/check.v1"
)

type mainSuite struct {
	h *fsHandler
}

var _ = Suite(&mainSuite{})

func TestMainSuite(t *T) {
	TestingT(t)
}

func (ts *mainSuite) SetUpSuite(c *C)    {}
func (ts *mainSuite) TearDownSuite(c *C) {}
func (ts *mainSuite) SetUpTest(c *C) {
	wd, err := os.Getwd()
	c.Assert(err, IsNil)
	ts.h = &fsHandler{baseDir: path.Join(wd, "test")}
}
func (ts *mainSuite) TearDownTest(c *C) {}

type fileinfo struct {
	Name string
	Type int
}

type response struct {
	Files []fileinfo
}

func checkResponse(c *C, body []byte, fi []fileinfo) {
	var r response
	c.Assert(json.Unmarshal(body, &r), IsNil)
	c.Assert(len(r.Files), Equals, len(fi))

	for i := 0; i < len(r.Files); i++ {
		c.Assert(r.Files[i].Name, Equals, fi[i].Name)
		c.Assert(r.Files[i].Type, Equals, fi[i].Type)
	}
}

func (ts *mainSuite) checkReq(c *C, method, dir string, body io.Reader, handler http.HandlerFunc, fi []fileinfo, ok, cont bool) {
	// Create a request to pass to our handler. We don't have any query parameters for now, so we'll
	// pass 'nil' as the third parameter.
	req, err := http.NewRequest(method, dir, body)
	c.Assert(err, IsNil)

	// We create a ResponseRecorder (which satisfies http.ResponseWriter) to record the response.
	rr := httptest.NewRecorder()

	// Our handlers satisfy http.Handler, so we can call their ServeHTTP method
	// directly and pass in our Request and ResponseRecorder.
	handler.ServeHTTP(rr, req)

	// Check the status code is what we expect.
	c.Assert(rr.Code == http.StatusOK, Equals, ok)

	if !cont || !ok {
		return
	}
	checkResponse(c, rr.Body.Bytes(), fi)
}

func (ts *mainSuite) checkGetDir(c *C, dir string, fi []fileinfo, ok bool) {
	ts.checkReq(c, "GET", dir, nil, http.HandlerFunc(ts.h.handleGet), fi, ok, true)
}

func (ts *mainSuite) TestGetDir(c *C) {
	c.Skip("deprecated")
	// check content
	res := []fileinfo{
		{Name: "Documents", Type: 0},
		{Name: "Movies", Type: 0},
		{Name: "Musics", Type: 0},
		{Name: "Photos", Type: 0},
	}
	ts.checkGetDir(c, "/", res, true)

	// check Document folder
	res2 := []fileinfo{
		{Name: "audio.mp3", Type: 1},
		{Name: "image.jpg", Type: 1},
		{Name: "pdf.pdf", Type: 1},
		{Name: "video.mp4", Type: 1},
	}
	ts.checkGetDir(c, "/Documents", res2, true)

	// check Movies
	res3 := []fileinfo{
		{Name: "video.mp4", Type: 1},
	}
	ts.checkGetDir(c, "/Movies", res3, true)

	// check Musics
	ts.checkGetDir(c, "/Musics", []fileinfo{}, true)

	// check photos
	res4 := []fileinfo{
		{Name: "2015", Type: 0},
	}
	ts.checkGetDir(c, "/Photos", res4, true)

	res5 := []fileinfo{
		{Name: "image.jpg", Type: 1},
	}
	ts.checkGetDir(c, "/Photos/2015", res5, true)
}

func (ts *mainSuite) checkCreate(c *C, dir string) {
	ts.checkReq(c, "PUT", dir, nil, http.HandlerFunc(ts.h.handlePut), []fileinfo{}, true, false)

	// check again should expect failure
	ts.checkReq(c, "PUT", dir, nil, http.HandlerFunc(ts.h.handlePut), []fileinfo{}, false, false)

	// Get again
	ts.checkGetDir(c, dir, []fileinfo{}, true)
}

func (ts *mainSuite) checkDelete(c *C, dir string) {
	ts.checkReq(c, "DELETE", dir, nil, http.HandlerFunc(ts.h.handleDelete), []fileinfo{}, true, false)

	// Get again
	ts.checkGetDir(c, dir, []fileinfo{}, false)
}

func (ts *mainSuite) TestDirCreateDelete(c *C) {
	c.Skip("deprecated")
	ts.checkCreate(c, "/test")
	ts.checkDelete(c, "/test")
}

func (ts *mainSuite) TestFileUploadDownloadDelete(c *C) {
	c.Skip("deprecated")
	ts.checkCreate(c, "/test")
	defer ts.checkDelete(c, "/test")

	// create temp file
	upload := "helloworld"
	dir := "/test/hello"
	ts.checkReq(c, "POST", dir, bytes.NewBufferString(upload), http.HandlerFunc(ts.h.handlePost), []fileinfo{}, true, false)

	// download file and compare
	req, err := http.NewRequest("GET", dir, nil)
	c.Assert(err, IsNil)

	rr := httptest.NewRecorder()

	http.HandlerFunc(ts.h.handleGet).ServeHTTP(rr, req)

	// Check the status code is what we expect.
	c.Assert(rr.Code, Equals, http.StatusOK)

	c.Assert(bytes.Compare(rr.Body.Bytes(), []byte(upload)), Equals, 0)

	// delete the file
	ts.checkDelete(c, dir)
}
