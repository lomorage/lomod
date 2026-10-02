package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/album"
	"bitbucket.org/lomoware/lomo-backend/common/asset"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/logger"
	"bitbucket.org/lomoware/lomo-backend/common/scan"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	scanResultFile     = "scan_result.json"
	scanPreview        = "scan_preview"
	importResultSuffix = "_result.json"
)

// importMode says what happens to the original file when a scanned asset is imported.
type importMode int

const (
	// importLink leaves the file where it is and only records it in the db
	importLink importMode = iota
	// importMove moves the file into the user's library
	importMove
	// importCopy copies the file into the user's library, leaving the original untouched
	importCopy
)

func importModeFromQuery(q url.Values) importMode {
	if q.Get(common.QueryKeyCopy) == "1" {
		return importCopy
	}
	if q.Get(common.QueryKeyMove) == "1" {
		return importMove
	}
	return importLink
}

// errNotImportable is an import request for a file in a folder the user may
// not import from (see scanIgnoreFolders)
var errNotImportable = errors.New("file is in another user's library or a Lomorage internal folder")

type importFailure struct {
	Path   string
	Reason string
}

type importResult struct {
	Path  string
	Err   error
	Begin time.Time
	End   time.Time

	// per-file progress of a tree import (scanImport), guarded by mu since
	// the import goroutine writes it while /assets/scan/status reads it
	mu       sync.Mutex
	userID   int
	total    int
	imported int
	skipped  int
	failures []importFailure
}

func (ir *importResult) running() bool {
	ir.mu.Lock()
	defer ir.mu.Unlock()
	return !ir.Begin.IsZero() && ir.End.IsZero()
}

// tryStart starts a new import unless one is already running.
func (ir *importResult) tryStart(userID, total int) bool {
	ir.mu.Lock()
	defer ir.mu.Unlock()
	if !ir.Begin.IsZero() && ir.End.IsZero() {
		return false
	}
	ir.resetLocked(userID, total)
	return true
}

func (ir *importResult) start(userID, total int) {
	ir.mu.Lock()
	defer ir.mu.Unlock()
	ir.resetLocked(userID, total)
}

func (ir *importResult) resetLocked(userID, total int) {
	ir.Err = nil
	ir.userID = userID
	ir.Begin = time.Now()
	ir.End = time.Time{}
	ir.total = total
	ir.imported = 0
	ir.skipped = 0
	ir.failures = nil
}

func (ir *importResult) finish() {
	ir.mu.Lock()
	defer ir.mu.Unlock()
	ir.End = time.Now()
}

func (ir *importResult) addImported() {
	ir.mu.Lock()
	defer ir.mu.Unlock()
	ir.imported++
}

func (ir *importResult) addSkipped() {
	ir.mu.Lock()
	defer ir.mu.Unlock()
	ir.skipped++
}

func (ir *importResult) addFailure(path string, err error) {
	ir.mu.Lock()
	defer ir.mu.Unlock()
	ir.failures = append(ir.failures, importFailure{Path: path, Reason: err.Error()})
}

type scanProgress struct {
	Running bool
	// Counting is true during the scanner's 1st pass, which only counts media
	// files so the 2nd pass (hashing + dedup) has a total to report against
	Counting bool
	// Total media files found; Checked of them processed so far, of which
	// New are not in the library yet and Existing already are
	Total    int
	Checked  int
	New      int
	Existing int
	Elapsed  int
}

type importProgress struct {
	Running  bool
	Total    int
	Done     int
	Imported int
	Skipped  int
	Failures []importFailure
}

type scanStatus struct {
	Scan   scanProgress
	Import importProgress
}

func (h *Handler) getScanResultFilepath() (string, string) {
	varDir := common.GetVarDir(h.conf.BaseDir)
	return varDir, filepath.Join(varDir, scanResultFile)
}

func (h *Handler) getImportFilepath(name string) string {
	varDir := common.GetVarDir(h.conf.BaseDir)
	return filepath.Join(varDir, strings.ReplaceAll(name, string(filepath.Separator), "_")+".json")
}

func (h *Handler) getImportResultFilepath(name string) (string, string) {
	varDir := common.GetVarDir(h.conf.BaseDir)
	return varDir, filepath.Join(varDir, strings.ReplaceAll(name, string(filepath.Separator), "_")+importResultSuffix)
}

