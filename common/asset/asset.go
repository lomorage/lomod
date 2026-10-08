package asset

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/leslie-wang/govips/pkg/vips"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// IsHash check if given ID is hash or not
func IsHash(id string) bool {
	return len(id) == 40
}

// MkAssetNameByTime creates asset name by create time
func MkAssetNameByTime(y, m, d, assetid int) string {
	return fmt.Sprintf("%d%02d%02d_%d", y, m, d, assetid)
}

// MkImageNameFromLivePhoto creates live photo image name with its correct location
func MkImageNameFromLivePhoto(assetpath, assetname string) string {
	return filepath.Join(assetpath, fmt.Sprintf("%s_image", assetname))
}

func findLivePhotoImageFile(base string) (string, error) {
	name := base + "_image.jpg"
	ok, err := common.IsFileExist(name)
	if ok && err == nil {
		return name, nil
	}
	name = base + "_image.heic"
	ok, err = common.IsFileExist(name)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("live photo %s hasn't master image file", base)
	}
	return name, nil
}

// ParseAssetID remove asset extension, and convert to integer
func ParseAssetID(aid string) (int, error) {
	return strconv.Atoi(strings.Split(aid, ".")[0])
}

func getassetpath(ctx context.Context, tx *sql.Tx, userid, assetid int, folderPerm os.FileMode) (string, string, string, int, error) {
	stmt, err := tx.Prepare("select year, month, day, user_id, ext_id, status from asset where asset.id = ?")
	if err != nil {
		return "", "", "", 0, err
	}
	defer stmt.Close()
	var y, m, d, userID, extID, status int
	if err := stmt.QueryRowContext(ctx, assetid).Scan(&y, &m, &d, &userID, &extID, &status); err != nil {
		return "", "", "", 0, err
	}

	if userid != 0 && userid != userID {
		return "", "", "", 0, errors.Errorf("Asset user belongs to %d, but request user is %d", userID, userid)
	}

	basedir, err := user.GetHomedir(ctx, tx, userID)
	if err != nil {
		return "", "", "", 0, err
	}

	masterBaseDir, previewBaseDir, err := common.GetUserPhotoMasterPreviewDir(basedir, y, m, d, folderPerm)
	if err != nil {
		return "", "", "", 0, err
	}
	if types.IsAssetStatus(status, types.AssetStatusScanLink) {
		// this is symbol link asset, and need find original folder
		var path, origFilename string

		err = tx.QueryRowContext(ctx,
			"select a.title, aa.orig_filename from asset_album as aa inner join album as a on aa.album_id = a.id where aa.asset_id=? and a.author=? and a.user_id=?",
			assetid, common.AlbumAuthorInternal, userID).Scan(&path, &origFilename)
		return filepath.Join(path, origFilename), previewBaseDir, ext.NormalizeAssetName(y, m, d, assetid), extID, err
	}
	extension, err := ext.GetExtString(extID)
	if err != nil {
		return "", "", "", 0, err
	}
	assetName := ext.NormalizeAssetName(y, m, d, assetid)
	return filepath.Join(masterBaseDir, ext.MkAssetName(assetName, extension)),
		previewBaseDir, assetName, extID, err
}

func getassetpathAndTrashPath(ctx context.Context, tx *sql.Tx, userid, assetid int,
	folderPerm os.FileMode) (string, string, string, string, int, int, error) {
	basedir, err := user.GetHomedir(ctx, tx, userid)
	if err != nil {
		return "", "", "", "", 0, 0, err
	}
	stmt, err := tx.Prepare("select year, month, day, ext_id, status from asset where asset.id = ?")
	if err != nil {
		return "", "", "", "", 0, 0, err
	}
	defer stmt.Close()
	var y, m, d, extID, status int
	if err := stmt.QueryRowContext(ctx, assetid).Scan(&y, &m, &d, &extID, &status); err != nil {
		return "", "", "", "", 0, 0, err
	}

	masterBaseDir, previewBaseDir, err := common.GetUserPhotoMasterPreviewDir(basedir, y, m, d, folderPerm)
	if err != nil {
		return "", "", "", "", 0, 0, err
	}
	trashDir, err := common.GetUserPhotoTrashDir(basedir, y, m, d, folderPerm)
	return masterBaseDir, previewBaseDir, trashDir, ext.NormalizeAssetName(y, m, d, assetid), extID,
		status, err
}

