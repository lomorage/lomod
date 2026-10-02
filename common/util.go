package common

import (
	"crypto/sha1"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	// HiddenLomoDir is the hidden lomod configuration directory
	HiddenLomoDir = ".lomo"
	// DBBackupDir is the db backup directory
	DBBackupDir = "db"
	// AppDoc is the base directory for documents
	AppDoc = "Documents"
	// AppPhoto is the base directory for photo application
	AppPhoto = "Photos"
	// AppPhotoMasterDir is relative dir to store photo.
	AppPhotoMasterDir = "master"
	// AppPhotoPreviewDir is relative dir to store photo and video preview version.
	AppPhotoPreviewDir = "preview"
	// AppPhotoTrashDir is relative dir to store trash photos.
	AppPhotoTrashDir = "lomodTrash"
	// AppPhotoSharedDir is relative dir to store shared photos.
	AppPhotoSharedDir = "share"

	binSubDir    = "bin"
	docSubDir    = "doc"
	etcSubDir    = "etc"
	certSubDir   = "cert" // etc/cert
	userSubDir   = "home" // home
	varSubDir    = "var"
	logSubDir    = "log"    // var/log
	webdevSubDir = "webdev" // var/log
	lomodTmpDir  = ".lomodTemp"

	// TimeFormatLog is the timestamp format in log file to be complaint with apache access log format.
	TimeFormatLog = "02/Jan/2006:15:04:05 -0700"
	// TimeFormatLomod is the timestamp format in lomod API request.
	TimeFormatLomod = "2006-01-02T15:04:05Z"
	// TimeFormatEXIF is the timestamp format for "CreateDate", "TrackCreateDate", "MediaCreateDate".
	TimeFormatEXIF = "2006:01:02 15:04:05"
	// TimeFormatEXIF2 is the timestamp format for "ContentCreateDate".
	TimeFormatEXIF2 = "UTC 2006-01-02 15:04:05"

	charset = "abcdefghijklmnopqrstuvwxyz0123456789"

	// if-match is http header for partial upload
	ifMatch = "If-Match"
)

var (
	oomRegex = regexp.MustCompile("cannot allocate memory")

	// TimeFormatDBs list potential DB format. It is mainly for backwards compatibility.
	TimeFormatDBs = []string{
		"2006-1-2 15:4:5-07:00",
		"2006-1-2 15:04:05-07:00",
		"2006-01-02 15:04:05-07:00",
	}

	seededRand *rand.Rand = rand.New(rand.NewSource(time.Now().UnixNano()))
)

// RandomStringWithCharset create random string with given length and charset.
func RandomStringWithCharset(length int, charset string) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

// RandomString create random string with given length.
func RandomString(length int) string {
	return RandomStringWithCharset(length, charset)
}

// GetBinDir returns bin dir.
func GetBinDir(basedir string) string {
	return filepath.Join(basedir, binSubDir)
}

// GetDocDir returns doc dir
func GetDocDir(basedir string) string {
	return filepath.Join(basedir, docSubDir)
}

// GetConfDir returns conf dir
func GetConfDir(basedir string) string {
	return filepath.Join(basedir, etcSubDir)
}

// GetCertDir returns certs dir.
func GetCertDir(basedir string) string {
	return filepath.Join(filepath.Join(basedir, etcSubDir), certSubDir)
}

// GetVarDir returns var dir.
func GetVarDir(basedir string) string {
	return filepath.Join(basedir, varSubDir)
}

// GetLogDir returns var dir.
func GetLogDir(basedir string) string {
	return filepath.Join(filepath.Join(basedir, varSubDir), logSubDir)
}

// IsReservedBaseDirName reports whether name is one of the subdirectories
// lomod itself manages directly under BaseDir (var, etc, bin, doc, and home
// -- the last used to nest Samba/bot-user home dirs). On Windows/macOS,
// where BaseDir doubles as the mount-listing root (see initConfig in
// cmd/lomod/main.go), these must never be offered as a storage-location
// "disk" candidate alongside real user folders.
func IsReservedBaseDirName(name string) bool {
	switch name {
	case binSubDir, docSubDir, etcSubDir, varSubDir, userSubDir:
		return true
	default:
		return false
	}
}

// GetScanImportLogFilename return full path log filename for import dir
func GetScanImportLogFilename(logDir, importDir string) string {
	// also flatten the drive colon of a Windows path (C:\...), which isn't
	// allowed in a filename there
	name := strings.NewReplacer(string(filepath.Separator), "_", ":", "_").Replace(importDir)
	return filepath.Join(logDir, "import"+name+".log")
}

// GetSambaUserDir returns samba user directory
// TODO: support windows.
func GetSambaUserDir(basedir string) string {
	return filepath.Join(basedir, userSubDir)
}

// GetSambaUserHomeDir returns samba user's home directory.
func GetSambaUserHomeDir(basedir, username string) string {
	return filepath.Join(GetSambaUserDir(basedir), username)
}

