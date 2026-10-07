package check

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	. "testing"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/asset"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/logger"
	"bitbucket.org/lomoware/lomo-backend/common/migrator"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"bitbucket.org/lomoware/lomo-backend/migrations/sqls/lomod"
	"github.com/sirupsen/logrus"
	. "gopkg.in/check.v1"
)

func TestStore(t *T) {
	TestingT(t)
}

type storeSuite struct {
	db      *sql.DB
	dir     string
	homeDir string
	userID  int
	n       int
}

var _ = Suite(&storeSuite{})

func (s *storeSuite) SetUpTest(c *C) {
	s.dir = c.MkDir()
	dbfile := filepath.Join(s.dir, "lomod.db")
	c.Assert(migrator.StartLomod(dbfile, "", lomod.SchemaStatements, common.DefaultFolderPermission), IsNil)

	var err error
	s.db, err = sql.Open("sqlite3", dbfile)
	c.Assert(err, IsNil)

	s.homeDir = filepath.Join(s.dir, "bob")
	c.Assert(os.MkdirAll(filepath.Join(s.homeDir, common.AppPhoto, common.AppPhotoMasterDir), common.DefaultFolderPermission), IsNil)
	s.inTx(c, func(ctx context.Context, tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `insert into user(user_name, password, phone, email, nick_name, home_dir,
			admin, status, create_time, last_modified_time, last_login_time) values('bob', '', '', '', 'bob', ?, 0, 0, 0, 0, 0)`,
			s.homeDir)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		s.userID = int(id)
		return err
	})
	s.n = 0
}

func (s *storeSuite) TearDownTest(c *C) {
	c.Assert(s.db.Close(), IsNil)
}

func (s *storeSuite) inTx(c *C, fun func(ctx context.Context, tx *sql.Tx) error) {
	c.Assert(dbx.InQuery(s.db, fun), IsNil)
}

// addAsset stores a new library asset the way an upload does and returns its id, hash and file.
func (s *storeSuite) addAsset(c *C, extension string) (int, string, string) {
	s.n++
	content := []byte(fmt.Sprintf("asset %d", s.n))
	hash := fmt.Sprintf("%x", sha1.Sum(content))
	src := filepath.Join(s.dir, fmt.Sprintf("upload%d.%s", s.n, extension))
	c.Assert(ioutil.WriteFile(src, content, 0644), IsNil)
	srcImg := ""
	if extension == ext.ZIPString {
		srcImg = filepath.Join(s.dir, fmt.Sprintf("upload%d_image.jpg", s.n))
		c.Assert(ioutil.WriteFile(srcImg, []byte("image"), 0644), IsNil)
	}
	extID, err := ext.GetExtID(extension)
	c.Assert(err, IsNil)

	a := &types.Asset{Hash: hash, Date: types.LomoTime{Time: time.Date(2003, 11, 23, 20, 0, 0, 0, time.UTC)}}
	var path string
	s.inTx(c, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		path, _, err = asset.CreateAsset(ctx, tx, s.userID, 0, extID, src, srcImg, a, common.DefaultFolderPermission, true)
		return err
	})
	id, err := ext.GetAssetIDByName(a.Name)
	c.Assert(err, IsNil)
	return id, hash, path
}

func (s *storeSuite) verify(c *C, hashes ...string) ([]AssetVerify, *CheckRun) {
	var lookup *VerifyLookup
	s.inTx(c, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		lookup, err = LookupAssets(ctx, tx, s.userID, hashes)
		return err
	})
	results := lookup.Check()
	c.Assert(results, HasLen, len(hashes))
	return results, lookup.Run
}

func (s *storeSuite) completeRun(c *C, start time.Time, maxAssetID int, unverified map[int]int) int64 {
	var runID int64
	s.inTx(c, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		runID, err = BeginRun(ctx, tx, start, maxAssetID)
		if err != nil {
			return err
		}
		return FinishRun(ctx, tx, runID, start.Add(time.Hour), unverified)
	})
	return runID
}

func (s *storeSuite) TestUploadedAssetWithoutAnyCheckIsOKByUpload(c *C) {
	_, hash, _ := s.addAsset(c, "jpg")
	results, run := s.verify(c, hash)
	c.Assert(run, IsNil)
	c.Assert(results[0].Status, Equals, VerifyOK)
	c.Assert(results[0].Evidence, Equals, EvidenceUpload)
	c.Assert(results[0].CheckedAt > time.Now().Add(-time.Hour).Unix(), Equals, true, Commentf("%d", results[0].CheckedAt))
}

func (s *storeSuite) TestUnknownHashIsNotFound(c *C) {
	results, _ := s.verify(c, "0123456789abcdef0123456789abcdef01234567")
	c.Assert(results[0].Status, Equals, VerifyNotFound)
}

func (s *storeSuite) TestHashIsCaseInsensitiveAndEchoedAsSent(c *C) {
	_, hash, _ := s.addAsset(c, "jpg")
	upper := strings.ToUpper(hash)
	results, _ := s.verify(c, upper)
	c.Assert(results[0].Hash, Equals, upper)
	c.Assert(results[0].Status, Equals, VerifyOK)
}

