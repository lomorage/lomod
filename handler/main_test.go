package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	. "testing"

	. "gopkg.in/check.v1"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/check"
	"bitbucket.org/lomoware/lomo-backend/common/cmd"
	"bitbucket.org/lomoware/lomo-backend/common/migrator"
	"bitbucket.org/lomoware/lomo-backend/common/preview"
	"bitbucket.org/lomoware/lomo-backend/common/scan"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"bitbucket.org/lomoware/lomo-backend/migrations/sqls/lomod"
	"github.com/leslie-wang/govips/pkg/vips"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type mainSuite struct {
	h        *Handler
	userdir  string
	token    string
	tokenBob string
	alice    user.User
	bob      user.User
	cancel   context.CancelFunc
}

const (
	mntdir                = "/tmp"
	testUserDir           = "test-basedir"
	photodir              = "/tmp/usbdisk1"
	backupdir             = "/tmp/backup1"
	mdnsName              = "test--123456789"
	mdnsService           = "test--xxxx.tcp"
	mdnsDomain            = "local."
	port                  = 8888
	adminToken            = "1234567"
	testImgPreviewWidth   = 480
	testVideoPreviewWidth = 320
)

var (
	invalidToken = common.ErrResponse{ID: "4", Text: common.ErrInvalidToken.Error()}
)

var _ = Suite(&mainSuite{})

func TestMainSuite(t *T) {
	TestingT(t)
}

func (ts *mainSuite) SetUpSuite(c *C) {
	testMnt = &map[string]*types.MountDir{
		photodir:  {Type: types.MountLocal, Dir: photodir},
		backupdir: {Type: types.MountLocal, Dir: backupdir},
	}
	conf := &Config{
		FilePerm:   common.DefaultFilePermission,
		FolderPerm: common.DefaultFolderPermission,
		MountDir:   mntdir,
	}
	ts.h = &Handler{
		conf:             conf,
		tokenDuration:    720 * time.Hour,
		mu:               &sync.Mutex{},
		castLock:         &sync.Mutex{},
		uploadCh:         make(chan struct{}, 2),
		previewCh:        make(chan struct{}, 2),
		previewWaitCh:    make(chan struct{}, previewQueueDepth(2)),
		backupCh:         make(chan struct{}),
		ccheckCh:         make(chan struct{}),
		userMountLock:    &sync.Mutex{},
		userMountStatus:  map[string]*userMountInfo{},
		userStatus:       map[int]map[int]user.KeepaliveStatus{},
		scanImportResult: importResult{},
	}
	ts.h.previewPendingJobCheckTimeout = time.Second
	ts.alice = user.User{
		ID:       1,
		Name:     "alice",
		Password: "alice123",
		Phone:    "alice_phone",
		Email:    "alice_email",
		NickName: "alice_nick",
		HomeDir:  photodir + "/alice",
	}
	ts.bob = user.User{
		ID:       2,
		Name:     "bob",
		Password: "bob123",
		Phone:    "bob_phone",
		Email:    "bob_email",
		NickName: "bob_nick",
		HomeDir:  photodir + "/bob",
	}

	if os.Getenv("DEBUG") != "" {
		ts.h.conf.Debug = true
	}
	logrus.SetLevel(logrus.DebugLevel)
	c.Assert(ts.h.initLog(), IsNil)

	// avoid too many console log
	ts.h.accessLogger.SetOutput(ts.h.accessFile.LogWriter)

	xcode, err := probeTranscodeApp("")
	c.Assert(err, IsNil)
	ts.h.transcodeApp = xcode

	exiftool, err := probeExiftool("")
	c.Assert(err, IsNil)
	ts.h.exiftool = exiftool

	ffprobe, err := probeFFProbe("")
	c.Assert(err, IsNil)
	ts.h.ffprobe = ffprobe

	ts.h.ccheckRunner = check.NewRunner(exiftool, ts.h.checkLogger)
	ts.h.scanRunner = scan.NewRunner(logrus.StandardLogger())
	ts.h.scanRunner.ParallelCount = 1

	logrus.SetFormatter(&logrus.TextFormatter{
		DisableColors: true,
	})

	vips.Startup(nil)
}
func (ts *mainSuite) TearDownSuite(c *C) {
	vips.Shutdown()
}

