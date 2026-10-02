package scan

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	. "testing"

	"bitbucket.org/lomoware/lomo-backend/common"
	"github.com/sirupsen/logrus"
	. "gopkg.in/check.v1"
)

// The scan tests build a small, self-contained folder from the repo's test
// media, so the expected counts don't drift when files elsewhere change.

const testMediaDir = "../../cmd/lomod/test"

type scanSuite struct {
	root string
}

var _ = Suite(&scanSuite{})

func TestScanSuite(t *T) {
	TestingT(t)
}

func copyTestFile(c *C, src, dst string) {
	content, err := ioutil.ReadFile(filepath.Join(testMediaDir, src))
	c.Assert(err, IsNil)
	c.Assert(os.MkdirAll(filepath.Dir(dst), 0755), IsNil)
	c.Assert(ioutil.WriteFile(dst, content, 0644), IsNil)
}

// SetUpTest creates:
//
//	trip/a.jpg, trip/b.jpg      images
//	trip/day2/v.mp4             video
//	notes.txt                   not media
//	.hidden/h.jpg               hidden folder (hidden attribute on Windows)
//	bad/zero.jpg                empty file
//	bad/fake.jpg                text with an image extension
//	bad/truncated.webp          first 10 bytes of a real webp
func (ts *scanSuite) SetUpTest(c *C) {
	ts.root = c.MkDir()
	copyTestFile(c, "img/1_2003_01_17.jpg", filepath.Join(ts.root, "trip", "a.jpg"))
	copyTestFile(c, "img/3_2003_11_01.jpg", filepath.Join(ts.root, "trip", "b.jpg"))
	copyTestFile(c, "video/10_2013_08_08.mp4", filepath.Join(ts.root, "trip", "day2", "v.mp4"))
	hidden, err := common.MkHideDir(filepath.Join(ts.root, ".hidden"), 0755)
	c.Assert(err, IsNil)
	copyTestFile(c, "img/5_2003_11_23.jpg", filepath.Join(hidden, "h.jpg"))
	c.Assert(ioutil.WriteFile(filepath.Join(ts.root, "notes.txt"), []byte("not media"), 0644), IsNil)

	bad := filepath.Join(ts.root, "bad")
	c.Assert(os.MkdirAll(bad, 0755), IsNil)
	var webp []byte
	c.Assert(ioutil.WriteFile(filepath.Join(bad, "zero.jpg"), nil, 0644), IsNil)
	c.Assert(ioutil.WriteFile(filepath.Join(bad, "fake.jpg"), []byte("hello-world"), 0644), IsNil)
	webp, err = ioutil.ReadFile(filepath.Join(testMediaDir, "img/11_2014_01_21.webp"))
	c.Assert(err, IsNil)
	c.Assert(ioutil.WriteFile(filepath.Join(bad, "truncated.webp"), webp[:10], 0644), IsNil)
}

// scan runs a full scan and returns the tree and the stats.
func (ts *scanSuite) scan(c *C, conf Config) (*File, Stats) {
	r := NewRunner(logrus.StandardLogger())
	fileCh := make(chan *File)
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		for range fileCh {
		}
	}()
	root, err := r.Start(conf, fileCh)
	close(fileCh)
	<-drained
	c.Assert(err, IsNil)
	c.Assert(root, NotNil)
	return root, r.Stats
}

// mediaFiles lists the files in the scanned tree under f, as slash-separated
// paths relative to it.
func mediaFiles(f *File, prefix string) []string {
	files := []string{}
	for _, child := range f.Children {
		p := prefix + child.Name
		if child.IsDir() {
			files = append(files, mediaFiles(child, p+"/")...)
		} else {
			files = append(files, p)
		}
	}
	sort.Strings(files)
	return files
}

func findFile(f *File, name string) *File {
	for _, child := range f.Children {
		if child.Name == name {
			return child
		}
		if found := findFile(child, name); found != nil {
			return found
		}
	}
	return nil
}