func (h *Handler) getScanResultFile() (*os.File, error) {
	varDir, resultFilepath := h.getScanResultFilepath()
	if err := os.MkdirAll(varDir, common.DefaultFolderPermission); err != nil {
		logrus.Warnf("create var director %s: %v", varDir, err)
		return nil, err
	}
	return os.Create(resultFilepath)
}

func (h *Handler) getScanPreviewRoot() (previewRoot string, err error) {
	// preview root is from base directory
	previewRoot = filepath.Join(common.GetVarDir(h.conf.BaseDir), scanPreview)
	err = os.MkdirAll(previewRoot, common.DefaultFolderPermission)
	return
}

func (h *Handler) persistScanResult(resultFilename string, scanRootFile *scan.File) error {
	f, err := os.Create(resultFilename)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", " ")
	return enc.Encode(scanRootFile)
}

func (h *Handler) scan(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	if !h.scanRunner.Begin.IsZero() && h.scanRunner.End.IsZero() {
		common.WriteError(w, common.ErrScanInProgress)
		return
	}

	q := r.URL.Query()
	rootFolder := h.conf.MountDir
	if q.Get(common.QueryKeyPath) != "" {
		var err error
		rootFolder, err = url.QueryUnescape(q.Get(common.QueryKeyPath))
		if err != nil {
			common.WriteError(w, err)
			return
		}
	}
	rootFolder = filepath.Clean(rootFolder)
	if q.Get(common.QueryKeyScanAndImport) == "1" {
		err := h.scanDirImport(wl.Userid, wl.Deviceid, rootFolder,
			importModeFromQuery(q), q.Get(common.QueryKeyScanVideo) == "1", q.Get(common.QueryKeyUseExifTime) == "1")
		if err != nil {
			common.WriteError(w, err)
		}
		return
	}
	if info, err := os.Stat(rootFolder); err != nil || !info.IsDir() {
		common.WriteError(w, common.ErrDeviceNotMount)
		return
	}
	// scan-video defaults on here (unlike scan-and-import above) since a
	// scan-only request just lists what's found for the caller to pick from
	err := h.scanDirOnly(wl.Userid, rootFolder,
		q.Get(common.QueryKeyScanVideo) != "0", q.Get(common.QueryKeyUseExifTime) == "1")
	if err != nil {
		common.WriteError(w, err)
	}
}

// getScanStatus reports progress of the running (or last) scan and scan
// import, so a UI can show more than a spinner while either runs.
func (h *Handler) getScanStatus(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	st := scanStatus{}
	stats := h.scanRunner.Stats
	st.Scan.Running = !h.scanRunner.Begin.IsZero() && h.scanRunner.End.IsZero()
	st.Scan.Counting = stats.Is1stPass
	st.Scan.Total = stats.TotalMediaFiles()
	if !stats.Is1stPass {
		st.Scan.New = stats.InProgressImageFiles() + stats.InProgressVideoFiles()
		st.Scan.Existing = stats.TotalDuplicateFiles()
		st.Scan.Checked = st.Scan.New + st.Scan.Existing + stats.TotalIgnoreVideoFiles()
		if st.Scan.Checked > st.Scan.Total {
			st.Scan.Checked = st.Scan.Total
		}
	}
	if !h.scanRunner.Begin.IsZero() {
		end := h.scanRunner.End
		if end.IsZero() {
			end = time.Now()
		}
		st.Scan.Elapsed = int(end.Sub(h.scanRunner.Begin).Seconds())
	}

	ir := &h.scanImportResult
	ir.mu.Lock()
	st.Import.Running = !ir.Begin.IsZero() && ir.End.IsZero()
	// anyone may see that an import is running (only one can run at a
	// time), but its details -- file paths included -- are the importer's
	if ir.userID == wl.Userid {
		st.Import.Total = ir.total
		st.Import.Imported = ir.imported
		st.Import.Skipped = ir.skipped
		st.Import.Failures = append([]importFailure{}, ir.failures...)
		st.Import.Done = ir.imported + ir.skipped + len(ir.failures)
	}
	ir.mu.Unlock()

	common.WriteBody(w, st)
}

type browseDirResult struct {
	Path   string
	Parent string
	Dirs   []string
	// Roots are top-level places to jump to that aren't reachable by going
	// up from Path: other drives on Windows (e.g. a USB stick on E:\), and
	// the folders removable media mounts under elsewhere
	Roots []string
}

