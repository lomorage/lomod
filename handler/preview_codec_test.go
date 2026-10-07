package handler

import (
	"crypto/sha1"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	. "gopkg.in/check.v1"
)

// previewFiles lists every preview file the runner has written for alice, skipping the
// dot-prefixed temp files it renames into place.
func (ts *mainSuite) previewRoot() string {
	return filepath.Join(ts.h.getUserDir(photodir+"/alice", "alice"), "Photos", "preview")
}

func (ts *mainSuite) previewFiles(c *C) []string {
	files := []string{}
	root := ts.previewRoot()
	err := filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !fi.IsDir() && !strings.HasPrefix(fi.Name(), ".") {
			files = append(files, filepath.ToSlash(p))
		}
		return nil
	})
	if os.IsNotExist(err) {
		return files
	}
	c.Assert(err, IsNil)
	sort.Strings(files)
	return files
}

func (ts *mainSuite) waitPreviewFile(c *C, suffix string) {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		for _, f := range ts.previewFiles(c) {
			if strings.HasSuffix(f, suffix) {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	c.Fatalf("no pre-generated preview *%s after 30s, have %v", suffix, ts.previewFiles(c))
}

func (ts *mainSuite) getPreview(c *C, url string) (contentType, sum string) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	c.Assert(err, IsNil)
	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	validateStatusCode(c, url, rr, http.StatusOK)
	h := sha1.New()
	_, err = io.Copy(h, rr.Result().Body)
	c.Assert(err, IsNil)
	return rr.Header().Get("Content-Type"), fmt.Sprintf("%x", h.Sum(nil))
}

func fileSHA1(c *C, p string) string {
	data, err := ioutil.ReadFile(p)
	c.Assert(err, IsNil)
	return fmt.Sprintf("%x", sha1.Sum(data))
}

func (ts *mainSuite) TestConfJsPreviewCodec(c *C) {
	defer func() { ts.h.conf.UseJpg = false }()
	for _, useJpg := range []bool{false, true} {
		ts.h.conf.UseJpg = useJpg
		r, err := ts.request("/static/lomo/js/conf.js", http.MethodGet, http.StatusOK, nil, nil)
		c.Assert(err, IsNil)
		body, err := ioutil.ReadAll(r)
		r.Close()
		c.Assert(err, IsNil)
		js := string(body)
		c.Assert(strings.Contains(js, fmt.Sprintf("WEBP_PREVIEW: %t,", !useJpg)), Equals, true,
			Commentf("UseJpg=%t", useJpg))
		c.Assert(strings.Contains(js, "__WEBP_PREVIEW__"), Equals, false)
		c.Assert(strings.Contains(js, `(CONFIG.WEBP_PREVIEW ? "&icodec=webp" : "")`), Equals, true)
	}
}

// The web UI's preview URLs (own gallery and inbox) ask for the codec the runner
// pre-generates, so they are served from disk instead of transcoding the original.
func (ts *mainSuite) TestPreviewCodecMatchesPregenerated(c *C) {
	const sha = "575db2e474109f982ead09e7f8676680a679c9c0"
	f := "../cmd/lomod/test/img/3_2003_11_01.jpg"
	// SetUpTest's cleanup misses the real home dir on Windows; start from an empty preview dir.
	c.Assert(os.RemoveAll(ts.previewRoot()), IsNil)
	url := fmt.Sprintf("/asset/%s?token=%s&ext=jpg&createtime=2003-11-01T00:01:00Z", sha, ts.token)
	ai := ts.importAsset(c, f, url)
	c.Assert(ai.Hash, Equals, sha)
	dims := fmt.Sprintf("width=%d&height=320", testImgPreviewWidth)
	webpSuffix := fmt.Sprintf("/2003/11/01/20031101_1_%d_320.webp", testImgPreviewWidth)
	ts.waitPreviewFile(c, webpSuffix)
	ts.waitPreviewFile(c, "/2003/11/01/20031101_1_75_75.webp")
	pregenerated := filepath.Join(ts.previewRoot(), filepath.FromSlash(strings.TrimPrefix(webpSuffix, "/")))
	before := ts.previewFiles(c)
	c.Assert(before, HasLen, 2)

	// own asset, as the gallery requests it
	ct, sum := ts.getPreview(c, fmt.Sprintf("/preview/%s?%s&icodec=webp&token=%s", ai.Hash, dims, ts.token))
	c.Assert(ct, Equals, "image/webp")
	c.Assert(sum, Equals, fileSHA1(c, pregenerated))
	c.Assert(ts.previewFiles(c), DeepEquals, before)

	// shared to bob, as the inbox requests it
	ts.requestWithMethod(c, "/send/user/2/"+ai.Hash+"?byhash=1&token="+ts.token, "POST", http.StatusOK, nil, nil, nil)
	ct, sum = ts.getPreview(c, fmt.Sprintf("/receive/preview/1?%s&icodec=webp&token=%s", dims, ts.tokenBob))
	c.Assert(ct, Equals, "image/webp")
	c.Assert(sum, Equals, fileSHA1(c, pregenerated))
	c.Assert(ts.previewFiles(c), DeepEquals, before)

	// without icodec both default to JPEG, which is not pre-generated and gets transcoded
	ct, _ = ts.getPreview(c, fmt.Sprintf("/receive/preview/1?%s&token=%s", dims, ts.tokenBob))
	c.Assert(ct, Equals, "image/jpeg")
	after := ts.previewFiles(c)
	c.Assert(len(after), Equals, len(before)+1)
	c.Assert(containsSuffix(after, fmt.Sprintf("/20031101_1_%d_320.jpg", testImgPreviewWidth)), Equals, true,
		Commentf("files: %v", after))

	// an unknown codec is rejected rather than silently served as JPEG
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/receive/preview/1?%s&icodec=nope&token=%s", dims, ts.tokenBob), nil)
	c.Assert(err, IsNil)
	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	c.Assert(rr.Code >= 400, Equals, true, Commentf("status %d", rr.Code))
}

func containsSuffix(files []string, suffix string) bool {
	for _, f := range files {
		if strings.HasSuffix(f, suffix) {
			return true
		}
	}
	return false
}