func (s *storeSuite) TestOtherUsersAssetIsNotFound(c *C) {
	_, hash, _ := s.addAsset(c, "jpg")
	var results []AssetVerify
	s.inTx(c, func(ctx context.Context, tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `insert into user(user_name, password, phone, email, nick_name, home_dir,
			admin, status, create_time, last_modified_time, last_login_time) values('eve', '', '', '', 'eve', ?, 0, 0, 0, 0, 0)`,
			s.homeDir)
		if err != nil {
			return err
		}
		eve, _ := res.LastInsertId()
		lookup, err := LookupAssets(ctx, tx, int(eve), []string{hash})
		if err != nil {
			return err
		}
		results = lookup.Check()
		return nil
	})
	c.Assert(results[0].Status, Equals, VerifyNotFound)
}

func (s *storeSuite) TestScanLinkedAssetIsLinked(c *C) {
	hash := "1111111111111111111111111111111111111111"
	s.inTx(c, func(ctx context.Context, tx *sql.Tx) error {
		status := types.SetAssetStatus(0, types.AssetStatusScanLink)
		_, err := asset.InsertAsset(ctx, tx, s.userID, 0, ext.JPG, &types.Asset{Hash: hash}, 10, status, time.Now())
		return err
	})
	results, _ := s.verify(c, hash)
	c.Assert(results[0].Status, Equals, VerifyLinked)
}

func (s *storeSuite) TestMissingOrEmptyFileIsFileMissing(c *C) {
	_, gone, goneFile := s.addAsset(c, "jpg")
	_, empty, emptyFile := s.addAsset(c, "jpg")
	c.Assert(os.Remove(goneFile), IsNil)
	c.Assert(ioutil.WriteFile(emptyFile, nil, 0644), IsNil)

	results, _ := s.verify(c, gone, empty)
	c.Assert(results[0].Status, Equals, VerifyFileMissing)
	c.Assert(results[1].Status, Equals, VerifyFileMissing)
}

func (s *storeSuite) TestLivePhotoNeedsZipAndImage(c *C) {
	_, hash, imgFile := s.addAsset(c, ext.ZIPString)
	results, _ := s.verify(c, hash)
	c.Assert(results[0].Status, Equals, VerifyOK)

	c.Assert(os.Remove(imgFile), IsNil)
	results, _ = s.verify(c, hash)
	c.Assert(results[0].Status, Equals, VerifyFileMissing)
}

func (s *storeSuite) TestLibraryNotMountedIsUnavailable(c *C) {
	_, hash, _ := s.addAsset(c, "jpg")
	c.Assert(os.Rename(s.homeDir, s.homeDir+".unplugged"), IsNil)
	results, _ := s.verify(c, hash, "not-even-a-hash")
	c.Assert(results[0].Status, Equals, VerifyUnavailable)
	c.Assert(results[1].Status, Equals, VerifyUnavailable)
}

func (s *storeSuite) TestCompletedCheckUpgradesEvidenceOrMarksBad(c *C) {
	goodID, good, _ := s.addAsset(c, "jpg")
	badID, bad, _ := s.addAsset(c, "jpg")
	start := time.Date(2026, 10, 1, 2, 0, 0, 0, time.UTC)
	s.completeRun(c, start, badID, map[int]int{badID: AssetMissInFS})
	_, after, _ := s.addAsset(c, "jpg") // uploaded after the check's snapshot

	results, run := s.verify(c, good, bad, after)
	c.Assert(run, NotNil)
	c.Assert(run.StartTime, Equals, start.Unix())
	c.Assert(goodID < badID, Equals, true)
	c.Assert(results[0], DeepEquals, AssetVerify{Hash: good, Status: VerifyOK, Evidence: EvidenceCCheck, CheckedAt: start.Unix()})
	c.Assert(results[1].Status, Equals, VerifyBad)
	c.Assert(results[2].Status, Equals, VerifyOK)
	c.Assert(results[2].Evidence, Equals, EvidenceUpload)
}

func (s *storeSuite) TestSameHashTwiceGetsTheSameAnswer(c *C) {
	id, hash, _ := s.addAsset(c, "jpg")
	s.completeRun(c, time.Now(), id, map[int]int{id: AssetMissInFS})
	results, _ := s.verify(c, hash, hash)
	c.Assert(results[0].Status, Equals, VerifyBad)
	c.Assert(results[1].Status, Equals, VerifyBad)
}

func (s *storeSuite) TestReuploadAfterDeleteIsNotHitByOldBadRecord(c *C) {
	oldID, hash, _ := s.addAsset(c, "jpg")
	s.completeRun(c, time.Now(), oldID, map[int]int{oldID: AssetMissInFS})

	// user deletes the broken asset on the server, then the phone uploads the same photo again
	s.inTx(c, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "delete from asset where id = ?", oldID)
		return err
	})
	s.n-- // same content, same hash
	newID, newHash, _ := s.addAsset(c, "jpg")
	c.Assert(newHash, Equals, hash)
	c.Assert(newID > oldID, Equals, true, Commentf("AUTOINCREMENT must not reuse ids"))

	results, _ := s.verify(c, hash)
	c.Assert(results[0].Status, Equals, VerifyOK)
	c.Assert(results[0].Evidence, Equals, EvidenceUpload)
}