// browseRoots lists the filesystem roots a folder picker should offer.
func browseRoots() []string {
	if runtime.GOOS == "windows" {
		// not stat'ed like the others below: probing a drive letter can
		// block for seconds (a disconnected network drive, an empty card
		// reader), and this runs on every folder the picker opens
		return windowsDriveRoots()
	}
	candidates := []string{"/", "/media", "/mnt"}
	if runtime.GOOS == "darwin" {
		candidates = []string{"/", "/Volumes"}
	}
	roots := []string{}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			roots = append(roots, c)
		}
	}
	return roots
}

// browseDir lists the immediate subdirectories of a path (default:
// MountDir), for a folder-picker UI to navigate before kicking off a full
// recursive /assets/scan -- deliberately a cheap, single-level ioutil.ReadDir
// rather than reusing scan's own tree walk, which hashes every file and
// generates previews and would be far too expensive just to browse.
func (h *Handler) browseDir(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	path := h.conf.MountDir
	if q := r.URL.Query().Get(common.QueryKeyPath); q != "" {
		var err error
		path, err = url.QueryUnescape(q)
		if err != nil {
			common.WriteError(w, err)
			return
		}
	}
	path = filepath.Clean(path)

	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		common.WriteError(w, common.ErrDeviceNotMount)
		return
	}

	entries, err := ioutil.ReadDir(path)
	if err != nil {
		common.WriteError(w, err)
		return
	}

	atMountRoot := path == filepath.Clean(h.conf.MountDir)
	dirs := []string{}
	for _, e := range entries {
		if !e.IsDir() || common.IsHiddenFile(filepath.Join(path, e.Name())) {
			continue
		}
		// IsHiddenFile alone doesn't catch dot-prefixed folders on Windows
		// (it checks the OS hidden attribute, which lomod's own .lomo/
		// .lomodTemp management folders don't set) -- mirror /mount's own
		// filter so internal bookkeeping dirs don't clutter the picker.
		begin := rune(e.Name()[0])
		if !unicode.IsLetter(begin) && !unicode.IsDigit(begin) {
			continue
		}
		if atMountRoot && common.IsReservedBaseDirName(e.Name()) {
			continue
		}
		dirs = append(dirs, e.Name())
	}
	sort.Strings(dirs)

	parent := filepath.Dir(path)
	if parent == path {
		// already at a filesystem root (e.g. "C:\" or "/") -- nowhere further up
		parent = ""
	}

	common.WriteBody(w, browseDirResult{Path: path, Parent: parent, Dirs: dirs, Roots: browseRoots()})
}

func (h *Handler) scanDirOnly(userID int, rootFolder string, scanVideo, useExifTime bool) error {
	previewRootFolder, err := h.getScanPreviewRoot()
	if err != nil {
		logrus.Errorf("failed to get scan preview root dir: %v", err)
		return err
	}

	reportFileChan := make(chan *scan.File)
	go func(reportFileChan chan *scan.File) {
		for {
			// log stats every minute
			after := time.After(time.Minute)
			select {
			case <-after:
				logrus.Infof("IN PROGRESS: scanned %d directories", h.scanRunner.Stats.TotalDirs())
			case f, ok := <-reportFileChan:
				if !ok {
					logrus.Infof("FINISH: scanned %d directories", h.scanRunner.Stats.TotalDirs())
					return
				}
				if f == nil {
					logrus.Warnf("received empty scan notification")
					continue
				}
				previewPath := common.NormalDatedDirName(previewRootFolder, f.CreateTime.Year(),
					int(f.CreateTime.Month()), f.CreateTime.Day())
				if !h.conf.UseJpg {
					h.previewRunner.Generate(f.Path(), previewPath, *f.SHA1, filepath.Ext(f.Name), false, true)
				} else {
					h.previewRunner.Generate(f.Path(), previewPath, *f.SHA1, filepath.Ext(f.Name), false, false)
				}
			}
		}
	}(reportFileChan)

	dir, scanResultFilename := h.getScanResultFilepath()
	if err := os.MkdirAll(dir, h.conf.FolderPerm); err != nil {
		return err
	}

	return h.scanDir(userID, rootFolder, scanResultFilename, reportFileChan, scanVideo, useExifTime, nil)
}