func (ts *mainSuite) SetUpTest(c *C) {
	// set useMemdb and dimension during setup test because each testcase may override these options
	ts.h.conf.UseMemdb = false
	ts.h.previewDims = []types.Dimension{{Width: 75, Height: 75}, {Width: testImgPreviewWidth, Height: 320}}
	ts.h.userMountStatus = map[string]*userMountInfo{}
	ts.h.userAssetSummary = map[int]map[string]types.AssetSummary{}
	ts.h.userAssetLock = &sync.Mutex{}
	ts.h.lastMaintStartTime = time.Time{}
	ts.h.lastMaintEndTime = time.Time{}

	ts.userdir = filepath.Join(os.TempDir(), testUserDir)
	if runtime.GOOS == "windows" {
		// no sudo (or bot users owning files) on a Windows dev box
		c.Assert(os.RemoveAll(ts.userdir), IsNil)
	} else {
		err := cmd.ExecWithSudo("rm", "-rf", ts.userdir) // user bot tests may create non-pi user. force delete
		c.Assert(err, IsNil)
		c.Assert(cmd.ExecWithSudo("sync"), IsNil)
	}
	c.Assert(os.MkdirAll(ts.userdir, 0755), IsNil)

	testDB, err := os.Create(path.Join(ts.userdir, "assets.db"))
	c.Assert(err, IsNil)
	c.Assert(testDB.Close(), IsNil)

	ts.h.conf.BaseDir = ts.userdir
	ts.resetDB(c, testDB.Name(), false)

	c.Assert(migrator.StartLomod(testDB.Name(), "", lomod.SchemaStatements, common.DefaultFolderPermission), IsNil)

	// clean up data folder
	done := false
	for i := 0; i < 10; i++ {
		if err := os.RemoveAll(photodir); err != nil {
			time.Sleep(time.Second)
			continue
		}
		if err := os.RemoveAll(backupdir); err != nil {
			time.Sleep(time.Second)
			continue
		}
		done = true
		break
	}
	c.Assert(done, Equals, true)

	c.Assert(os.Mkdir(photodir, 0755), IsNil)
	c.Assert(os.Mkdir(backupdir, 0755), IsNil)

	c.Assert(ts.h.loadConf(), IsNil)
	info := ts.listSystemInfo(c, "/system")
	c.Assert(info.SystemStatus, Equals, common.SystemStatusNew)

	ts.h.gCtx, ts.cancel = context.WithCancel(context.Background())

	// create user and login
	err = ts.createUser(ts.alice, "", true)
	c.Assert(err, IsNil)
	ts.token, err = ts.login("alice", "alice123")
	c.Assert(err, IsNil)
	c.Assert(ts.token, Not(Equals), "")

	info = ts.listSystemInfo(c, "/system")
	c.Assert(info.SystemStatus, Equals, common.SystemStatusInited)
	info = ts.listSystemInfo(c, "/system?token="+ts.token)
	c.Assert(info.SystemStatus, Equals, common.SystemStatusInited)

	// TODO: skip this case for now
	//err = ts.createUser("bob", "")
	//c.Assert(err, NotNil)
	//c.Assert(err.Error(), Equals, "got non-200 status code on user create")
	err = ts.createUser(ts.bob, ts.token, true)
	c.Assert(err, IsNil)
	ts.tokenBob, err = ts.login("bob", "bob123")
	c.Assert(err, IsNil)
	c.Assert(ts.tokenBob, Not(Equals), "")

	ts.h.initMonitorMount()

	info = ts.listSystemInfo(c, "/system?token="+ts.tokenBob)
	logrus.Infof("SetUpTest system info: %+v", info)
	c.Assert(info.SystemStatus, Equals, common.SystemStatusInited)

	ts.h.previewVideoDims = []types.Dimension{{Width: testVideoPreviewWidth}}

	// preview runner
	ts.h.previewRunner = preview.NewRunner(ts.h.gCtx, ts.h.transcodeApp, ts.h.ffprobe, ts.h.exiftool,
		common.DefaultFolderPermission, ts.h.previewDims, ts.h.previewVideoDims, 1, nil)
	go ts.h.previewRunner.Start()
}

