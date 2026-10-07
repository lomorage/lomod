package check

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/asset"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/user"
)

// Verify statuses returned to clients deciding whether a local original can be deleted.
const (
	VerifyOK          = "ok"           // stored in the library and the file is there
	VerifyNotFound    = "not_found"    // no asset with this hash for the user
	VerifyLinked      = "linked"       // only indexed from a user folder, not copied into the library
	VerifyFileMissing = "file_missing" // DB record exists but the file is missing or empty
	VerifyBad         = "bad"          // the latest consistency check could not confirm it
	VerifyUnavailable = "unavailable"  // the user's storage can't be read right now
)

// Evidence behind VerifyOK.
const (
	EvidenceUpload = "upload" // SHA1-checked when it was written (asset.save)
	EvidenceCCheck = "ccheck" // re-hashed by a completed consistency check
)

// CheckRun is one completed or in-progress consistency check run.
type CheckRun struct {
	ID         int64
	StartTime  int64
	EndTime    int64
	MaxAssetID int64
}

// AssetVerify is the verify result for one hash.
type AssetVerify struct {
	Hash      string `json:"hash"`
	Status    string `json:"status"`
	Evidence  string `json:"evidence,omitempty"`
	CheckedAt int64  `json:"checkedAt,omitempty"`
}

// BeginRun records the start of a consistency check over every asset with id <= maxAssetID,
// and drops runs that never finished (crash or power loss mid-check).
func BeginRun(ctx context.Context, tx *sql.Tx, start time.Time, maxAssetID int) (int64, error) {
	if _, err := tx.ExecContext(ctx,
		"delete from ccheck_bad where run_id in (select id from ccheck_run where end_time is null)"); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, "delete from ccheck_run where end_time is null"); err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, "insert into ccheck_run(start_time, max_asset_id) values(?, ?)",
		start.Unix(), maxAssetID)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// FinishRun stores the assets a run could not confirm, marks it complete and drops older runs,
// all in the caller's transaction so a crash leaves either the old or the new result.
func FinishRun(ctx context.Context, tx *sql.Tx, runID int64, end time.Time, unverified map[int]int) error {
	stmt, err := tx.PrepareContext(ctx, "insert into ccheck_bad(run_id, asset_id, type) values(?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()
	for assetID, typ := range unverified {
		if _, err := stmt.ExecContext(ctx, runID, assetID, typ); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, "update ccheck_run set end_time = ? where id = ?", end.Unix(), runID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "delete from ccheck_bad where run_id < ?", runID); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "delete from ccheck_run where id < ?", runID)
	return err
}