func (h *Handler) scanDir(userID int, rootFolder, resultFilename string, reportFileChan chan *scan.File,
	scanVideo, useExifTime bool, logFile io.Writer) error {
	ignoreFolders, err := h.scanIgnoreFolders(userID)
	if err != nil {
		return err
	}

	// mark the scan as started before returning to the caller: Start only
	// resets Begin/End once its goroutine runs, and until then a GET
	// /assets/scan would serve the previous scan's result as if it were this one
	h.scanRunner.ResetStats()
	h.scanRunner.Begin = time.Now()

	go func() {
		defer func() {
			close(reportFileChan)
		}()
		files, err := h.scanRunner.Start(scan.Config{
			RootFolderName: rootFolder,
			IgnoreFolders:  ignoreFolders,
			IgnoreVideo:    !scanVideo,
			NoExifAnalysis: !useExifTime,
			Logger:         logFile,
			Exiftool:       h.exiftool,
			HashExist: func(hash string) bool {
				if userID == 0 {
					// skip check
					return false
				}
				return h.userHasAsset(userID, hash)
			},
		}, reportFileChan)
		if err != nil {
			logrus.Errorf("failed to scan %s staring from %s: %v", rootFolder,
				h.scanRunner.Begin.Format(common.TimeFormatLomod), err)
			// surface it on GET /assets/scan instead of the previous scan's result
			h.scanRunner.Err = err
			return
		}
		// persist json file
		if err := h.persistScanResult(resultFilename, files); err != nil {
			logrus.Warnf("persist scan result: %v", err)
		}
	}()

	return nil
}

// scanIgnoreFolders is the set of folders the user may neither scan nor
// import from: other users' libraries, and lomod's own generated folders
// inside the user's.
func (h *Handler) scanIgnoreFolders(userID int) (map[string]struct{}, error) {
	ignoreFolders := map[string]struct{}{}
	err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		//return home dir if request is from localhost
		users, err := user.ListUsers(ctx, tx)
		if err != nil {
			return err
		}
		for _, u := range users.Users {
			if u.IsBotUser() {
				continue
			}
			// Don't ignore the initiating user's own home/backup dir: if they're
			// pointing a scan at (or into) their own folder -- e.g. to recover
			// files that survived a reset but never made it into a fresh
			// catalog -- that's the one case where "already a known user's
			// folder" should NOT mean "skip it". Every *other* user's folder is
			// still excluded, so scanning a shared parent (e.g. a USB mount)
			// won't sweep up someone else's already-imported library.
			if u.ID == userID {
				// Still skip lomod's own generated/internal subfolders within
				// it, though: preview (thumbnail cache), lomodTrash, and share
				// aren't source photos to (re-)import -- master, the rest of
				// the home dir, is what's actually worth scanning.
				_, previewDir := common.GetUserPhotoDir(u.HomeDir)
				ignoreFolders[previewDir] = struct{}{}
				ignoreFolders[common.GetUserPhotoTrashRootDir(u.HomeDir)] = struct{}{}
				ignoreFolders[common.GetUserPhotoSharedDir(u.HomeDir)] = struct{}{}
				continue
			}
			ignoreFolders[u.HomeDir] = struct{}{}
			if u.BackupDir != "" {
				ignoreFolders[u.BackupDir] = struct{}{}
			}
		}
		return nil
	})
	return ignoreFolders, err
}

// isUnderFolder reports whether path is folder itself or inside it.
func isUnderFolder(path, folder string) bool {
	if folder == "" {
		return false
	}
	p, f := filepath.Clean(path), filepath.Clean(folder)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		// case-insensitive filesystems by default
		p, f = strings.ToLower(p), strings.ToLower(f)
	}
	rel, err := filepath.Rel(f, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) &&
		!filepath.IsAbs(rel)
}

func inIgnoredFolder(path string, ignoreFolders map[string]struct{}) bool {
	for f := range ignoreFolders {
		if isUnderFolder(path, f) {
			return true
		}
	}
	return false
}

