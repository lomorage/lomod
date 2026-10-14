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
// gone or empty. Never while the library's disk isn't there (LibraryDiskPresent): that doesn't
// mean the file is lost, and an upload must not refill an empty mount point.
func (r *Repair) Missing() bool {
	if !LibraryDiskPresent(r.homeDir) {
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
//
// The upload took a while since Missing was asked, so the disk is checked again: if it went away
// meanwhile, folders made now would land on the empty mount point and make the disk look present
// (LibraryDiskPresent) from then on. Without the disk id file, folders are only made below a
// library folder that exists, never the library folder itself.
func (r *Repair) Restore(srcFile, srcImgFile string, folderPerm os.FileMode) (string, error) {
	masterDir := filepath.Dir(r.MasterFile)
	if err := r.mkMasterDir(masterDir, folderPerm); err != nil {
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

func (r *Repair) mkMasterDir(masterDir string, folderPerm os.FileMode) error {
	if !LibraryDiskPresent(r.homeDir) {
		return common.ErrDeviceNotMount
	}
	if nonEmptyFile(filepath.Join(r.homeDir, common.HiddenLomoDir, types.UUIDFilename)) {
		return os.MkdirAll(masterDir, folderPerm) // the disk is known: the library may be recreated
	}
	root := filepath.Join(r.homeDir, common.AppPhoto, common.AppPhotoMasterDir)
	rel, err := filepath.Rel(root, masterDir)
	if err != nil {
		return err
	}
	dir := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) { // year, month, day
		dir = filepath.Join(dir, part)
		// Mkdir, not MkdirAll: fails if the library folder vanished since the check above
		if err := os.Mkdir(dir, folderPerm); err != nil && !os.IsExist(err) {
			return err
		}
	}
	return nil
}

// LibraryDiskPresent reports whether the disk holding a user's library is there: its disk id
// file (.lomo/disk_uuid) or the library folder exists. With neither, the disk is unplugged or not
// mounted: nothing can be said about the files on it, and nothing must be written to the empty
// mount point. With the id file but no library folder, the library was deleted from a disk that
// is there, and uploads may put it back.
//
// Not the .lomo folder itself: the DB backup (handler/replicate.go) writes under it and creates
// it as needed, so on an empty mount point it would reappear on the system disk. lomod writes
// disk_uuid only after finding the user's Photos folder on the disk (handler migrateMntUUID).
func LibraryDiskPresent(homeDir string) bool {
	return nonEmptyFile(filepath.Join(homeDir, common.HiddenLomoDir, types.UUIDFilename)) ||
		isDir(filepath.Join(homeDir, common.AppPhoto, common.AppPhotoMasterDir))
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func nonEmptyFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular() && fi.Size() > 0
}
