package handler

import (
	"fmt"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	. "gopkg.in/check.v1"
)

func (ts *mainSuite) getJPEGSize(c *C, url string) (int, int) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	c.Assert(err, IsNil)
	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	validateStatusCode(c, url, rr, http.StatusOK)
	cfg, err := jpeg.DecodeConfig(rr.Result().Body)
	c.Assert(err, IsNil, Commentf("%s did not return a jpeg", url))
	return cfg.Width, cfg.Height
}

// iPhone HEIC (HEVC) photos get previews and full-size transcodes on every platform. libvips'
// Windows build can't decode HEVC, so there this goes through the ffmpeg fallback.
func (ts *mainSuite) TestHEICPreviewAndTranscode(c *C) {
	c.Assert(os.RemoveAll(ts.previewRoot()), IsNil)
	for _, a := range []struct {
		file, date, day string
		idx             int
		fullW, fullH    int
	}{
		// 4032x3024 stored, irot 90 CW: displayed portrait
		{"14_2017_09_13.heic", "2017-09-13T00:01:00Z", "2017/09/13/20170913", 1, 3024, 4032},
		// 4032x3024 stored, rotated 180: stays landscape
		{"12_2014_01_21.heic", "2014-01-21T00:01:00Z", "2014/01/21/20140121", 2, 4032, 3024},
	} {
		f := "../cmd/lomod/test/img/" + a.file
		sha := fileSHA1(c, f)
		ai := ts.importAsset(c, f, fmt.Sprintf("/asset/%s?token=%s&ext=heic&createtime=%s", sha, ts.token, a.date))
		c.Assert(ai.Hash, Equals, sha)

		// background generation after upload
		ts.waitPreviewFile(c, fmt.Sprintf("/%s_%d_%d_320.webp", a.day, a.idx, testImgPreviewWidth))
		ts.waitPreviewFile(c, fmt.Sprintf("/%s_%d_75_75.webp", a.day, a.idx))

		// on-demand thumbnail keeps the displayed orientation (width alone means a
		// width x width box, so the long side is 200)
		w, h := ts.getJPEGSize(c, fmt.Sprintf("/preview/%s?width=200&icodec=jpg&token=%s", sha, ts.token))
		if a.fullW > a.fullH {
			c.Assert([]int{w, h}, DeepEquals, []int{200, 150}, Commentf(a.file))
		} else {
			c.Assert([]int{w, h}, DeepEquals, []int{150, 200}, Commentf(a.file))
		}

		// full-size transcode
		w, h = ts.getJPEGSize(c, fmt.Sprintf("/asset/%s?icodec=jpg&token=%s", sha, ts.token))
		c.Assert([]int{w, h}, DeepEquals, []int{a.fullW, a.fullH}, Commentf(a.file))
	}

	// previewFiles hides dot files, and the decode temp file is one
	c.Assert(filepath.Walk(ts.previewRoot(), func(p string, fi os.FileInfo, err error) error {
		if err == nil && strings.Contains(fi.Name(), ".decoded.") {
			c.Errorf("decode temp file left: %s", p)
		}
		return err
	}), IsNil)
}