// GetAssetPath resolves an asset's master file path, preview directory, preview filename
// prefix, and extension ID via DB lookups only -- no filesystem or transcode work. Callers
// that need to generate a preview and want to avoid holding a DB transaction across that
// (potentially slow) work should call this inside their transaction, then call
// GenerateAssetPreview afterwards, outside it. See GetAssetMasterPreviewPath for the
// combined, transaction-spanning convenience version most callers still want.
func GetAssetPath(ctx context.Context, tx *sql.Tx, userid, assetid int, folderPerm os.FileMode) (
	string, string, string, int, error) {
	return getassetpath(ctx, tx, userid, assetid, folderPerm)
}

// GenerateAssetPreview resolves (generating if necessary) the preview file for an asset whose
// master/preview paths have already been looked up via GetAssetPath. It touches only the
// filesystem and, on a cache miss, an external transcoder -- no database access -- so it's
// safe to call outside of a DB transaction.
func GenerateAssetPreview(ctx context.Context, masterFile, previewPath, assetPreviewPrefix string,
	assetid, extID int, width, height uint, previewCodec *int, runner types.PreviewRunner,
	folderPerm os.FileMode) (string, string, error) {
	_, err := os.Stat(masterFile)
	if err != nil {
		if os.IsNotExist(err) {
			logrus.Warnf("unable to locate asset %d's original file: %s", assetid, masterFile)
			return "", "", common.ErrNotExistMaster
		}
		return "", "", err
	}

	if previewCodec == nil {
		if !ext.IsVideoFileByID(extID) {
			return masterFile, "", nil
		}
		// not JITT generate because video file take longer time, let caller decide
		previewFile := filepath.Join(previewPath, ext.MkPreviewVideoAssetName(assetPreviewPrefix, width, height))
		return masterFile, previewFile, nil
	}

	newExt, err := ext.GetExtString(*previewCodec)
	if err != nil {
		return "", "", err
	}

	// TODO: live photo transcoding need support
	if width == 0 && height == 0 {
		previewFile := filepath.Join(previewPath, assetPreviewPrefix+"."+newExt)
		return masterFile, previewFile, XcodeImage(ctx, masterFile, previewFile, folderPerm)
	}

	refFile := masterFile
	if extID == ext.ZIP {
		refFile, err = findLivePhotoImageFile(strings.TrimSuffix(masterFile, filepath.Ext(masterFile)))
		if err != nil {
			return "", "", err
		}
	}

	if newExt == ext.MP4String {
		previewFile, err := runner.GeneratePreviewByPath(ctx, folderPerm, types.PreviewRequest{
			MasterPath: refFile, PreviewPath: previewPath, Width: width, Height: height,
			GenVideo: true})
		return masterFile, previewFile, err
	}

	previewFile, err := runner.GeneratePreviewByPath(ctx, folderPerm, types.PreviewRequest{
		MasterPath: refFile, PreviewPath: previewPath, Width: width, Height: height,
		IsWebp: newExt == ext.WebPString})
	return masterFile, previewFile, err
}

// GetAssetMasterPreviewPath returns asset's master and preview path. It holds tx for the
// duration of any preview generation -- fine for callers that already run inside a short-lived
// transaction and don't expect a cache miss, but see GetAssetPath/GenerateAssetPreview for
// hot paths (like on-demand preview serving) where that transcode can be slow and holding a
// DB transaction across it would starve unrelated requests.
func GetAssetMasterPreviewPath(ctx context.Context, tx *sql.Tx, userid, assetid int, width, height uint,
	previewCodec *int, runner types.PreviewRunner, folderPerm os.FileMode) (string, string, error) {
	masterFile, previewPath, assetPreviewPrefix, extID, err := getassetpath(ctx, tx, userid, assetid, folderPerm)
	if err != nil {
		if common.IsErrNoRows(err) {
			err = common.ErrNotExistAsset
		}
		return "", "", err
	}
	return GenerateAssetPreview(ctx, masterFile, previewPath, assetPreviewPrefix, assetid, extID,
		width, height, previewCodec, runner, folderPerm)
}