// GetUserConfDir returns hidden lomorage conf directory for given user
func GetUserConfDir(basedir, username string) string {
	return filepath.Join(filepath.Join(basedir, username), HiddenLomoDir)
}

// GetUserBackupDBDir returns db backup directory for given user
func GetUserBackupDBDir(homeDir string) string {
	return filepath.Join(filepath.Join(homeDir, HiddenLomoDir), DBBackupDir)
}

// GetUserPhotoDir returns user's both master and preview photo directory.
func GetUserPhotoDir(homedir string) (string, string) {
	return filepath.Join(filepath.Join(homedir, AppPhoto), AppPhotoMasterDir),
		filepath.Join(filepath.Join(homedir, AppPhoto), AppPhotoPreviewDir)
}

// GetUserPhotoSharedDir returns user's shared photo directory.
func GetUserPhotoSharedDir(homedir string) string {
	return filepath.Join(filepath.Join(homedir, AppPhoto), AppPhotoSharedDir)
}

// GetUserPhotoTrashRootDir returns user's photo trash directory.
func GetUserPhotoTrashRootDir(homedir string) string {
	return filepath.Join(filepath.Join(homedir, AppPhoto), AppPhotoTrashDir)
}

// GetUserTmpDir return user's tmpe directory. This is for move usage
func GetUserTmpDir(homedir string) string {
	return filepath.Join(homedir, lomodTmpDir)
}

// NormalDatedDirName returns normalized directory name with year/month/day
func NormalDatedDirName(dir string, year, month, day int) string {
	return filepath.Join(dir, filepath.Join(filepath.Join(strconv.Itoa(year),
		fmt.Sprintf("%02d", month)), fmt.Sprintf("%02d", day)))
}

// GetUserPhotoMasterPreviewDir returns user's master and preview photo directory.
func GetUserPhotoMasterPreviewDir(homedir string, year, month, day int, folderPerm os.FileMode) (string, string, error) {
	return GetUserPhotoMasterPreviewDirCreate(homedir, year, month, day, folderPerm)
}

// GetUserPhotoMasterPreviewDirCreate returns user's master and preview photo directory.
func GetUserPhotoMasterPreviewDirCreate(homedir string, year, month, day int, folderPerm os.FileMode) (string, string, error) {
	master, preview := GetUserPhotoDir(homedir)
	ymd := filepath.Join(filepath.Join(strconv.Itoa(year), fmt.Sprintf("%02d", month)), fmt.Sprintf("%02d", day))
	masterdir := filepath.Join(master, ymd)
	previewdir := filepath.Join(preview, ymd)

	if folderPerm == 0 {
		return masterdir, previewdir, nil
	}
	if err := os.MkdirAll(masterdir, folderPerm); err != nil {
		return "", "", err
	}
	return masterdir, previewdir, os.MkdirAll(previewdir, folderPerm)
}

// GetUserPhotoTrashDir return user's trash photo directory.
func GetUserPhotoTrashDir(homedir string, year, month, day int, folderPerm os.FileMode) (string, error) {
	trashDir, err := MkHideDir(filepath.Join(filepath.Join(filepath.Join(homedir, AppPhoto), AppPhotoTrashDir)), folderPerm)
	if err != nil {
		return "", err
	}
	ymd := filepath.Join(filepath.Join(strconv.Itoa(year), fmt.Sprintf("%02d", month)), fmt.Sprintf("%02d", day))
	trashDir = filepath.Join(trashDir, ymd)
	return trashDir, os.MkdirAll(trashDir, folderPerm)
}

// GetUserBackupDir returns user's backup directory.
func GetUserBackupDir(backupdir, username string) string {
	return filepath.Join(backupdir, username)
}

// GetWebdevDir returns webdev dir.
func GetWebdevDir(basedir string) string {
	return filepath.Join(filepath.Join(basedir, varSubDir), webdevSubDir)
}

// ParseAndFormatTime parse time whose format is '2003-11-01 16:00:00+00:00' in DB, and convert to '2003-11-01T16:00:00Z'.
func ParseAndFormatTime(dt string) (string, error) {
	for _, f := range TimeFormatDBs {
		d, err := time.Parse(f, dt)
		if err == nil {
			return d.Format(TimeFormatLomod), nil
		}
	}
	return "", errors.Errorf("unable to parse time %s", dt)
}

// GetFileSHA calculates sha1 of given file.
func GetFileSHA(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()

	sha := sha1.New()
	if _, err := io.Copy(sha, f); err != nil {
		return "", err
	}
	defer func() {
		// set sha to empty for GC
		sha = nil
	}()

	return fmt.Sprintf("%x", sha.Sum(nil)), nil
}