func (ts *scanSuite) TestScanCountsMedia(c *C) {
	root, stats := ts.scan(c, Config{RootFolderName: ts.root})

	c.Assert(stats.TotalImageFiles(), Equals, 2)
	c.Assert(stats.TotalVideoFiles(), Equals, 1)
	c.Assert(stats.TotalMediaFiles(), Equals, 3)
	c.Assert(stats.InProgressImageFiles(), Equals, 2)
	c.Assert(stats.InProgressVideoFiles(), Equals, 1)
	c.Assert(stats.TotalMediaDirs(), Equals, 2) // trip, trip/day2

	c.Assert(mediaFiles(root, ""), DeepEquals, []string{"trip/a.jpg", "trip/b.jpg", "trip/day2/v.mp4"})
	c.Assert(findFile(root, "trip").HasMediaFiles(), Equals, true)
}

func (ts *scanSuite) TestScanReportsBadFiles(c *C) {
	_, stats := ts.scan(c, Config{RootFolderName: ts.root})

	c.Assert(stats.ZeroSizeFiles, DeepEquals, []string{filepath.Join("bad", "zero.jpg")})
	malformed := append([]string{}, stats.MalformFiles...)
	sort.Strings(malformed)
	c.Assert(malformed, DeepEquals, []string{
		filepath.Join("bad", "fake.jpg"),
		filepath.Join("bad", "truncated.webp"),
	})
}

func (ts *scanSuite) TestScanIgnoreVideo(c *C) {
	root, stats := ts.scan(c, Config{RootFolderName: ts.root, IgnoreVideo: true})

	c.Assert(stats.TotalImageFiles(), Equals, 2)
	// the first pass still counts the video it found; the second pass skips it
	c.Assert(stats.TotalVideoFiles(), Equals, 1)
	c.Assert(stats.InProgressVideoFiles(), Equals, 0)
	c.Assert(stats.TotalIgnoreVideoFiles(), Equals, 1)
	c.Assert(mediaFiles(root, ""), DeepEquals, []string{"trip/a.jpg", "trip/b.jpg"})
}

func (ts *scanSuite) TestScanIgnoreFolders(c *C) {
	root, stats := ts.scan(c, Config{RootFolderName: ts.root,
		IgnoreFolders: map[string]struct{}{filepath.Join(ts.root, "trip", "day2"): {}}})

	c.Assert(stats.TotalMediaFiles(), Equals, 2)
	c.Assert(mediaFiles(root, ""), DeepEquals, []string{"trip/a.jpg", "trip/b.jpg"})
}

func (ts *scanSuite) TestScanSkipsHashesThatExist(c *C) {
	root, _ := ts.scan(c, Config{RootFolderName: ts.root})
	a := findFile(root, "a.jpg")
	c.Assert(a, NotNil)
	c.Assert(a.SHA1, NotNil)
	known := *a.SHA1

	root, stats := ts.scan(c, Config{RootFolderName: ts.root,
		HashExist: func(h string) bool { return h == known }})
	c.Assert(stats.TotalDuplicateFiles(), Equals, 1)
	c.Assert(mediaFiles(root, ""), DeepEquals, []string{"trip/b.jpg", "trip/day2/v.mp4"})
}

func (ts *scanSuite) TestLinkByDate(c *C) {
	if runtime.GOOS == "windows" {
		c.Skip("creating symlinks needs extra privileges on Windows")
	}
	root, _ := ts.scan(c, Config{RootFolderName: ts.root})

	linkDir := c.MkDir()
	c.Assert(Link(linkDir, "", "", root, common.DefaultFolderPermission), IsNil)

	links := []string{}
	c.Assert(filepath.Walk(linkDir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			return err
		}
		target, err := filepath.EvalSymlinks(p)
		if err != nil {
			return err
		}
		links = append(links, filepath.Base(target))
		return nil
	}), IsNil)
	sort.Strings(links)
	c.Assert(links, DeepEquals, []string{"a.jpg", "b.jpg", "v.mp4"})
}