func (h *Handler) scanDirImport(userID, deviceID int, dir string, mode importMode, scanVideo, useExifTime bool) error {
	_, err := os.Stat(dir)
	if err != nil {
		return err
	}

	userHome := ""
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		userHome, err = user.GetHomedir(ctx, tx, userID)
		return err
	}); err != nil {
		return err
	}

	// make sure the folder is created
	err = os.MkdirAll(common.GetUserTmpDir(userHome), h.conf.FolderPerm)
	if err != nil {
		return err
	}

	logFile, err := common.BackupAndCreate(common.GetScanImportLogFilename(h.conf.LogDir, dir), true)
	if err != nil {
		return err
	}

	h.scanImportResult.start(userID, 0)

	reportFileChan := make(chan *scan.File)
	go func(logFile *os.File, reportFileChan chan *scan.File) {
		defer logFile.Close()
		totalImportedImageFiles := 0
		totalImportedVideoFiles := 0
		// log stats every minute
		after := time.After(time.Minute)
		for {
			select {
			case <-after:
				totalMediaDirs := h.scanRunner.Stats.InProgressMediaDirs()
				if (totalImportedImageFiles + totalImportedVideoFiles) != 0 {
					totalMediaDirs++
				}
				info := fmt.Sprintf("IN PROGRESS [elapsed: %s | dir: %d(%d), files: %d(%d)",
					time.Since(h.scanRunner.Begin).Truncate(time.Second),
					totalMediaDirs, h.scanRunner.Stats.TotalDirs(),
					totalImportedImageFiles+totalImportedVideoFiles, h.scanRunner.Stats.TotalFiles())
				logrus.Info(info)
				logFile.WriteString(info + "\n")
				after = time.After(time.Minute)
			case f, ok := <-reportFileChan:
				if !ok {
					h.scanImportResult.finish()
					logrus.Infof("finish scan from %s to %s", h.scanRunner.Begin.Format(common.TimeFormatLomod),
						h.scanRunner.End.Format(common.TimeFormatLomod))

					info := fmt.Sprintf("FINISH: scanned %d directories, imported %d/%d image/video files, ignore %d video and %d duplicated files",
						h.scanRunner.Stats.TotalDirs(),
						totalImportedImageFiles,
						totalImportedVideoFiles,
						h.scanRunner.Stats.TotalIgnoreVideoFiles(),
						h.scanRunner.Stats.TotalDuplicateFiles(),
					)
					logrus.Info(info)
					logFile.WriteString(info + "\n")
					return
				}
				if f == nil {
					logrus.Warnf("received empty scan notification")
					continue
				}
				logFile.WriteString("start import " + f.Path() + "\n")
				newPath, err := h.importScannedAssets(userID, deviceID, f, "", userHome, mode, scanVideo)
				if err != nil {
					str := fmt.Sprintf("failed to import scanned asset (%+v) : %v", f, err)
					logrus.Warn(str)
					logFile.WriteString(str + "\n")
				} else if newPath != "" {
					if ext.IsImageFile(f.Path()) {
						totalImportedImageFiles++
					} else {
						totalImportedVideoFiles++
					}
					logFile.WriteString("finish import " + f.Path() + " to " + newPath + "\n")
				} else if !scanVideo {
					logFile.WriteString("ignore video\n")
				} else {
					logFile.WriteString("skip because it is existed\n")
				}
			}
		}
	}(logFile, reportFileChan)

	varDir, scanResultFilename := h.getImportResultFilepath(dir)
	if err := os.MkdirAll(varDir, h.conf.FolderPerm); err != nil {
		return err
	}
	return h.scanDir(userID, dir, scanResultFilename, reportFileChan, scanVideo, useExifTime, logFile)
}

func (h *Handler) checkScanResult() (string, error) {
	if h.scanRunner.End.IsZero() {
		return "", common.ErrScanInProgress
	}
	if h.scanRunner.Err != nil {
		return "", h.scanRunner.Err
	}
	_, filename := h.getScanResultFilepath()
	return filename, nil
}

func (h *Handler) getScanResult(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	filename, err := h.checkScanResult()
	if err != nil {
		common.WriteError(w, err)
		return
	}

	http.ServeFile(w, r, filename)
}

func (h *Handler) getScanLog(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	rootFolder, err := url.QueryUnescape(r.URL.Query().Get(common.QueryKeyPath))
	if err != nil {
		common.WriteError(w, err)
		return
	}
	rootFolder = filepath.Clean(rootFolder)
	http.ServeFile(w, r, common.GetScanImportLogFilename(h.conf.LogDir, rootFolder))
}

func (h *Handler) getScanImportResult(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	if h.scanImportResult.End.IsZero() {
		common.WriteError(w, common.ErrScanInProgress)
		return
	}
	if h.scanImportResult.Err != nil {
		common.WriteError(w, h.scanImportResult.Err)
		return
	}
	importRequestFile := h.getImportFilepath(mux.Vars(r)["name"])
	http.ServeFile(w, r, importRequestFile)
}