// RetryIfKnownError keep retrying reqFunc if error is known.
func RetryIfKnownError(log string, reqFunc func() error) error {
	// retry 10 times if out of memory, and rely on JIT preview creation
	for i := 0; i < 10; i++ {
		err := reqFunc()
		if err != nil {
			logrus.Warnf("%s: %v", log, err)
			if oomRegex.MatchString(err.Error()) {
				time.Sleep(time.Second)
				continue
			}
			return err
		}
		break
	}
	return nil
}

// IsFileExist check if file is exist or tnot.
func IsFileExist(name string) (bool, error) {
	if _, err := os.Stat(name); err != nil {
		if !os.IsNotExist(err) {
			return false, err
		}
		return false, nil
	}
	return true, nil
}

// CreateOrOpenFile create of open one file.
func CreateOrOpenFile(filename string, filePerm os.FileMode) (*os.File, error) {
	exist, err := IsFileExist(filename)
	if err != nil {
		return nil, err
	}
	flag := os.O_WRONLY
	if !exist {
		flag = flag | os.O_CREATE
	} else {
		flag = flag | os.O_APPEND
	}
	return os.OpenFile(filename, flag, filePerm)
}

// GetRequestAddress returns http request address
func GetRequestAddress(r *http.Request) string {
	addrs := strings.Split(r.RemoteAddr, ":")
	addr := addrs[0]
	if addr == "" {
		addr = "127.0.0.1"
	}
	return addr
}

// SetHeaderIfMatch set if match header.
func SetHeaderIfMatch(header http.Header, size int64, hash string) {
	header.Set(ifMatch, fmt.Sprintf("size=%d, sha1=%s", size, hash))
}

// GetHeaderIfMatch parts http header and get size and hash.
func GetHeaderIfMatch(header http.Header) (int, string, error) {
	im := header.Get(ifMatch)
	if im == "" {
		return -1, "", errors.New("not found If-Match header")
	}
	parts := strings.Split(im, ",")
	size := 0
	hash := ""
	for _, p := range parts {
		parts2 := strings.Split(strings.TrimSpace(p), "=")
		if len(parts2) != 2 {
			return -1, "", errors.Errorf("wrong if-Match header: %s", im)
		}
		switch parts2[0] {
		case "size":
			var err error
			size, err = strconv.Atoi(parts2[1])
			if err != nil {
				return -1, "", err
			}
		case "sha1":
			hash = parts2[1]
		default:
			return -1, "", errors.Errorf("un-recognized key in if-Match header: %s", im)
		}
	}
	return size, hash, nil
}

// CopyFile copies file content
func CopyFile(src, dst string) error {
	sf, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sf.Close()

	df, err := os.Create(dst)
	if err != nil {
		return err
	}

	if _, err = io.Copy(df, sf); err != nil {
		df.Close()
		return err
	}
	// Close flushes the last writes; a failure there (e.g. disk full) means
	// dst is incomplete, so it must not be swallowed
	return df.Close()
}

// CopyDocs copy lomod document to each user's home directory
func CopyDocs(srcDocDir string, homeDir string, dirPerm os.FileMode) error {
	srcFiles := map[string]int64{}
	srcFileInfos, err := ioutil.ReadDir(srcDocDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrapf(err, "read %s", srcDocDir)
	}
	for _, fi := range srcFileInfos {
		srcFiles[fi.Name()] = fi.Size()
	}
	dstDocDir := filepath.Join(homeDir, AppDoc)
	if err := os.MkdirAll(dstDocDir, dirPerm); err != nil {
		return err
	}
	dstFileInfos, err := ioutil.ReadDir(dstDocDir)
	if err != nil {
		logrus.Errorf("read destination doc folder %s got: %s", dstDocDir, err)
		return err
	}
	dstFiles := map[string]int64{}
	for _, fi := range dstFileInfos {
		dstFiles[fi.Name()] = fi.Size()
	}
	// skip same file
	for srcFilename, size := range srcFiles {
		dstSize, ok := dstFiles[srcFilename]
		if ok && dstSize == size {
			continue
		}
		err := CopyFile(filepath.Join(srcDocDir, srcFilename), filepath.Join(dstDocDir, srcFilename))
		if err != nil {
			logrus.Error(err)
		}
	}
	return nil
}

// BackupAndCreate will backup existing file with create time, then create new one
func BackupAndCreate(filename string, force bool) (*os.File, error) {
	info, err := os.Stat(filename)
	if err != nil {
		if !os.IsNotExist(err) {
			if !force {
				return nil, err
			}
			logrus.Warnf("get %s information: %s", filename, err)
		}
	} else {
		newName := fmt.Sprintf("%s.%d%02d%02d", filename, info.ModTime().Year(), info.ModTime().Month(), info.ModTime().Day())
		err = MoveFile(filename, newName)
		if err != nil {
			if !force {
				return nil, err
			}
			logrus.Warnf("failed to backup %s: %s", filename, err)
		}
	}
	return os.Create(filename)
}