func (ts *mainSuite) TearDownTest(c *C) {
	ts.h.previewRunner.Stop()
	c.Assert(ts.h.db.Close(), IsNil)
	ts.h.db = nil
	ts.cancel()
	//c.Assert(os.RemoveAll(ts.userdir), IsNil)
}

func (ts *mainSuite) resetDB(c *C, name string, useMemDB bool) {
	if ts.h.db != nil {
		c.Assert(ts.h.db.Close(), IsNil)
	}
	ts.h.conf.DbFilename = name
	c.Assert(ts.h.openSqliteDB(), IsNil)

	ts.h.conf.UseMemdb = useMemDB
	if useMemDB {
		c.Assert(ts.h.openMemDB(), IsNil)
	}
	ts.h.accessLogger.DB = ts.h.db
}

func validateStatusCode(c *C, url string, rr *httptest.ResponseRecorder, expectedStatusCode int) {
	res := rr.Result()
	defer res.Body.Close()
	if res.StatusCode != expectedStatusCode {
		data, _ := ioutil.ReadAll(res.Body)
		c.Fatalf("%s expect %d, got %d(%s)", url, expectedStatusCode, res.StatusCode, string(data))
	}
}

func (ts *mainSuite) requestDeleteByID(c *C, url string) {
	req, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, IsNil)

	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	validateStatusCode(c, url, rr, http.StatusOK)
}

func (ts *mainSuite) requestDelete(c *C, token string, items *types.DeleteAssetItems) {
	url := fmt.Sprintf("/asset?token=%s", token)
	body := &bytes.Buffer{}
	c.Assert(json.NewEncoder(body).Encode(items), Equals, nil)
	req, err := http.NewRequest("DELETE", url, body)
	c.Assert(err, IsNil)

	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	validateStatusCode(c, url, rr, http.StatusOK)
}

func (ts *mainSuite) requestGet(c *C, url string, result, expect interface{}) {
	ts.requestWithMethod(c, url, http.MethodGet, http.StatusOK, result, expect, nil)
}

func (ts *mainSuite) requestWithMethod(c *C, url, method string, status int, result, expect interface{}, expectHeaders map[string]string) {
	ts.requestWithMethodBody(c, url, method, status, nil, result, expect, expectHeaders)
}

func (ts *mainSuite) requestWithMethodBody(c *C, url, method string, status int, body io.Reader, result, expect interface{}, expectHeaders map[string]string) {
	r, err := ts.request(url, method, status, body, expectHeaders)
	c.Assert(err, IsNil)
	defer r.Close()
	content, err := ioutil.ReadAll(r)
	c.Assert(err, IsNil)
	json.Unmarshal(content, result)
	if expect != nil {
		c.Assert(result, DeepEquals, expect, Commentf(string(content)))
	}
}

func (ts *mainSuite) request(url, method string, expectStatus int, body io.Reader, expectHeaders map[string]string) (io.ReadCloser, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)

	res := rr.Result()
	if res.StatusCode != expectStatus {
		defer res.Body.Close()
		data, _ := ioutil.ReadAll(res.Body)
		return nil, errors.Errorf("%s got %d(%s)", url, res.StatusCode, string(data))
	}

	for k, v := range expectHeaders {
		if res.Header.Get(k) != v {
			defer res.Body.Close()
			return nil, errors.Errorf("expect header %s, but not found", k)
		}
	}

	return res.Body, nil
}

func (ts *mainSuite) TestAdminToken(c *C) {
	// admin token should not be used for non user update api
	url := fmt.Sprintf("/category?token=%s", adminToken)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	c.Assert(err, IsNil)

	rr := httptest.NewRecorder()
	ts.h.accessLogger.AdminToken = adminToken
	ts.h.CreateRouter().ServeHTTP(rr, req)
	res := rr.Result()
	defer res.Body.Close()
	c.Assert(res.StatusCode, Equals, http.StatusUnauthorized)
	data, err := ioutil.ReadAll(res.Body)
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, `{"id": "29", "text": "Invalid admin token"}
`)
}