func (h *Handler) persistImportResult(name string, importRootFile *scan.File) error {
	_, fn := h.getImportResultFilepath(name)
	f, err := os.Create(fn)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", " ")
	return enc.Encode(importRootFile)
}

func (h *Handler) scanImport(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	name, err := url.PathUnescape(mux.Vars(r)["name"])
	if err != nil {
		common.WriteError(w, err)
		return
	}

	_, err = h.checkScanResult()
	if err != nil {
		common.WriteError(w, err)
		return
	}
	if h.scanImportResult.running() {
		common.WriteError(w, common.ErrScanInProgress)
		return
	}

	importRequestFile := h.getImportFilepath(name)
	buf := &bytes.Buffer{}
	defer r.Body.Close()
	if _, err := io.Copy(buf, r.Body); err != nil {
		common.WriteError(w, err)
		return
	}
	if err := ioutil.WriteFile(importRequestFile, buf.Bytes(), h.conf.FilePerm); err != nil {
		common.WriteError(w, err)
		return
	}

	importRequest := &scan.File{}
	if err := json.NewDecoder(buf).Decode(importRequest); err != nil {
		common.WriteError(w, err)
		return
	}
	scan.RecreateParent(importRequest)

	userHome := ""
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		userHome, err = user.GetHomedir(ctx, tx, wl.Userid)
		return err
	}); err != nil {
		common.WriteError(w, err)
		return
	}

	err = os.MkdirAll(common.GetUserTmpDir(userHome), h.conf.FolderPerm)
	if err != nil {
		common.WriteError(w, err)
		return
	}

	srcPreviewRoot, err := h.getScanPreviewRoot()
	if err != nil {
		logrus.Errorf("get scan preview root director: %v", err)
	}
	q := r.URL.Query()
	mode := importModeFromQuery(q)
	scanVideo := q.Get(common.QueryKeyScanVideo) == "1"

	// the posted tree comes from the client, so hold its paths to the same
	// rule the scan itself follows: nothing from another user's library or
	// lomod's own generated folders
	ignoreFolders, err := h.scanIgnoreFolders(wl.Userid)
	if err != nil {
		common.WriteError(w, err)
		return
	}

	// the running() check above is only a fast path; this is the one that
	// counts if two imports race past it
	if !h.scanImportResult.tryStart(wl.Userid, countScannedFiles(importRequest)) {
		common.WriteError(w, common.ErrScanInProgress)
		return
	}

	// same log file scan-and-import writes, keyed by the scanned root folder,
	// so /assets/scan/log shows this import rather than a stale earlier one
	var logFile io.Writer = ioutil.Discard
	lf, err := common.BackupAndCreate(common.GetScanImportLogFilename(h.conf.LogDir, importRequest.Path()), true)
	if err != nil {
		logrus.Warnf("create scan import log: %v", err)
	} else {
		logFile = lf
	}

	go func(name string) {
		defer h.scanImportResult.finish()
		if lf != nil {
			defer lf.Close()
		}
		h.importScannedTree(wl.Userid, wl.Deviceid, importRequest, srcPreviewRoot, userHome, mode, scanVideo,
			ignoreFolders, logFile)
		ir := &h.scanImportResult
		ir.mu.Lock()
		fmt.Fprintf(logFile, "FINISH: imported %d, skipped %d already in library, failed %d\n",
			ir.imported, ir.skipped, len(ir.failures))
		ir.mu.Unlock()
		// persist new import result
		if err := h.persistImportResult(name, importRequest); err != nil {
			logrus.Warnf("persist scan result: %v", err)
		}
	}(name)
}

// userHasAsset reports whether the user's library already has an asset with this hash.
func (h *Handler) userHasAsset(userID int, hash string) bool {
	if h.conf.UseMemdb {
		h.memdb.Lock()
		exist := h.memdb.ExistByHash(userID, hash)
		h.memdb.Unlock()
		return exist != nil
	}
	err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		_, err := asset.GetAssetByHash(ctx, tx, userID, hash)
		return err
	})
	return err == nil
}

func countScannedFiles(item *scan.File) int {
	if !item.IsDir() {
		return 1
	}
	n := 0
	for _, f := range item.Children {
		n += countScannedFiles(f)
	}
	return n
}