// GetAssetHashByID return asset hash and extension id by its ID
func GetAssetHashByID(ctx context.Context, tx *sql.Tx, userid, assetID int) (string, int, error) {
	sqlStatement := ""
	if userid == 0 {
		sqlStatement = "select hash, ext_id from asset where id = ?"
	} else {
		sqlStatement = "select hash, ext_id from asset where user_id = ? and id = ?"
	}
	stmt, err := tx.Prepare(sqlStatement)
	if err != nil {
		return "", -1, err
	}
	defer stmt.Close()
	var (
		hash  string
		extID int
	)
	if userid == 0 {
		err = stmt.QueryRowContext(ctx, assetID).Scan(&hash, &extID)
	} else {
		err = stmt.QueryRowContext(ctx, userid, assetID).Scan(&hash, &extID)
	}
	if err != nil {
		if common.IsErrNoRows(err) {
			return "", -1, common.ErrAssetNotExistForUser
		}
		return "", -1, err
	}

	return hash, extID, nil
}

// GetAssetByHash return asset's metadata by its hash
func GetAssetByHash(ctx context.Context, tx *sql.Tx, userid int, assetHash string) (*types.Asset, error) {
	return getAsset(ctx, tx, true, userid, " and a.hash = ?", assetHash)
}

// GetAssetByID return asset's metadata by its ID
func GetAssetByID(ctx context.Context, tx *sql.Tx, userID, assetID int) (*types.Asset, error) {
	return getAsset(ctx, tx, true, userID, " and a.id = ?", assetID)
}

// GetAssetExtensionByID return asset's name by its ID
func GetAssetExtensionByID(ctx context.Context, tx *sql.Tx, assetID interface{}) (e int, err error) {
	err = tx.QueryRowContext(ctx, "select ext_id from asset where id = ?", assetID).Scan(&e)
	return
}

// NormalizeAssetID validate and convert asset ID to int
func NormalizeAssetID(ctx context.Context, tx *sql.Tx, userid int, id string,
	typ types.AssetIDType) (int, int, error) {
	if typ == types.Hash {
		return GetAssetIDByHash(ctx, tx, userid, id)
	}
	assetid, err := ParseAssetID(id)
	if err != nil {
		return -1, -1, err
	}
	_, extid, err := GetAssetHashByID(ctx, tx, userid, assetid)
	if err != nil {
		return -1, -1, err
	}
	return assetid, extid, nil
}

// parseAssetID returns the numeric ID of an asset name like "123.jpg" or "123".
func parseAssetID(name string) (int, error) {
	id, err := strconv.Atoi(strings.Split(name, ".")[0])
	if err != nil {
		return 0, common.ErrBadRequest
	}
	return id, nil
}

// parseAssetIDs parses asset names into IDs, and joins them for an SQL "in (...)" list.
// The IDs are integers, so the list is safe to put into the statement.
func parseAssetIDs(names []string) ([]int, string, error) {
	ids := make([]int, 0, len(names))
	strs := make([]string, 0, len(names))
	for _, n := range names {
		id, err := parseAssetID(n)
		if err != nil {
			return nil, "", err
		}
		ids = append(ids, id)
		strs = append(strs, strconv.Itoa(id))
	}
	return ids, strings.Join(strs, ","), nil
}

