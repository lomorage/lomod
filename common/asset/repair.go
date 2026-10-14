package asset

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/sirupsen/logrus"
)

// Repair is an uploaded asset whose DB row exists, used to tell whether its file in the library
// is gone and, if so, to put an upload of the same content back in its place. The asset keeps
// its id, name, albums and metadata; only the file comes back.
type Repair struct {
	AssetID    int
	ExtID      int
	MasterFile string
	PreviewDir string
	homeDir    string
	date       time.Time
}

// LookupRepair finds userid's asset with this hash. nil when there's none, or when it's a
// scanned-in link (its file lives in the user's own folder, not ours to restore). DB only; call
// Missing after the transaction.
func LookupRepair(ctx context.Context, tx *sql.Tx, userid int, hash string) (*Repair, error) {
	var id, y, m, d, extID, status int
	var created interface{}
	err := tx.QueryRowContext(ctx,
		"select id, year, month, day, ext_id, status, create_time from asset where user_id = ? and hash = ?",
		userid, strings.ToLower(hash)).Scan(&id, &y, &m, &d, &extID, &status, &created)
	if common.IsErrNoRows(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	if types.IsAssetStatus(status, types.AssetStatusScanLink) {
		return nil, nil
	}
	homeDir, err := user.GetHomedir(ctx, tx, userid)
	if err != nil {
		return nil, err
	}
	extension, err := ext.GetExtString(extID)
	if err != nil {
		return nil, err
	}
	masterDir, previewDir, err := common.GetUserPhotoMasterPreviewDir(homeDir, y, m, d, 0)
	if err != nil {
		return nil, err
	}
	date, ok := created.(time.Time)
	if !ok {
		date = time.Date(y, time.Month(m), d, 12, 0, 0, 0, time.Local)
	}
	return &Repair{
		AssetID:    id,
		ExtID:      extID,
		MasterFile: filepath.Join(masterDir, ext.MkAssetName(ext.NormalizeAssetName(y, m, d, id), extension)),
		PreviewDir: previewDir,
		homeDir:    homeDir,
		date:       date,
	}, nil
}

// Missing reports whether the asset's file (for a live photo: the video or its still image) is
// gone or empty. A missing library root means the disk isn't there right now, not that the file
// is lost, so that is never "missing": an upload must not refill an empty mount point.
func (r *Repair) Missing() bool {
	master, _ := common.GetUserPhotoDir(r.homeDir)
	if ok, err := common.IsFileExist(master); err != nil || !ok {
		return false
	}
	if !nonEmptyFile(r.MasterFile) {
		return true
	}
	if r.ExtID == ext.ZIP {
		prefix := strings.TrimSuffix(r.MasterFile, "."+ext.ZIPString)
		return !nonEmptyFile(ext.MkLivePhotoImageNameJPG(prefix)) && !nonEmptyFile(ext.MkLivePhotoImageNameHEIC(prefix))
	}
	return false
}

// Restore moves an uploaded, SHA-verified file (and a live photo's still image) to where the
// asset's file belongs. Returns the path to make previews from.
func (r *Repair) Restore(srcFile, srcImgFile string, folderPerm os.FileMode) (string, error) {
	masterDir := filepath.Dir(r.MasterFile)
	if err := os.MkdirAll(masterDir, folderPerm); err != nil {
		return "", err
	}
	if err := os.MkdirAll(r.PreviewDir, folderPerm); err != nil {
		return "", err
	}
	if err := moveAsset(srcFile, r.MasterFile, r.date); err != nil {
		return "", err
	}
	previewSrc := r.MasterFile
	if r.ExtID == ext.ZIP {
		prefix := strings.TrimSuffix(r.MasterFile, "."+ext.ZIPString)
		previewSrc = prefix + "_image" + filepath.Ext(srcImgFile)
		if err := moveAsset(srcImgFile, previewSrc, r.date); err != nil {
			return "", err
		}
	}
	if err := common.SyncDir(masterDir); err != nil {
		logrus.Warnf("sync dir %s: %v", masterDir, err)
	}
	logrus.Infof("restored missing file of asset %d: %s", r.AssetID, r.MasterFile)
	return previewSrc, nil
}

func nonEmptyFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular() && fi.Size() > 0
}
