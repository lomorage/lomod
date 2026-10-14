package handler

import (
	"bytes"
	"context"
	"crypto/sha1"
	"database/sql"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/asset"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/logger"
	"bitbucket.org/lomoware/lomo-backend/common/migrator"
	"bitbucket.org/lomoware/lomo-backend/common/preview"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/migrations/sqls/lomod"
	"github.com/gorilla/mux"
)

// repairFixture: a lomod Handler on a fresh DB with user bob and one uploaded photo, enough to
// drive HEAD and POST /asset/{sha1} without the whole server (TestMainSuite doesn't run in CI).
type repairFixture struct {
	h       *Handler
	homeDir string
	userID  int
	content []byte
	hash    string
	file    string
}

func newRepairFixture(t *testing.T) *repairFixture {
	dir := t.TempDir()
	dbfile := filepath.Join(dir, "lomod.db")
	if err := migrator.StartLomod(dbfile, "", lomod.SchemaStatements, common.DefaultFolderPermission); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite3", dbfile)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	f := &repairFixture{homeDir: filepath.Join(dir, "bob"), content: []byte("a photo")}
	f.hash = fmt.Sprintf("%x", sha1.Sum(f.content))
	f.h = &Handler{
		conf: &Config{FolderPerm: common.DefaultFolderPermission, FilePerm: 0644},
		db:   db, userMountLock: &sync.Mutex{}, userMountStatus: map[string]*userMountInfo{},
		uploadCh: make(chan struct{}, 1),
		previewRunner: preview.NewRunner(context.Background(), "", "", "", common.DefaultFolderPermission,
			nil, nil, 1, nil),
	}
	if err := os.MkdirAll(filepath.Join(f.homeDir, common.AppPhoto, common.AppPhotoMasterDir), common.DefaultFolderPermission); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "upload.jpg")
	if err := ioutil.WriteFile(src, f.content, 0644); err != nil {
		t.Fatal(err)
	}
	if err := dbx.InQuery(db, func(ctx context.Context, tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `insert into user(user_name, password, phone, email, nick_name, home_dir,
			admin, status, create_time, last_modified_time, last_login_time) values('bob', '', '', '', 'bob', ?, 0, 0, 0, 0, 0)`,
			f.homeDir)
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		f.userID = int(id)
		a := &types.Asset{Hash: f.hash, Date: types.LomoTime{Time: time.Date(2003, 11, 23, 20, 0, 0, 0, time.UTC)}}
		f.file, _, err = asset.CreateAsset(ctx, tx, f.userID, 0, ext.JPG, src, "", a, common.DefaultFolderPermission, true)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *repairFixture) serve(method string, body []byte, handle func(http.ResponseWriter, *http.Request)) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "/asset/"+f.hash+"?ext=jpg&createtime=2003-11-23T20:00:00Z", bytes.NewReader(body))
	r = mux.SetURLVars(r, map[string]string{"sha1": f.hash})
	rec := httptest.NewRecorder()
	wl := logger.NewResponseLogger(rec)
	wl.Userid, wl.Username = f.userID, "bob"
	handle(wl, r)
	return rec
}

func (f *repairFixture) head() int {
	return f.serve(http.MethodHead, nil, f.h.getAssetHashInfo).Code
}

func (f *repairFixture) post() int {
	return f.serve(http.MethodPost, f.content, f.h.createAsset).Code
}

func TestUploadOfAStoredPhotoIsAConflict(t *testing.T) {
	f := newRepairFixture(t)
	if code := f.head(); code != http.StatusOK {
		t.Fatalf("HEAD = %d, want 200", code)
	}
	if code := f.post(); code != http.StatusConflict {
		t.Fatalf("POST = %d, want 409", code)
	}
}

func TestUploadPutsAMissingFileBack(t *testing.T) {
	f := newRepairFixture(t)
	if err := os.Remove(f.file); err != nil {
		t.Fatal(err)
	}
	if code := f.head(); code != http.StatusNotFound {
		t.Fatalf("HEAD = %d, want 404 so clients upload it again", code)
	}
	if code := f.post(); code != http.StatusOK {
		t.Fatalf("POST = %d, want 200", code)
	}
	got, err := ioutil.ReadFile(f.file)
	if err != nil || !bytes.Equal(got, f.content) {
		t.Fatalf("file not restored: %v", err)
	}
	if code := f.head(); code != http.StatusOK {
		t.Fatalf("HEAD after repair = %d, want 200", code)
	}
}

// Disk not there (no library folder, no .lomo): don't ask for, or accept, an upload into the
// empty mount point.
func TestNoRepairWhileTheDiskIsAway(t *testing.T) {
	f := newRepairFixture(t)
	if err := os.RemoveAll(f.homeDir); err != nil {
		t.Fatal(err)
	}
	if code := f.head(); code != http.StatusOK {
		t.Fatalf("HEAD = %d, want 200", code)
	}
	if code := f.post(); code != http.StatusConflict {
		t.Fatalf("POST = %d, want 409", code)
	}
	if _, err := os.Stat(f.homeDir); !os.IsNotExist(err) {
		t.Fatalf("something was written to the missing disk's place: %v", err)
	}
}