func (s *storeSuite) TestUnfinishedRunIsIgnoredAndCleanedUp(c *C) {
	id, hash, _ := s.addAsset(c, "jpg")
	first := s.completeRun(c, time.Unix(1000, 0), id, nil)

	// a later check starts, then lomod dies (crash / power loss) before FinishRun
	s.inTx(c, func(ctx context.Context, tx *sql.Tx) error {
		_, err := BeginRun(ctx, tx, time.Unix(2000, 0), id)
		return err
	})
	results, run := s.verify(c, hash)
	c.Assert(run.ID, Equals, first)
	c.Assert(results[0].CheckedAt, Equals, int64(1000))

	// the next check clears the abandoned run and, once done, replaces the old result
	third := s.completeRun(c, time.Unix(3000, 0), id, map[int]int{id: AssetMissInFS})
	var runs, bads int
	s.inTx(c, func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "select count(*) from ccheck_run").Scan(&runs); err != nil {
			return err
		}
		return tx.QueryRowContext(ctx, "select count(*) from ccheck_bad").Scan(&bads)
	})
	c.Assert(runs, Equals, 1)
	c.Assert(bads, Equals, 1)
	results, run = s.verify(c, hash)
	c.Assert(run.ID, Equals, third)
	c.Assert(results[0].Status, Equals, VerifyBad)
}

func (s *storeSuite) TestFailedFinishKeepsPreviousResult(c *C) {
	id, hash, _ := s.addAsset(c, "jpg")
	first := s.completeRun(c, time.Unix(1000, 0), id, nil)

	var second int64
	s.inTx(c, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		second, err = BeginRun(ctx, tx, time.Unix(2000, 0), id)
		return err
	})
	err := dbx.InQuery(s.db, func(ctx context.Context, tx *sql.Tx) error {
		if err := FinishRun(ctx, tx, second, time.Unix(2100, 0), map[int]int{id: AssetMissInFS}); err != nil {
			return err
		}
		return fmt.Errorf("crash before commit")
	})
	c.Assert(err, NotNil)

	results, run := s.verify(c, hash)
	c.Assert(run.ID, Equals, first)
	c.Assert(results[0].Status, Equals, VerifyOK)
}

// Runner.Unverified: every DB asset not found on disk with its hash, including the
// early-return paths that the text report only records per user.
func (s *storeSuite) TestRunnerMarksEverythingItCouldNotConfirm(c *C) {
	okID, okHash, _ := s.addAsset(c, "jpg")
	missID, missHash, missFile := s.addAsset(c, "jpg")
	c.Assert(os.Remove(missFile), IsNil)

	// an asset whose whole day directory is gone (e.g. that folder was on a disk that dropped)
	dayGoneID := 1000
	snapshot := map[int]map[int][][][]types.Asset{s.userID: {}}
	put := func(y, m, d, id int, hash string) {
		months, ok := snapshot[s.userID][y]
		if !ok {
			months = make([][][]types.Asset, 12)
			for i := range months {
				months[i] = make([][]types.Asset, 31)
			}
			snapshot[s.userID][y] = months
		}
		months[m-1][d-1] = append(months[m-1][d-1], types.Asset{Name: fmt.Sprintf("%d.jpg", id), Hash: hash})
	}
	put(2003, 11, 23, okID, okHash)
	put(2003, 11, 23, missID, missHash)
	put(2004, 1, 2, dayGoneID, "2222222222222222222222222222222222222222")

	r := NewRunner("", logger.NewCheckLogger(logrus.New()))
	users := []user.User{{ID: s.userID, Name: "bob", HomeDir: s.homeDir}}
	c.Assert(r.Start(users, snapshot), IsNil)

	c.Assert(r.Unverified, DeepEquals, map[int]int{missID: AssetMissInFS, dayGoneID: MasterDirMiss})
	c.Assert(MaxAssetID(snapshot), Equals, dayGoneID)
}

func (s *storeSuite) TestRunnerMarksAllAssetsOfUserWhoseLibraryIsGone(c *C) {
	id, hash, _ := s.addAsset(c, "jpg")
	c.Assert(os.RemoveAll(filepath.Join(s.homeDir, common.AppPhoto)), IsNil)

	months := make([][][]types.Asset, 12)
	for i := range months {
		months[i] = make([][]types.Asset, 31)
	}
	months[10][22] = []types.Asset{{Name: fmt.Sprintf("%d.jpg", id), Hash: hash}}
	snapshot := map[int]map[int][][][]types.Asset{s.userID: {2003: months}}

	r := NewRunner("", logger.NewCheckLogger(logrus.New()))
	c.Assert(r.Start([]user.User{{ID: s.userID, Name: "bob", HomeDir: s.homeDir}}, snapshot), IsNil)
	c.Assert(r.Unverified, DeepEquals, map[int]int{id: UserMissInFS})
}