// importScannedTree imports every file under item, recording per-file
// progress in h.scanImportResult and logFile as it goes.
func (h *Handler) importScannedTree(userID, deviceID int, item *scan.File, srcPreviewRoot, userHome string,
	mode importMode, scanVideo bool, ignoreFolders map[string]struct{}, logFile io.Writer) {
	if item.IsDir() {
		for _, f := range item.Children {
			h.importScannedTree(userID, deviceID, f, srcPreviewRoot, userHome, mode, scanVideo, ignoreFolders, logFile)
		}
		return
	}
	var newPath string
	var err error
	if inIgnoredFolder(item.Path(), ignoreFolders) {
		err = errNotImportable
	} else {
		newPath, err = h.importScannedAssets(userID, deviceID, item, srcPreviewRoot, userHome, mode, scanVideo)
	}
	switch {
	case err != nil:
		logrus.Warnf("import %s: %v", item.Path(), err)
		fmt.Fprintf(logFile, "failed to import %s: %v\n", item.Path(), err)
		h.scanImportResult.addFailure(item.Path(), err)
	case newPath != "":
		fmt.Fprintf(logFile, "finish import %s to %s\n", item.Path(), newPath)
		h.scanImportResult.addImported()
	default:
		fmt.Fprintf(logFile, "skip %s\n", item.Path())
		h.scanImportResult.addSkipped()
	}
}

func (h *Handler) importScannedAssets(userID, deviceID int, item *scan.File, srcPreviewRoot, userHome string,
	mode importMode, scanVideo bool) (string, error) {
	// copy ends up as a move of a private copy, see srcPath below
	move := mode != importLink
	if item.IsDir() {
		for _, f := range item.Children {
			if _, err := h.importScannedAssets(userID, deviceID, f, srcPreviewRoot, userHome, mode, scanVideo); err != nil {
				logrus.Warnf("import directory %s: %s", f.Path(), err)
			}
		}
		return "", nil
	}
	if item.SHA1 == nil {
		return "", errors.Errorf("%s has empty SHA", item.Path())
	}

	extStr := strings.ToLower(strings.TrimPrefix(filepath.Ext(item.Name), "."))
	if !scanVideo && ext.IsVideoFile(extStr) {
		return "", nil
	}

	extID, err := ext.GetExtID(extStr)
	if err != nil {
		return "", errors.Wrapf(err, "while import %s", item.Path())
	}
	newPath := item.Path()
	moved := item.IsImported()
	if moved != nil {
		// never convert a linked asset by moving the user's original away
		// when they asked for it to be left untouched
		if *moved == move || mode == importCopy {
			return "", nil
		} else if *moved {
			// file has been moved
			logrus.Infof("require link %s, but already moved", newPath)
			return "", nil
		}
		return newPath, dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
			return asset.MoveAsset(ctx, tx, userID, *item.SHA1, item.CreateTime, h.conf.FolderPerm)
		})
	}

	// checked up front rather than left to CreateAsset's insert: without
	// memdb a duplicate there is a raw sqlite UNIQUE error, not ErrDuplicate,
	// and in copy mode it saves copying a file only to throw it away
	if h.userHasAsset(userID, *item.SHA1) {
		return "", nil
	}
	srcPath := newPath
	if mode == importCopy {
		// copy into the user's own tmp dir first, then let CreateAsset move
		// that copy into the library -- the tmp dir sits on the same disk as
		// the library, so the move is a cheap rename
		srcPath = filepath.Join(common.GetUserTmpDir(userHome), "import-"+*item.SHA1+"."+extStr)
		if err := common.CopyFile(newPath, srcPath); err != nil {
			os.Remove(srcPath)
			return "", errors.Wrapf(err, "copy %s", newPath)
		}
		// no-op once CreateAsset has moved it; cleans up if it failed or was a duplicate
		defer os.Remove(srcPath)
	}

	var assetPath, previewDir string
	a := &types.Asset{Date: types.LomoTime{Time: item.CreateTime}, Hash: *item.SHA1}
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		// don't use symbol link for move because FAT doesn't support
		// newPath = filepath.Join(common.GetUserTmpDir(userHome), *item.SHA1)
		// // remove any old files in case it failed
		// os.Remove(newPath) // skip log or check any failure
		// if err := os.Symlink(item.Path(), newPath); err != nil {
		// 	return "", err
		// }
		assetPath, previewDir, err = asset.CreateAsset(ctx, tx, userID, deviceID, extID, srcPath, "",
			a, h.conf.FolderPerm, move)
		return err
	}); err != nil {
		if !dupRegex.MatchString(err.Error()) {
			return "", err
		}
		return "", nil
	}
	item.SetImported(move)

	if h.conf.UseMemdb {
		h.memdb.Lock()
		h.memdb.Insert(*a, userID, item.CreateTime.Year(), int(item.CreateTime.Month()), item.CreateTime.Day())
		h.memdb.Unlock()
	}

	// add import into album
	err = h.addImportedAssetInAlbum(userID, item, a)
	if err != nil {
		logrus.Warnf("unable add into import album %v: %s", item, err)
	}

	if srcPreviewRoot == "" {
		previewPrefix := ""
		if !move {
			_, dstName := filepath.Split(assetPath)
			previewPrefix = strings.TrimSuffix(dstName, filepath.Ext(dstName))
			assetPath = item.Path()
		}
		if !h.conf.UseJpg {
			h.previewRunner.Generate(assetPath, previewDir, previewPrefix, extStr, false, true)
		} else {
			h.previewRunner.Generate(assetPath, previewDir, previewPrefix, extStr, false, false)
		}
		return assetPath, nil
	}

	if !move {
		assetPath = item.Path()
	}

	return assetPath, h.copyScanPreview(item, assetPath, previewDir, srcPreviewRoot)
}

