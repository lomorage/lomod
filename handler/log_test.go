package handler

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	"bytes"

	. "gopkg.in/check.v1"
)

func (ts *mainSuite) TestDownloadLog(c *C) {
	p, err := os.Getwd()
	c.Assert(err, IsNil)
	ts.h.conf.LogDir = filepath.Join(p, "testdata")

	found := ts.testDownloadLog(c, map[string]string{
		"test1.log":   "12345\nhello world\n",
		"test2.log":   "hello world\n12345\n",
		"test1.log.1": "",
	})
	_, ok := found["test1.log"]
	c.Assert(ok, Equals, true)
	_, ok = found["test2.log"]
	c.Assert(ok, Equals, true)
	_, ok = found["test1.log.1"]
	c.Assert(ok, Equals, false)
}

func (ts *mainSuite) testDownloadLog(c *C, files map[string]string) map[string]struct{} {
	req, err := http.NewRequest("GET", "/log?token="+ts.token, nil)
	c.Assert(err, IsNil)

	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)

	res := rr.Result()
	defer res.Body.Close()

	c.Assert(res.StatusCode, Equals, 200)

	r, err := gzip.NewReader(res.Body)
	c.Assert(err, IsNil)

	found := map[string]struct{}{}
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			log.Fatal(err)
		}
		body, ok := files[hdr.Name]
		if !ok {
			_, err := io.Copy(ioutil.Discard, tr)
			c.Assert(err, IsNil)
			continue
		}
		found[hdr.Name] = struct{}{}
		buf := &bytes.Buffer{}
		_, err = io.Copy(buf, tr)
		c.Assert(err, IsNil)
		c.Assert(buf.String(), Equals, body)
	}
	return found
}

func (ts *mainSuite) TestUploadLog(c *C) {
	p, err := os.Getwd()
	c.Assert(err, IsNil)
	ts.h.conf.LogDir = filepath.Join(p, "testdata")

	expectLog := filepath.Join(ts.h.conf.LogDir, "alice.log")
	defer os.Remove(expectLog)

	// new upload should override new one
	logContent := "upload log test"
	ts.testUploadLog(c, logContent)
	_, err = os.Stat(expectLog)
	c.Assert(err, IsNil)

	content, err := ioutil.ReadFile(expectLog)
	c.Assert(err, IsNil)
	c.Assert(string(content), Equals, logContent)

	logContent = "new upload test"
	ts.testUploadLog(c, logContent)
	_, err = os.Stat(expectLog)
	c.Assert(err, IsNil)

	content, err = ioutil.ReadFile(expectLog)
	c.Assert(err, IsNil)
	c.Assert(string(content), Equals, logContent)
}

func (ts *mainSuite) testUploadLog(c *C, logContent string) {
	buf := bytes.NewBufferString(logContent)
	ts.requestWithMethodBody(c, fmt.Sprintf("/log?token="+ts.token), "POST", http.StatusOK, buf, nil, nil, nil)

	found := ts.testDownloadLog(c, map[string]string{
		"test1.log":   "12345\nhello world\n",
		"test2.log":   "hello world\n12345\n",
		"test1.log.1": "",
		"alice.log":   logContent,
	})
	_, ok := found["test1.log"]
	c.Assert(ok, Equals, true)
	_, ok = found["test2.log"]
	c.Assert(ok, Equals, true)
	_, ok = found["test1.log.1"]
	c.Assert(ok, Equals, false)
	_, ok = found["alice.log"]
	c.Assert(ok, Equals, true)
}