// LatestRun returns the latest completed run, or nil if no run has completed.
func LatestRun(ctx context.Context, tx *sql.Tx) (*CheckRun, error) {
	run := CheckRun{}
	err := tx.QueryRowContext(ctx,
		"select id, start_time, end_time, max_asset_id from ccheck_run where end_time is not null order by id desc limit 1").
		Scan(&run.ID, &run.StartTime, &run.EndTime, &run.MaxAssetID)
	if common.IsErrNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

// MaxAssetID returns the largest asset ID in a memdb snapshot.
func MaxAssetID(assetsDB map[int]map[int][][][]types.Asset) int {
	maxID := 0
	for _, userAssets := range assetsDB {
		for _, assetsY := range userAssets {
			for _, assetsM := range assetsY {
				for _, assetsD := range assetsM {
					for _, a := range assetsD {
						if id, err := ext.GetAssetIDByName(a.Name); err == nil && id > maxID {
							maxID = id
						}
					}
				}
			}
		}
	}
	return maxID
}

// VerifyLookup is the DB half of a verify request: everything needed to answer it, read in
// one transaction. Check then touches the file system with the transaction already closed,
// because lomod runs SQLite on a single connection and stat on a sleeping USB disk can
// take seconds.
type VerifyLookup struct {
	Run     *CheckRun
	homeDir string
	assets  []assetLookup
}

type assetLookup struct {
	hash       string // as sent by the client
	found      bool
	linked     bool
	masterFile string
	extID      int
	covered    bool // asset id <= Run.MaxAssetID
	bad        bool // in Run's ccheck_bad
	uploadTime int64
}

// LookupAssets reads userID's records for hashes, the latest completed consistency check, and
// which of the assets it flagged.
func LookupAssets(ctx context.Context, tx *sql.Tx, userID int, hashes []string) (*VerifyLookup, error) {
	run, err := LatestRun(ctx, tx)
	if err != nil {
		return nil, err
	}
	homeDir, err := user.GetHomedir(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	l := &VerifyLookup{Run: run, homeDir: homeDir, assets: make([]assetLookup, len(hashes))}
	coveredIDs := map[int][]int{} // asset id -> indexes in l.assets (a hash may be sent twice)
	for i, h := range hashes {
		a := &l.assets[i]
		a.hash = h
		var assetID, status int
		var uploadTime interface{}
		err := tx.QueryRowContext(ctx, "select id, status, upload_time from asset where user_id = ? and hash = ?",
			userID, strings.ToLower(h)).Scan(&assetID, &status, &uploadTime)
		if common.IsErrNoRows(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		a.found = true
		if types.IsAssetStatus(status, types.AssetStatusScanLink) {
			a.linked = true
			continue
		}
		a.masterFile, _, _, a.extID, err = asset.GetAssetPath(ctx, tx, userID, assetID, 0)
		if err != nil {
			return nil, err
		}
		a.uploadTime = dbTimeUnix(uploadTime)
		if run != nil && int64(assetID) <= run.MaxAssetID {
			a.covered = true
			coveredIDs[assetID] = append(coveredIDs[assetID], i)
		}
	}

	if len(coveredIDs) > 0 {
		ids := make([]interface{}, 0, len(coveredIDs)+1)
		ids = append(ids, run.ID)
		for id := range coveredIDs {
			ids = append(ids, id)
		}
		rows, err := tx.QueryContext(ctx, "select asset_id from ccheck_bad where run_id = ? and asset_id in (?"+
			strings.Repeat(",?", len(coveredIDs)-1)+")", ids...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var id int
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			for _, i := range coveredIDs[id] {
				l.assets[i].bad = true
			}
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return l, nil
}

// Check answers each looked-up hash, in request order. Call it after the lookup's transaction
// is closed: it stats files.
func (l *VerifyLookup) Check() []AssetVerify {
	results := make([]AssetVerify, len(l.assets))
	master, _ := common.GetUserPhotoDir(l.homeDir)
	if ok, err := common.IsFileExist(master); err != nil || !ok {
		// disk not mounted or library moved: nothing can be confirmed, but nothing is known lost
		for i, a := range l.assets {
			results[i] = AssetVerify{Hash: a.hash, Status: VerifyUnavailable}
		}
		return results
	}
	for i, a := range l.assets {
		results[i] = l.check(a)
		results[i].Hash = a.hash
	}
	return results
}

func (l *VerifyLookup) check(a assetLookup) AssetVerify {
	switch {
	case !a.found:
		return AssetVerify{Status: VerifyNotFound}
	case a.linked:
		return AssetVerify{Status: VerifyLinked}
	case !nonEmptyFile(a.masterFile):
		return AssetVerify{Status: VerifyFileMissing}
	}
	if a.extID == ext.ZIP {
		prefix := strings.TrimSuffix(a.masterFile, "."+ext.ZIPString)
		if !nonEmptyFile(ext.MkLivePhotoImageNameJPG(prefix)) && !nonEmptyFile(ext.MkLivePhotoImageNameHEIC(prefix)) {
			return AssetVerify{Status: VerifyFileMissing}
		}
	}
	if a.covered {
		if a.bad {
			return AssetVerify{Status: VerifyBad}
		}
		return AssetVerify{Status: VerifyOK, Evidence: EvidenceCCheck, CheckedAt: l.Run.StartTime}
	}
	return AssetVerify{Status: VerifyOK, Evidence: EvidenceUpload, CheckedAt: a.uploadTime}
}

func nonEmptyFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular() && fi.Size() > 0
}

// dbTimeUnix converts a timestamp column written through go-sqlite3 (time.Time is stored
// as text) to unix seconds; 0 if it can't be parsed.
func dbTimeUnix(v interface{}) int64 {
	switch t := v.(type) {
	case time.Time:
		return t.Unix()
	case int64:
		return t
	case []byte:
		return parseDBTime(string(t))
	case string:
		return parseDBTime(t)
	}
	return 0
}

func parseDBTime(s string) int64 {
	for _, layout := range []string{
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02T15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02T15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Unix()
		}
	}
	return 0
}