func (h *Handler) addImportedAssetInAlbum(userID int, item *scan.File, a *types.Asset) error {
	albumTitle := filepath.Dir(item.Path())
	assetID, err := asset.ParseAssetID(a.Name)
	if err != nil {
		return err
	}
	return dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		albumID, err := album.IsUserAlbumExist(ctx, tx, userID, albumTitle)
		if err != nil {
			if !common.IsErrNoRows(err) {
				return err
			}
			id, err := album.CreateAlbum(ctx, tx, userID, album.Album{
				Title:       albumTitle,
				Description: "auto generated during import",
				Author:      common.AlbumAuthorInternal,
			})
			if err != nil {
				return err
			}
			albumID = int(id)
		}
		err = album.IsAssetInAlbum(ctx, tx, albumID, assetID)
		if err == nil {
			return nil
		}
		if !common.IsErrNoRows(err) {
			return err
		}
		return album.AddAssets(ctx, tx, albumID, map[int]string{assetID: item.Name})
	})
}

func (h *Handler) copyScanPreview(item *scan.File, assetPath, dstPreviewDir, srcPreviewRoot string) error {
	// scanned preview file
	srcDir := common.NormalDatedDirName(srcPreviewRoot,
		item.CreateTime.Year(), int(item.CreateTime.Month()), item.CreateTime.Day())
	srcPrefix := *item.SHA1

	// new destination file
	_, dstName := filepath.Split(assetPath)
	extension := filepath.Ext(dstName)
	dstPrefix := strings.TrimSuffix(dstName, extension)
	if err := h.previewRunner.Copy(assetPath, srcDir, srcPrefix, dstPreviewDir, dstPrefix, extension,
		false, h.conf.FolderPerm); err != nil {
		logrus.Warnf("copy jpg preview got: %v", err)
	}
	if err := h.previewRunner.Copy(assetPath, srcDir, srcPrefix, dstPreviewDir, dstPrefix, extension,
		true, h.conf.FolderPerm); err != nil {
		logrus.Warnf("copy webp preview got: %v", err)
	}
	return nil
}

func (h *Handler) getScanPreview(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	width, height := h.parsePreviewDimension(r)

	previewRoot, err := h.getScanPreviewRoot()
	if err != nil {
		common.WriteError(w, err)
		return
	}

	vars := mux.Vars(r)
	y, err := strconv.Atoi(vars["year"])
	if err != nil {
		common.WriteError(w, err)
		return
	}
	m, err := strconv.Atoi(vars["month"])
	if err != nil {
		common.WriteError(w, err)
		return
	}
	d, err := strconv.Atoi(vars["day"])
	if err != nil {
		common.WriteError(w, err)
		return
	}

	// TODO: support JITT and video
	parts := strings.Split(vars["sha1"], ".")
	extension := ext.JPGString
	if len(parts) > 2 {
		common.WriteError(w, errors.Errorf("wrong filename: %s", vars["sha1"]))
		return
	} else if len(parts) == 2 {
		extension = strings.ToLower(parts[1])
	}
	previewFile := ext.MkPreviewAssetName(filepath.Join(common.NormalDatedDirName(previewRoot, y, m, d), parts[0]),
		extension, width, height, ext.IsVideoFile(extension), false)

	http.ServeFile(w, r, previewFile)
}
