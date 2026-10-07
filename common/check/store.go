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

// VerifyAssets tells, for each hash, whether userID's copy is safe to rely on. The caller
// checks the user's mount status first and reports VerifyUnavailable without calling this.
func VerifyAssets(ctx context.Context, tx *sql.Tx, userID int, hashes []string) ([]AssetVerify, *CheckRun, error) {
	run, err := LatestRun(ctx, tx)
	if err != nil {
		return nil, nil, err
	}
	homeDir, err := user.GetHomedir(ctx, tx, userID)
	if err != nil {
		return nil, nil, err
	}
	master, _ := common.GetUserPhotoDir(homeDir)
	if ok, err := common.IsFileExist(master); err != nil || !ok {
		// disk not mounted or library moved: nothing can be confirmed, but nothing is known lost
		results := make([]AssetVerify, len(hashes))
		for i, h := range hashes {
			results[i] = AssetVerify{Hash: h, Status: VerifyUnavailable}
		}
		return results, run, nil
	}

	results := make([]AssetVerify, 0, len(hashes))
	for _, h := range hashes {
		v, err := verifyAsset(ctx, tx, userID, strings.ToLower(h), run)
		if err != nil {
			return nil, nil, err
		}
		v.Hash = h
		results = append(results, v)
	}
	return results, run, nil
}

func verifyAsset(ctx context.Context, tx *sql.Tx, userID int, hash string, run *CheckRun) (AssetVerify, error) {
	var assetID, status int
	var uploadTime interface{}
	err := tx.QueryRowContext(ctx, "select id, status, upload_time from asset where user_id = ? and hash = ?",
		userID, hash).Scan(&assetID, &status, &uploadTime)
	if common.IsErrNoRows(err) {
		return AssetVerify{Status: VerifyNotFound}, nil
	}
	if err != nil {
		return AssetVerify{}, err
	}
	if types.IsAssetStatus(status, types.AssetStatusScanLink) {
		return AssetVerify{Status: VerifyLinked}, nil
	}

	masterFile, _, _, extID, err := asset.GetAssetPath(ctx, tx, userID, assetID, 0)
	if err != nil {
		return AssetVerify{}, err
	}
	if !nonEmptyFile(masterFile) {
		return AssetVerify{Status: VerifyFileMissing}, nil
	}
	if extID == ext.ZIP {
		prefix := strings.TrimSuffix(masterFile, "."+ext.ZIPString)
		if !nonEmptyFile(ext.MkLivePhotoImageNameJPG(prefix)) && !nonEmptyFile(ext.MkLivePhotoImageNameHEIC(prefix)) {
			return AssetVerify{Status: VerifyFileMissing}, nil
		}
	}

	if run != nil && int64(assetID) <= run.MaxAssetID {
		var n int
		if err := tx.QueryRowContext(ctx, "select count(*) from ccheck_bad where run_id = ? and asset_id = ?",
			run.ID, assetID).Scan(&n); err != nil {
			return AssetVerify{}, err
		}
		if n > 0 {
			return AssetVerify{Status: VerifyBad}, nil
		}
		return AssetVerify{Status: VerifyOK, Evidence: EvidenceCCheck, CheckedAt: run.StartTime}, nil
	}
	return AssetVerify{Status: VerifyOK, Evidence: EvidenceUpload, CheckedAt: dbTimeUnix(uploadTime)}, nil
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