// placeholders returns "?,?,..." with n placeholders.
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// ValidateAssetIDs validate and return existed and not exist asset IDs in the system separately
func ValidateAssetIDs(ctx context.Context, tx *sql.Tx, userid int, assetIDs []string) ([]int,
	[]string, error) {
	names := []string{}
	hashs := []interface{}{}
	existIDs := []int{}
	notExistIDs := map[string]struct{}{}
	notExistHashs := map[string]struct{}{}
	for _, id := range assetIDs {
		if IsHash(id) {
			hashs = append(hashs, id)
			notExistHashs[id] = struct{}{}
		} else {
			names = append(names, id)
		}
	}
	ids, idList, err := parseAssetIDs(names)
	if err != nil {
		return nil, nil, err
	}
	for _, id := range ids {
		notExistIDs[strconv.Itoa(id)] = struct{}{}
	}

	idCondition := ""
	if len(ids) > 0 {
		idCondition = fmt.Sprintf("id in (%s)", idList)
	}
	hashCondition := ""
	if len(hashs) > 0 {
		hashCondition = fmt.Sprintf("hash in (%s)", placeholders(len(hashs)))
	}
	statement := fmt.Sprintf("select id, hash from asset where user_id = %d", userid)
	if idCondition != "" && hashCondition != "" {
		statement += " and (" + idCondition + " or " + hashCondition + ")"
	} else if idCondition != "" {
		statement += " and " + idCondition
	} else {
		statement += " and " + hashCondition
	}
	logrus.Debug(statement)
	stmt, err := tx.Prepare(statement)
	if err != nil {
		return nil, nil, err
	}
	defer stmt.Close()

	rows, err := stmt.QueryContext(ctx, hashs...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	for rows.Next() {
		id := 0
		hash := ""
		if err := rows.Scan(&id, &hash); err != nil {
			return nil, nil, err
		}
		existIDs = append(existIDs, id)
		delete(notExistIDs, strconv.Itoa(id))
		delete(notExistHashs, hash)
	}
	retIDs := []string{}
	for id := range notExistIDs {
		retIDs = append(retIDs, id)
	}
	for id := range notExistHashs {
		retIDs = append(retIDs, id)
	}
	return existIDs, retIDs, rows.Err()
}

// SplitAssetIDs split asset ID into not exist and exist in given album
func SplitAssetIDs(ctx context.Context, tx *sql.Tx, albumID int, assetIDs []string) ([]int,
	[]string, error) {
	existIDs := []int{}
	ids, idList, err := parseAssetIDs(assetIDs)
	if err != nil {
		return nil, nil, err
	}
	notExistIDs := map[string]struct{}{}
	for _, id := range ids {
		notExistIDs[strconv.Itoa(id)] = struct{}{}
	}

	statement := fmt.Sprintf("select asset_id from asset_album where album_id = %d and asset_id in (%s)", albumID,
		idList)
	stmt, err := tx.Prepare(statement)
	if err != nil {
		return nil, nil, err
	}
	defer stmt.Close()

	rows, err := stmt.QueryContext(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	for rows.Next() {
		id := 0
		if err := rows.Scan(&id); err != nil {
			return nil, nil, err
		}
		existIDs = append(existIDs, id)
		delete(notExistIDs, strconv.Itoa(id))
	}
	retIDs := []string{}
	for id := range notExistIDs {
		retIDs = append(retIDs, id)
	}
	return existIDs, retIDs, rows.Err()
}

func getAsset(ctx context.Context, tx *sql.Tx, inclMeta bool, userid int, where string, arg interface{}) (*types.Asset, error) {
	sqlStatement := "select a.id, hash, latitude, longitude, ext_id, device_name, create_time from asset as a inner join device as d on a.device_id = d.id where a.user_id = ? " + where
	stmt, err := tx.Prepare(sqlStatement)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	var (
		dt        string
		id, extID int
	)
	a := &types.Asset{}
	err = stmt.QueryRowContext(ctx, userid, arg).Scan(&id, &a.Hash, &a.Latitude, &a.Longitude, &extID, &a.Device, &dt)
	if err != nil {
		return nil, err
	}

	a.Name, err = ext.MkAssetNameByID(id, extID)
	if err != nil {
		return nil, err
	}

	a.Date, err = types.ParseDBTime(dt)
	if err != nil {
		return nil, err
	}

	if !inclMeta {
		return a, nil
	}

	metas, err := GetMetadatas(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if len(metas) > 0 {
		a.Metadatas = &metas
	}
	return a, nil
}

// GetAssetIDByHash returns asset id and extension id by hash
func GetAssetIDByHash(ctx context.Context, tx *sql.Tx, userid int, hash string) (int, int, error) {
	sqlStatement := ""
	if userid == 0 {
		sqlStatement = "select id, ext_id from asset where hash = ?"
	} else {
		sqlStatement = "select id, ext_id from asset where user_id = ? and hash = ?"
	}

	stmt, err := tx.Prepare(sqlStatement)
	if err != nil {
		return -1, -1, err
	}
	defer stmt.Close()

	var assetID, extID int
	if userid == 0 {
		err = stmt.QueryRowContext(ctx, hash).Scan(&assetID, &extID)
	} else {
		err = stmt.QueryRowContext(ctx, userid, hash).Scan(&assetID, &extID)
	}
	if err != nil {
		if common.IsErrNoRows(err) {
			return -1, -1, common.ErrAssetNotExistForUser
		}
		return -1, -1, err
	}
	return assetID, extID, err
}

// DeleteAsset delete asset
func DeleteAsset(ctx context.Context, tx *sql.Tx, userid, assetid int, force bool, folderPerm os.FileMode) error {
	// delete preview firstly
	logrus.Debugf("delete user %d's asset id: %d", userid, assetid)
	masterPath, previewPath, trashPath, assetName, extID, status, err := getassetpathAndTrashPath(ctx, tx,
		userid, assetid, folderPerm)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(trashPath, folderPerm); err != nil {
		return err
	}

	// delete both default sized preview and specified sized preview
	pfs, err := ioutil.ReadDir(previewPath)
	if err != nil {
		return err
	}
	for _, pf := range pfs {
		if !strings.HasPrefix(pf.Name(), assetName) {
			continue
		}
		logrus.Debugf("delete preview path: %s", pf.Name())
		if err := os.Remove(filepath.Join(previewPath, pf.Name())); err != nil {
			return err
		}
	}

	extension, err := ext.GetExtString(extID)
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare("delete from asset where id = ? ")
	if err != nil {
		return err
	}
	defer stmt.Close()

	if _, err = stmt.ExecContext(ctx, assetid); err != nil {
		return err
	}

	// this asset is not imported by symbol link
	if types.IsAssetStatus(status, types.AssetStatusScanLink) {
		return nil
	}
	masterfile := filepath.Join(masterPath, ext.MkAssetName(assetName, extension))
	ok, err := common.IsFileExist(masterfile)
	if err != nil {
		return err
	}
	if !ok {
		goto checkAndDeleteLivePhoto
	}
	if force {
		if err := os.Remove(masterfile); err != nil {
			// still try to delete live photo file
			if extID == ext.ZIP {
				// move zipball to trash, and delete image file
				if err2 := deleteLivephotoImage(masterfile); err2 != nil {
					if os.IsNotExist(err) && os.IsNotExist(err2) {
						// if both master file and zip file are not exist, return only 1
						logrus.Warnf("both livephoto master file and image file not exist: %s", masterfile)
						return err
					}
					return errors.Wrapf(err2, err.Error())
				}
			}

			return err
		}
	} else {
		trashfile := filepath.Join(trashPath, ext.MkAssetName(assetName, extension))

		logrus.Infof("move asset from %s to %s", masterfile, trashfile)

		if err := os.Rename(masterfile, trashfile); err != nil {
			return err
		}
	}

checkAndDeleteLivePhoto:
	// delete asset from fs
	if extID == ext.ZIP {
		// move zipball to trash, and delete image file
		if err := deleteLivephotoImage(masterfile); err != nil {
			logrus.Infof("delete live photo's image file %s: %v", masterfile, err)
		}
	}
	return nil
}

func deleteLivephotoImage(livephotopath string) error {
	base := strings.TrimSuffix(livephotopath, ".zip")
	imageFilename, err := findLivePhotoImageFile(base)
	if err != nil {
		return errors.Wrapf(err, "while find live photo image file %s", livephotopath)
	}

	if err := os.Remove(imageFilename); err != nil {
		return errors.Wrapf(err, "while removing %s", imageFilename)
	}
	return nil
}

func getCachedFilename(homeDir, hash string) string {
	return filepath.Join(common.GetUserTmpDir(homeDir), hash)
}

// GetPartialUploadContent gets the partial content information. It may hash a large file, so
// callers must not hold a DB transaction across it.
func GetPartialUploadContent(homeDir, hash string) (*types.LastSavedAsset, error) {
	lsa := &types.LastSavedAsset{FinalSHA: hash}
	cachedFilename := getCachedFilename(homeDir, hash)
	fi, err := os.Stat(cachedFilename)
	if err != nil {
		if os.IsNotExist(err) {
			return lsa, nil
		}
		return nil, err
	}
	lsa.CurrSize = fi.Size()

	if lsa.CurrSize == 0 {
		removeResumeState(cachedFilename)
		return lsa, os.RemoveAll(cachedFilename)
	}

	if h := loadResumeState(cachedFilename, lsa.CurrSize); h != nil {
		lsa.CurrSHA = fmt.Sprintf("%x", h.Sum(nil))
		return lsa, nil
	}

	// no usable state (e.g. a temp file from before resume states existed): hash the file once
	// and keep the state, so the PATCH that follows doesn't hash it again
	f, err := os.Open(cachedFilename)
	if err != nil {
		return lsa, err
	}
	defer f.Close()

	h := sha1.New()
	if size, err := io.Copy(h, f); err != nil {
		return lsa, err
	} else if size != lsa.CurrSize {
		return lsa, errors.Errorf("Expect to copy %d bytes, but only copy %d", lsa.CurrSize, size)
	}
	if err := saveResumeState(cachedFilename, lsa.CurrSize, h); err != nil {
		logrus.Warnf("save resume state of %s: %v", cachedFilename, err)
	}

	lsa.CurrSHA = fmt.Sprintf("%x", h.Sum(nil))
	return lsa, nil
}

// XcodeImage transcode image to target file
func XcodeImage(ctx context.Context, assetFile, previewFile string, folderPerm os.FileMode) error {
	if !ext.IsImageFile(assetFile) {
		return errors.Errorf("master asset %s is not image", assetFile)
	} else if !ext.IsImageFile(previewFile) {
		return errors.Errorf("preview asset %s is not image", previewFile)
	}

	logrus.Debugf("start transcode %s -> %s", assetFile, previewFile)
	if fi, err := os.Stat(previewFile); err == nil {
		if fi.Size() != 0 {
			return nil
		}
		// truncated file, remove and recreate it
		if err := os.Remove(previewFile); err != nil {
			logrus.Warnf("remove truncated preview file %s: %s", previewFile, err)
		}
	} else if !os.IsNotExist(err) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(previewFile), folderPerm); err != nil {
		return err
	}

	outImage, err := vips.Vipsload(assetFile)
	if err != nil {
		return err
	}
	defer vips.FreeImage(outImage)

	vips.RemoveImageMetadata(outImage, "jpeg-thumbnail-data")
	vips.RemoveImageMetadata(outImage, "exif-data")
	vips.RemoveImageMetadata(outImage, "xmp-data")
	vips.RemoveImageMetadata(outImage, "iptc-data")
	vips.RemoveImageMetadata(outImage, "icc-profile-data")

	if strings.TrimPrefix(filepath.Ext(previewFile), ".") == ext.JPGString {
		if err := vips.Jpegsave(outImage, previewFile); err != nil {
			logrus.Debugf("error transcode %s -> %s: %s", assetFile, previewFile, err)
			return err
		}
	} else {
		if err := vips.Webpsave(outImage, previewFile); err != nil {
			logrus.Debugf("error transcode %s -> %s: %s", assetFile, previewFile, err)
			return err
		}
	}
	logrus.Debugf("finish transcode %s -> %s", assetFile, previewFile)
	return nil
}

// UpdateAssetTime update asset time and move both master and preview to corresponding location
func UpdateAssetTime(ctx context.Context, tx *sql.Tx, userid, aid, newYear, newMonth, newDay int,
	folderPerm os.FileMode) (*types.Asset, error) {
	basedir, err := user.GetHomedir(ctx, tx, userid)
	if err != nil {
		return nil, err
	}
	a, err := getAsset(ctx, tx, false, userid, " and a.id = ?", aid)
	if err != nil {
		return nil, err
	}

	oldYear := a.Date.Year()
	oldMonth := int(a.Date.Month())
	oldDay := a.Date.Day()
	oldMasterBaseDir, oldPreviewBaseDir, err := common.GetUserPhotoMasterPreviewDir(basedir,
		oldYear, oldMonth, oldDay, folderPerm)
	if err != nil {
		return nil, err
	}
	oldMasterAssetFilename := filepath.Join(oldMasterBaseDir, ext.NormalizeAssetNameString(oldYear,
		oldMonth, oldDay, a.Name))
	_, err = os.Stat(oldMasterAssetFilename)
	if err != nil {
		return nil, err
	}

	// update db record firstly, then move both master and preview
	a.Date.Time = a.Date.AddDate(newYear-oldYear, newMonth-oldMonth, newDay-oldDay)
	err = UpdateCreateTime(ctx, tx, userid, aid, a.Date.Time)
	if err != nil {
		return nil, err
	}

	// move master and return error
	newMasterBaseDir, newPreviewBaseDir, err := common.GetUserPhotoMasterPreviewDir(basedir,
		newYear, newMonth, newDay, folderPerm)
	if err != nil {
		return nil, err
	}
	newMasterAssetFilename := filepath.Join(newMasterBaseDir, ext.NormalizeAssetNameString(newYear,
		newMonth, newDay, a.Name))
	err = os.Rename(oldMasterAssetFilename, newMasterAssetFilename)
	if err != nil {
		return nil, err
	}

	// move preview and log error only
	oldPreviewPrefix := ext.NormalizeAssetName(oldYear, oldMonth, oldDay, aid)
	newPreviewPrefix := ext.NormalizeAssetName(newYear, newMonth, newDay, aid)

	fis, err := ioutil.ReadDir(oldPreviewBaseDir)
	if err != nil {
		logrus.Warnf("read preview directory %s: %s", oldPreviewBaseDir, err)
		return a, nil
	}
	for _, fi := range fis {
		if !strings.HasPrefix(fi.Name(), oldPreviewPrefix) {
			continue
		}
		newName := strings.Replace(fi.Name(), oldPreviewPrefix, newPreviewPrefix, -1)
		src := filepath.Join(oldPreviewBaseDir, fi.Name())
		dst := filepath.Join(newPreviewBaseDir, newName)
		err = os.Rename(src, dst)
		if err != nil {
			logrus.Warnf("move preview file %s -> %s: %s", src, dst, err)
		}
	}
	return a, nil
}

// UpdateCreateTime update asset create time
func UpdateCreateTime(ctx context.Context, tx *sql.Tx, userid, assetID int, t time.Time) error {
	stmt, err := tx.Prepare("update asset set year = ?, month = ?, day = ?, create_time = ? where user_id = ? and id =?")
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.ExecContext(ctx, t.Year(), t.Month(), t.Day(), t, userid, assetID)
	return err
}

// UpdateGeo update asset geo data
func UpdateGeo(ctx context.Context, tx *sql.Tx, userid, assetID int, lat, lon float64) error {
	stmt, err := tx.Prepare("update asset set latitude = ?, longitude = ? where user_id = ? and id =?")
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.ExecContext(ctx, lat, lon, userid, assetID)
	return err
}

type SetAssetStatusReply struct {
	Status          int
	AssetCreateDate time.Time
}

// UpdateAssetsHidden set assets hidden or un-hidden
func UpdateAssetsStatus(ctx context.Context, tx *sql.Tx, uid int, flag types.AssetStatus,
	assetIDs []string, isSet bool) (map[string]SetAssetStatusReply, error) {
	ids, idList, err := parseAssetIDs(assetIDs)
	if err != nil {
		return nil, err
	}
	idMap := map[string]string{}
	for i, id := range ids {
		idMap[strconv.Itoa(id)] = assetIDs[i]
	}
	statement := fmt.Sprintf("select id, status, create_time from asset where user_id=%d and id in (%s)", uid,
		idList)
	rows, err := tx.QueryContext(ctx, statement)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	updateArgs := map[int]int{}
	reply := map[string]SetAssetStatusReply{}
	for rows.Next() {
		var (
			id, status int
			dtStr      string
		)
		err = rows.Scan(&id, &status, &dtStr)
		if err != nil {
			return nil, err
		}

		dt, err := types.ParseDBTime(dtStr)
		if err != nil {
			return nil, err
		}

		newStatus := 0
		if isSet {
			newStatus = types.SetAssetStatus(status, flag)
		} else {
			newStatus = types.UnsetAssetStatus(status, flag)
		}
		updateArgs[id] = newStatus

		aid, ok := idMap[strconv.Itoa(id)]
		if !ok {
			logrus.Warnf("during id remap, unable to find %d in %v", id, idMap)
			continue
		}
		reply[aid] = SetAssetStatusReply{Status: newStatus, AssetCreateDate: dt.Time}
	}
	err = rows.Err()
	if err != nil {
		return nil, err
	}

	for id, status := range updateArgs {
		_, err = tx.ExecContext(ctx, "update asset set status = ? where id = ?", status, id)
		if err != nil {
			return nil, err
		}
	}
	return reply, nil
}

// GetAssetsNotInAlbums returns assets not in any albums yet
func GetAssetsNotInAlbums(ctx context.Context, tx *sql.Tx, userid, page, limit int) (interface{}, int, error) {
	sqlStatement := "from asset where user_id=" + strconv.Itoa(userid) +
		" and id not in (select distinct asset_id from asset_album) "
	return searchAssets(ctx, tx, true,
		"select count(id) "+sqlStatement,
		"select id, ext_id, hash "+sqlStatement+mkPageStatement(page*limit, limit))
}
