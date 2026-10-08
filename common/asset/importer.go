package asset

import (
	"context"
	"database/sql"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func moveAsset(srcFile, assetFilename string, dt time.Time) error {
	stat, err := os.Stat(assetFilename)
	if err != nil && !os.IsNotExist(err) {
		return err
	} else if err == nil {
		if stat.Mode()&os.ModeSymlink != 0 || stat.Size() == 0 {
			if err := os.Remove(assetFilename); err != nil {
				return err
			}
		}
	}
	if err := os.Rename(srcFile, assetFilename); err != nil {
		logrus.Warnf("rename %s to %s: %s", srcFile, assetFilename, err)
		if !strings.Contains(err.Error(), "invalid cross-device link") {
			return err
		}

		// os.Rename can not move cross different disks. Try system command to move file
		err = common.MoveFile(srcFile, assetFilename)
		if err != nil {
			return err
		}
	}
	// change file timestamp to its create time
	if err := os.Chtimes(assetFilename, dt, dt); err != nil {
		logrus.Warnf("change timestamp to %s: %v", dt, err)
	}
	return nil
}

// InsertAsset inserts asset
func InsertAsset(ctx context.Context, tx *sql.Tx, userid, deviceid, extid int,
	a *types.Asset, size, status int, createTime time.Time) (int64, error) {
	stmt, err := tx.Prepare(`insert into 
  asset(user_id, hash, year, month, day, ext_id, device_id, latitude, longitude, size, status, create_time, upload_time)
	values(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	result, err := stmt.ExecContext(ctx, userid, a.Hash, createTime.Year(), createTime.Month(), createTime.Day(),
		extid, deviceid, a.Latitude, a.Longitude, size, status, createTime, time.Now().UTC())
	if err != nil {
		return 0, err
	}

	return result.LastInsertId()
}

// MoveAsset is mainly used to move previous link file to real file
func MoveAsset(ctx context.Context, tx *sql.Tx, userid int, hash string, dt time.Time, folderPerm os.FileMode) error {
	basedir, err := user.GetHomedir(ctx, tx, userid)
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare("select id, year, month, day, ext_id from asset where hash = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()
	var id, y, m, d, extID int
	if err := stmt.QueryRowContext(ctx, hash).Scan(&id, &y, &m, &d, &extID); err != nil {
		return err
	}

	masterBaseDir, _, err := common.GetUserPhotoMasterPreviewDir(basedir, y, m, d, folderPerm)
	if err != nil {
		return err
	}

	name, err := ext.MkAssetNameByID(id, extID)
	if err != nil {
		return err
	}
	masterFilename := filepath.Join(masterBaseDir, ext.NormalizeAssetNameString(y, m, d, name))
	orig, err := os.Readlink(masterFilename)
	if err != nil {
		return err
	}
	if err := os.Remove(masterFilename); err != nil {
		return err
	}
	return moveAsset(orig, masterFilename, dt)
}

// CreateAsset analaysis file and copy it into corresponding location
// device: name of uploaded device name, or remote ip for imported mount device
// os.Rename may encounter "invalid cross-device link" failure, so do copy, then remove old one
func CreateAsset(ctx context.Context, tx *sql.Tx, userid, deviceid, extid int, srcFile, srcImgFile string,
	a *types.Asset, folderPerm os.FileMode, move bool) (string, string, error) {
	createTime := a.Date.Time
	// normalize extension
	extension, err := ext.GetExtString(extid)
	if err != nil {
		return "", "", err
	}
	logrus.Infof("start create asset: %s (%s) for user: %d from %d", srcFile, extension, userid, deviceid)
	// check asset exist or not
	basedir, err := user.GetHomedir(ctx, tx, userid)
	if err != nil {
		return "", "", err
	}
	info, err := os.Stat(srcFile)
	if err != nil {
		return "", "", err
	}

	typ := 0
	if !move {
		typ = 1
	}
	assetid, err := InsertAsset(ctx, tx, userid, deviceid, extid, a, int(info.Size()), typ, createTime)
	if err != nil {
		return "", "", err
	}

	name := ext.NormalizeAssetName(createTime.Year(), int(createTime.Month()), createTime.Day(), int(assetid))
	assetFilename := ext.MkAssetName(name, extension)

	masterdir, previewdir, err := common.GetUserPhotoMasterPreviewDir(basedir, createTime.Year(), int(createTime.Month()), createTime.Day(), folderPerm)
	if err != nil {
		return "", "", err
	}

	// create asset in master dir
	assetpath := filepath.Join(masterdir, assetFilename)
	if move {
		if err := moveAsset(srcFile, assetpath, createTime); err != nil {
			logrus.Warnf("while copy %s, got fail: %v", assetFilename, err)
			return "", "", err
		}
	}

	// create thumbnail
	if extid == ext.ZIP {
		assetpath = MkImageNameFromLivePhoto(masterdir, name) + filepath.Ext(srcImgFile)
		if err := moveAsset(srcImgFile, assetpath, createTime); err != nil {
			logrus.Warnf("while copy live photo image %s, got fail: %v", assetpath, err)
			return "", "", err
		}
	}
	if move {
		// Make the rename(s) durable before the caller commits the DB row and replies, so a
		// power loss can't leave a committed asset whose file is still under its temp name.
		// Some mounts (FUSE, SMB) reject fsync on a directory; that must not fail the upload.
		if err := common.SyncDir(masterdir); err != nil {
			logrus.Warnf("sync dir %s: %v", masterdir, err)
		}
	}
	logrus.Infof("created asset %s: %s @ %d-%d-%d from %s", assetFilename, a.Hash, createTime.Year(), createTime.Month(), createTime.Day(), srcFile)

	a.Name, err = ext.MkAssetNameByID(int(assetid), extid)
	return assetpath, previewdir, err
}

// SaveLivephoto copy data from reader to temp dir, then compare sha with specified data
func SaveLivephoto(basedir string, savedAsset *types.LastSavedAsset, r io.ReadCloser, sha hash.Hash,
	folderPerm, filePerm os.FileMode) (string, string, error) {
	// sha isn't fed while a zip arrives, so no resume state (trackState false)
	cacheFilename, err := save(basedir, savedAsset, r, sha, folderPerm, filePerm, false, func(dst *os.File) (string, error) {
		_, err := io.Copy(dst, r)
		return savedAsset.FinalSHA, err
	})
	var cachedImgFile *os.File
	cachedImgFilename := cacheFilename + "_image"

	hash, err := types.GetLivePhotoFileSHAByContent(cacheFilename, func(name string, imgSHA hash.Hash) (io.Writer, error) {
		e := strings.ToLower(filepath.Ext(name))
		isJPG := false
		switch e {
		case ".jpeg":
			fallthrough
		case ".jpg":
			isJPG = true
			fallthrough
		case ".heic":
			fallthrough
		case ".heif":
			if isJPG {
				cachedImgFilename = cachedImgFilename + ".jpg"
			} else {
				cachedImgFilename = cachedImgFilename + ".heic"
			}
			cachedImgFile, err = os.OpenFile(cachedImgFilename, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, filePerm)
			if err != nil {
				return nil, err
			}
			return io.MultiWriter(imgSHA, cachedImgFile), nil
		default:
			return nil, common.ErrNotImplementedFormat
		}
	})
	if err != nil {
		return "", "", err
	}

	defer cachedImgFile.Close()

	if hash.TotalSHA1 != savedAsset.FinalSHA {
		logrus.Infof("import expect %s, got %s(image: %s,video: %s)", savedAsset.FinalSHA, hash.TotalSHA1, hash.ImageSHA1, hash.VideoSHA1)
		return "", "", common.ErrAssetDiffHash
	}
	return cacheFilename, cachedImgFilename, nil
}

// SaveAsset copy data from reader to specified location, then compare sha with specified data
func SaveAsset(basedir string, savedAsset *types.LastSavedAsset, r io.ReadCloser, sha hash.Hash,
	folderPerm, filePerm os.FileMode) (string, error) {
	return save(basedir, savedAsset, r, sha, folderPerm, filePerm, true, func(dst *os.File) (string, error) {
		size, err := io.Copy(hashingWriter{w: dst, h: sha}, r)
		if err != nil {
			return "", err
		}
		hash := fmt.Sprintf("%x", sha.Sum(nil))

		if size == 0 && hash != savedAsset.FinalSHA {
			return "", common.ErrEmptyAsset
		}

		return hash, nil
	})
}

// hashingWriter hashes exactly the bytes written to w, so the hash always matches the file --
// unlike io.MultiWriter(h, w), which has already hashed what a failed or short write dropped.
type hashingWriter struct {
	w io.Writer
	h hash.Hash
}

func (hw hashingWriter) Write(p []byte) (int, error) {
	n, err := hw.w.Write(p)
	hw.h.Write(p[:n])
	return n, err
}

// save writes r to the user's temp file for savedAsset, resuming it at CurrSize when set.
// With trackState, sha's state is kept next to the temp file whenever the upload ends
// incomplete (see resume_state.go), so the next PATCH or HEAD needn't re-read the file.
func save(basedir string, savedAsset *types.LastSavedAsset, r io.ReadCloser, sha hash.Hash,
	folderPerm, filePerm os.FileMode, trackState bool, calculateFinalSHA func(dst *os.File) (string,
		error)) (filename string, err error) {
	defer func() {
		// set sha to empty for GC
		sha.Reset()
		sha = nil
		r.Close()
	}()

	cachedFilename := getCachedFilename(basedir, savedAsset.FinalSHA)
	flag := os.O_RDWR
	if savedAsset.CurrSize == 0 {
		_, err := common.MkHideDir(filepath.Dir(cachedFilename), folderPerm)
		if err != nil {
			return "", err
		}
		flag = flag | os.O_CREATE | os.O_TRUNC
		// even without trackState: HEAD may have saved one for this file
		removeResumeState(cachedFilename)
	} else {
		fi, err := os.Stat(cachedFilename)
		if err != nil {
			return "", err
		}
		if fi.Size() != savedAsset.CurrSize {
			return "", errors.Errorf("Current cached file size %d is different from specified in request %d", fi.Size(), savedAsset.CurrSize)
		}
	}

	var f *os.File
	f, err = os.OpenFile(cachedFilename, flag, filePerm)
	if err != nil {
		return "", err
	}

	// Runs after the sync and close below (defers run last-registered first).
	var (
		wrote, complete bool
		endOffset       int64
	)
	defer func() {
		if complete {
			// even without trackState: HEAD may have saved one for this file
			removeResumeState(cachedFilename)
			return
		}
		if trackState {
			if !wrote {
				return // failed before writing anything; the existing state still holds
			}
			// sha covers exactly the endOffset bytes now in the file
			fi, serr := os.Stat(cachedFilename)
			if serr != nil || fi.Size() != endOffset || endOffset == 0 {
				removeResumeState(cachedFilename)
				return
			}
			if serr := saveResumeState(cachedFilename, endOffset, sha); serr != nil {
				logrus.Warnf("save resume state of %s: %v", cachedFilename, serr)
				removeResumeState(cachedFilename)
			}
		} else if wrote {
			// the file changed under a state HEAD may have saved; it no longer applies
			removeResumeState(cachedFilename)
		}
	}()

	defer func() {
		// execute fsync only if no error before
		ferr := f.Sync()
		if err == nil {
			err = ferr
		} else if ferr != nil {
			err = errors.Wrapf(err, "sync %s: %v", f.Name(), ferr)
		}
		rerr := f.Close()
		if err == nil {
			err = rerr
		} else if rerr != nil {
			err = errors.Wrapf(err, "close %s: %v", f.Name(), rerr)
		}
	}()

	resumed := false
	if savedAsset.CurrSize != 0 && trackState {
		if st := loadResumeState(cachedFilename, savedAsset.CurrSize); st != nil &&
			fmt.Sprintf("%x", st.Sum(nil)) == savedAsset.CurrSHA {
			if rerr := restoreHash(sha, st); rerr == nil {
				if _, err = f.Seek(0, io.SeekEnd); err != nil {
					return "", err
				}
				resumed = true
			}
		}
	}
	if savedAsset.CurrSize != 0 && !resumed {
		// read until end of current file, so that
		// 1. compare current sha
		// 2. resume the file
		if size, err := io.Copy(sha, f); err != nil {
			return "", err
		} else if size != savedAsset.CurrSize {
			return "", errors.Errorf("Expect to copy %d bytes, but only copy %d", savedAsset.CurrSize, size)
		}

		h := fmt.Sprintf("%x", sha.Sum(nil))
		if h != savedAsset.CurrSHA {
			return "", errors.Errorf("Expect sha %s, but got %s", savedAsset.CurrSHA, h)
		}
	}

	logrus.Infof("start writing file to temp file: %s", cachedFilename)

	var hash string
	wrote = true
	hash, err = calculateFinalSHA(f)
	if pos, serr := f.Seek(0, io.SeekCurrent); serr == nil {
		endOffset = pos
	}
	if err != nil {
		return "", err
	}

	if savedAsset.FinalSHA != hash {
		logrus.Infof("%s expect sha %s, but got %s", cachedFilename, savedAsset.FinalSHA, hash)
		return "", common.ErrAssetDiffHash
	}

	complete = true
	return f.Name(), nil
}
