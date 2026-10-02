package handler

import (
	"archive/zip"
	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/album"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/logger"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"context"
	"database/sql"
	"fmt"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/webdav"
)

const (
	dirPhotos = "Photos"
	dirAlbums = "Albums"
)

const (
	dirLayoutYyyymmdd = iota
	dirLayoutYyyymm
	dirLayoutYyyy
)

var knownOSFiles map[string]struct{} = map[string]struct{}{
	"desktop.ini": struct{}{},
	"Thumbs.db": struct{}{},
}

type srv struct {
	h *webdav.Handler
	lomodHandler *Handler
}

func mkDateName(n int) string {
	return fmt.Sprintf("%02d", n)
}

func isLivePhoto(name string) bool {
	return strings.HasSuffix(name, ".zip")
}

func webdevLogger() func(*http.Request, error) {
	return func(r *http.Request, err error) {
		if err != nil {
			logrus.Warnf("webdev request %s %s %s %s: %v\n", r.Method, r.URL,
				r.RemoteAddr, r.UserAgent(), err)
			return
		}
		logrus.Tracef("REQUEST %s %s length:%d %s %s\n", r.Method, r.URL, r.ContentLength,
			r.RemoteAddr, r.UserAgent())
	}
}

func (s *srv) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	s.lomodHandler.accessLogger.LogHTTP(r, "", "", r.URL.Path, "", start, true, false)

	l := logger.NewResponseLogger(w)
	s.h.ServeHTTP(l, r)

	s.lomodHandler.accessLogger.LogHTTP(r, "", "", r.URL.Path,
		fmt.Sprintf(" %d %d %s", l.Status(), l.Size(), time.Since(start).Truncate(time.Second)),
		time.Now(), false, false)
}

type myFS struct {
	h *Handler
	rootFile *lomoFile
	rootFileInfo *fileInfo
	users map[string]*user.User
	albums map[string]map[string]int  // album name, user name, album id
}

func newMyFS(h *Handler) *myFS {
	return &myFS{h: h, rootFile: &lomoFile{isDir: true, name: "lomorage"},
		rootFileInfo: &fileInfo{name: "/"}, users: map[string]*user.User{},
		albums: map[string]map[string]int{},
	}
}

func (m *myFS) isAssetExist(username, name string) bool {
	u, ok := m.users[username]
	if !ok {
		logrus.Warnf("while checking asset exist, user %s is not found", username)
		return false
	}
	y, mon, d, _, err := ext.ParseNormalizedAssetName(name)
	if err != nil {
		logrus.Warnf("while checking asset exist, parsing asset name %s: %s", name, err)
		return false
	}
	assetDir, _, err := common.GetUserPhotoMasterPreviewDir(u.HomeDir, y, mon, d, m.h.conf.FolderPerm)
	if err != nil {
		logrus.Warnf("while checking asset exist, get asset dir %s: %s", name, err)
		return false
	}
	_, err = os.Stat(filepath.Join(assetDir, name))
	if err != nil {
		logrus.Warnf("checking asset stat %s: %s", name, err)
		return false
	}
	return true
}

func (m *myFS) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	//fmt.Printf("mkdir: %s\n", name)
	return nil
}

func (m *myFS) buildUserDir(name string) os.FileInfo {
	userDir := &fileInfo{name: name}

	return userDir
}

func (m *myFS) openRoot() (webdav.File, error) {
	m.rootFile.files = []os.FileInfo{}
	err := dbx.InQuery(m.h.db, func(ctx context.Context, tx *sql.Tx) error {
		//return home dir if request is from localhost
		users, err := user.ListUsers(ctx, tx)
		if err != nil {
			return err
		}
		for _, u := range users.Users {
			if u.IsBotUser() {
				continue
			}
			m.users[u.Name] = u
			m.rootFile.files = append(m.rootFile.files, m.buildUserDir(u.Name))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return m.rootFile, nil
}

func (m *myFS) openUserAlbums(username string) (webdav.File, error) {
	u, ok := m.users[username]
	if !ok {
		return nil, errors.Errorf("user %s not found", username)
	}

	albumInfos := []os.FileInfo{}
	dirNames := map[string]struct{}{}
	err := dbx.InQuery(m.h.db, func(ctx context.Context, tx *sql.Tx) error {
		//return home dir if request is from localhost
		as, err := album.ListAlbums(ctx, tx, u.ID)
		if err != nil {
			return err
		}
		for _, a := range as.Albums {
			albumName := strings.TrimSuffix(strings.TrimPrefix(a.Title, "/"), "/")
			ua, ok := m.albums[albumName]
			if !ok {
				ua = map[string]int{}
			}
			ua[username] = a.ID
			m.albums[albumName] = ua

			dirName := strings.Split(albumName, "/")[0]
			_, ok = dirNames[dirName]
			if ok {
				continue
			}
			dirNames[dirName] = struct{}{}
			albumInfos = append(albumInfos, &fileInfo{name: dirName})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &lomoFile{isDir: true, name: dirAlbums, files: albumInfos}, nil
}

func (m *myFS) openUserAlbum(username, albumName string) (webdav.File, error) {
	ua, ok := m.albums[albumName]
	if !ok {
		return nil, errors.Errorf("album %s is not found", albumName)
	}
	aid, ok := ua[username]
	if !ok {
		return nil, errors.Errorf("album %s is not found for user %s", albumName, username)
	}

	return m.openUserAlbumByID(username, albumName, aid)
}

func (m *myFS) openUserAlbumByID(userName, albumName string, albumID int) (webdav.File, error) {
	var names []string
	err := dbx.InQuery(m.h.db, func(ctx context.Context, tx *sql.Tx) error {
		//return home dir if request is from localhost
		var err error
		names, err = album.ListAssetIDsWithDate(ctx, tx, albumID)
		return err
	})
	if err != nil {
		return nil, err
	}
	userPhotos := &lomoFile{isDir: true, name: albumName}
	for _, n := range names {
		// check if files
		if !m.isAssetExist(userName, n) {
			continue
		}
		userPhotos.files = append(userPhotos.files, &fileInfo{name: n})
	}
	return userPhotos, nil
}

func (m *myFS) openUserPhotos(username string) (webdav.File, error) {
	u, ok := m.users[username]
	if !ok {
		return nil, errors.Errorf("user %s not found", username)
	}
	m.h.memdb.Lock()
	years := m.h.memdb.GetYears(u.ID)
	m.h.memdb.Unlock()

	userPhotos := &lomoFile{isDir: true, name: dirPhotos}
	for _, y := range years {
		userPhotos.files = append(userPhotos.files, &fileInfo{name: strconv.Itoa(y)})
	}
	return userPhotos, nil
}

func (m *myFS) openUserPhotosYear(username, year string, includeAll bool) (webdav.File, error) {
	u, ok := m.users[username]
	if !ok {
		return nil, errors.Errorf("user %s not found", username)
	}

	y, err := strconv.Atoi(year)
	if err != nil {
		return nil, err
	}

	if includeAll {
		m.h.memdb.Lock()
		assets := m.h.memdb.GetAssetsByYear(u.ID, y, true)
		m.h.memdb.Unlock()

		userPhotos := &lomoFile{isDir: true, name: year}
		for _, mon := range assets.Months {
			for _, d := range mon.Days {
				for _, a := range d.Assets {
					name := ext.NormalizeAssetNameString(y, mon.Month, d.Day, a.Name)
					if !m.isAssetExist(username, name) {
						continue
					}
					fmt.Printf("---- exist name: %s\n", name)
					userPhotos.files = append(userPhotos.files, &fileInfo{name: name})
				}
			}
		}
		fmt.Printf("---- return photos: %v\n", userPhotos)
		return userPhotos, nil
	}

	m.h.memdb.Lock()
	months := m.h.memdb.GetMonths(u.ID, y)
	m.h.memdb.Unlock()

	userPhotos := &lomoFile{isDir: true, name: year}
	for _, m := range months {
		userPhotos.files = append(userPhotos.files, &fileInfo{name: mkDateName(m)})
	}
	return userPhotos, nil
}

func (m *myFS) openUserPhotosMonth(username, year, month string, includeAssets bool) (webdav.File, error) {
	u, ok := m.users[username]
	if !ok {
		return nil, errors.Errorf("user %s not found", username)
	}

	y, err := strconv.Atoi(year)
	if err != nil {
		return nil, err
	}

	mon, err := strconv.Atoi(month)
	if err != nil {
		return nil, err
	}

	userPhotos := &lomoFile{isDir: true, name: mkDateName(mon)}
	if includeAssets {
		m.h.memdb.Lock()
		days := m.h.memdb.GetDays(u.ID, y, mon)
		for _, d := range days {
			assets := m.h.memdb.GetDayAssets(u.ID, y, mon, d)
			for _, a := range assets {
				name := ext.NormalizeAssetNameString(y, mon, d, a)
				if !m.isAssetExist(username, name) {
					continue
				}
				userPhotos.files = append(userPhotos.files, &fileInfo{name: name})
			}
		}
		m.h.memdb.Unlock()

		return userPhotos, nil
	}
	m.h.memdb.Lock()
	days := m.h.memdb.GetDays(u.ID, y, mon)
	m.h.memdb.Unlock()

	for _, d := range days {
		userPhotos.files = append(userPhotos.files, &fileInfo{name: mkDateName(d)})
	}
	return userPhotos, nil
}

func (m *myFS) openUserPhotosDay(username, year, month, day string) (*lomoFile, error) {
	u, ok := m.users[username]
	if !ok {
		return nil, errors.Errorf("user %s not found", username)
	}

	y, err := strconv.Atoi(year)
	if err != nil {
		return nil, err
	}

	mon, err := strconv.Atoi(month)
	if err != nil {
		return nil, err
	}

	d, err := strconv.Atoi(day)
	if err != nil {
		return nil, err
	}

	m.h.memdb.Lock()
	assets := m.h.memdb.GetDayAssets(u.ID, y, mon, d)
	m.h.memdb.Unlock()

	userPhotos := &lomoFile{isDir: true, name: mkDateName(d)}
	for _, a := range assets {
		name := ext.NormalizeAssetNameString(y, mon, d, a)
		if !m.isAssetExist(username, name) {
			continue
		}
		userPhotos.files = append(userPhotos.files, &fileInfo{name: name})
	}
	return userPhotos, nil
}

func (m *myFS) openLivePhotoSingleFile(username, year, month, day, parent, name string) (*lomoFile, error) {
	u, ok := m.users[username]
	if !ok {
		return nil, errors.Errorf("user %s not found", username)
	}

	y, err := strconv.Atoi(year)
	if err != nil {
		return nil, err
	}

	mon, err := strconv.Atoi(month)
	if err != nil {
		return nil, err
	}

	d, err := strconv.Atoi(day)
	if err != nil {
		return nil, err
	}

	assetDir, _, err := common.GetUserPhotoMasterPreviewDir(u.HomeDir, y, mon, d, m.h.conf.FolderPerm)
	if err != nil {
		return nil, err
	}
	lf := &lomoFile{name: name, path: filepath.Join(assetDir, parent)}

	lf.parent, err = zip.OpenReader(lf.path)
	if err != nil {
		return nil, err
	}

	for _, f := range lf.parent.File {
		//fmt.Printf("--- %s -> %s\n", name, f.Name)
		if f.Name != name {
			continue
		}

		if lf.fileInfo == nil {
			fi := f.FileInfo()
			lf.fileInfo = &fileInfo{
				name: name,
				size: fi.Size(),
				modTime: fi.ModTime(),
			}
		}
		lf.subReadCloser, err = f.Open()
		if err != nil {
			return nil, err
		}
		//fmt.Printf("---- add open files %+v\n", lf)
		return lf, nil
	}
	return nil, common.ErrAssetNotExistForUser
}

func (m *myFS) openLivePhoto(name, path string) (*lomoFile, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	lf := &lomoFile{isDir: true, name: name, path: path}
	for _, f := range zr.File {
		//fmt.Printf("--- openLivePhoto %s add files: %s\n", name, f.Name)
		e, err := ext.GetExtID(strings.ToLower(strings.TrimPrefix(filepath.Ext(f.Name), ".")))
		if err != nil {
			logrus.Warnf("live photo %s embedded unrecognized file %s", name, f.Name)
			continue
		}
		if e == ext.HEIF || e == ext.HEIC{
			logrus.Warnf("live photo %s embedded not supported file %s", name, f.Name)
			continue
		}
		lf.files = append(lf.files, &fileInfo{name: f.Name})
	}
	return lf, nil
}

func (m *myFS) openUserPhotoDayAsset(username, year, month, day, name string) (*lomoFile, error) {
	u, ok := m.users[username]
	if !ok {
		return nil, errors.Errorf("user %s not found", username)
	}

	y, err := strconv.Atoi(year)
	if err != nil {
		return nil, err
	}

	mon, err := strconv.Atoi(month)
	if err != nil {
		return nil, err
	}

	d, err := strconv.Atoi(day)
	if err != nil {
		return nil, err
	}

	assetDir, _, err := common.GetUserPhotoMasterPreviewDir(u.HomeDir, y, mon, d, m.h.conf.FolderPerm)
	if err != nil {
		return nil, err
	}
	assetPath := filepath.Join(assetDir, name)

	if isLivePhoto(name) {
		return m.openLivePhoto(name, assetPath)
	}

	f, err := os.Open(assetPath)
	if err != nil {
		return nil, err
	}
	lf := &lomoFile{isDir: false, name: name, file: f, path: assetPath}
	//fmt.Printf("---- opening file: %s - %+v\n", assetPath, *lf)
	return lf, nil
}

func (m *myFS) handleOpenFileYyyy(parts [] string) (webdav.File, error) {
	switch len(parts) {
	case 3:
		return m.openUserPhotosYear(parts[0], parts[2], true)
	case 4:
		y, mon, d, _, err := ext.ParseNormalizedAssetName(parts[3])
		if err != nil {
			return nil, err
		}
		if !isLivePhoto(parts[3]) {
			return m.openUserPhotoDayAsset(parts[0], strconv.Itoa(y), strconv.Itoa(mon), strconv.Itoa(d), parts[3])
		}

		u, ok := m.users[parts[0]]
		if !ok {
			return nil, errors.Errorf("user %s not found", parts[0])
		}

		assetDir, _, err := common.GetUserPhotoMasterPreviewDir(u.HomeDir, y, mon, d, m.h.conf.FolderPerm)
		if err != nil {
			return nil, err
		}
		return m.openLivePhoto(parts[3], filepath.Join(assetDir, parts[3]))
	case 5:
		if isLivePhoto(parts[3]) {
			y, mon, d, _, err := ext.ParseNormalizedAssetName(parts[3])
			if err != nil {
				return nil, err
			}
			return m.openLivePhotoSingleFile(parts[0], strconv.Itoa(y), strconv.Itoa(mon), strconv.Itoa(d),
				parts[3], parts[4])
		}
	}

	return nil, common.ErrNotImplementedFormat
}

func (m *myFS) handleOpenFileYyyymm(parts [] string) (webdav.File, error) {
	switch len(parts) {
	case 3:
		return m.openUserPhotosYear(parts[0], parts[2], false)
	case 4:
		return m.openUserPhotosMonth(parts[0], parts[2], parts[3], true)
	case 5:
		y, mon, d, _, err := ext.ParseNormalizedAssetName(parts[4])
		if err != nil {
			return nil, err
		}
		if !isLivePhoto(parts[4]) {
			return m.openUserPhotoDayAsset(parts[0], parts[2], parts[3], strconv.Itoa(d), parts[4])
		}
		u, ok := m.users[parts[0]]
		if !ok {
			return nil, errors.Errorf("user %s not found", parts[0])
		}

		assetDir, _, err := common.GetUserPhotoMasterPreviewDir(u.HomeDir, y, mon, d, m.h.conf.FolderPerm)
		if err != nil {
			return nil, err
		}
		return m.openLivePhoto(parts[4], filepath.Join(assetDir, parts[4]))
	case 6:
		if isLivePhoto(parts[4]) {
			y, mon, d, _, err := ext.ParseNormalizedAssetName(parts[4])
			if err != nil {
				return nil, err
			}
			return m.openLivePhotoSingleFile(parts[0], strconv.Itoa(y), strconv.Itoa(mon), strconv.Itoa(d),
				parts[4], parts[5])
		}
	}

	return nil, common.ErrNotImplementedFormat
}

func (m *myFS) handleOpenFileYyyymmdd(parts []string) (webdav.File, error) {
	switch len(parts) {
	case 3:
		return m.openUserPhotosYear(parts[0], parts[2], false)
	case 4:
		return m.openUserPhotosMonth(parts[0], parts[2], parts[3], false)
	case 5:
		return m.openUserPhotosDay(parts[0], parts[2], parts[3], parts[4])
	case 6:
		return m.openUserPhotoDayAsset(parts[0], parts[2], parts[3], parts[4], parts[5])
	case 7:
		if isLivePhoto(parts[5]) {
			return m.openLivePhotoSingleFile(parts[0], parts[2], parts[3], parts[4], parts[5], parts[6])
		}
	}

	return nil, common.ErrNotImplementedFormat
}

func (m *myFS) isDirInAlbum(username, dir string) (map[string]struct{}, int) {
	nextDirs := map[string]struct{}{}
	//fmt.Printf("--- compare %s in %+v\n", dir, m.albums)
	for albumName, ua := range m.albums {
		if !strings.HasPrefix(albumName, dir) {
			continue
		}
		id, ok := ua[username]
		if !ok {
			// album name is good, but not belong to the user
			continue
		}

		// same name if length is same
		if len(albumName) == len(dir) {
			return nil, id
		}
		// trim dir and see if it starts with /.
		subname := strings.TrimPrefix(albumName, dir)
		if !strings.HasPrefix(subname, "/") {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(subname, "/"), "/")
		nextDirs[parts[0]] = struct{}{}
	}
	return nextDirs, -1
}

func (m *myFS) handleOpenFileAlbum(parts []string) (webdav.File, error) {
	//fmt.Printf("---- handleOpenFileAlbum: %s\n", parts)
	// TODO: is windows also using '/' as path for importer?
	// steps:
	// 1. find the whole path and see if it is one directory name
	// otherwise, if length of dirs is equal to 3 (<username>/Albums/<albumname>), return
	// 2. if the last part is live photo, check the rest of it
	// 3. if the 2nd last part is live photo, check the rest of it
	albumName := strings.Join(parts[2:], "/")
	dirs, albumID := m.isDirInAlbum(parts[0], albumName)
	if albumID > 0 {
		return m.openUserAlbumByID(parts[0], albumName, albumID)
	}
	if len(dirs) > 0 {
		userPhotos := &lomoFile{isDir: true, name: parts[len(parts) - 1]}
		for n := range dirs {
			userPhotos.files = append(userPhotos.files, &fileInfo{name: n})
		}
		return userPhotos, nil
	}
	if len(parts) == 3 {
		return nil, common.ErrNotImplementedFormat
	}

	// step 2: check if last part is live photo
	assetName := parts[len(parts) - 1]
	_, albumID = m.isDirInAlbum(parts[0], strings.Join(parts[2:len(parts) - 1], "/"))
	if albumID > 0 {
		// current path is valid album name and current asset is live photo.
		y, mon, d, _, err := ext.ParseNormalizedAssetName(assetName)
		if err != nil {
			return nil, err
		}
		if !isLivePhoto(assetName) {
			return m.openUserPhotoDayAsset(parts[0], strconv.Itoa(y), strconv.Itoa(mon), strconv.Itoa(d), assetName)
		}
		u, ok := m.users[parts[0]]
		if !ok {
			return nil, errors.Errorf("user %s not found", parts[0])
		}

		assetDir, _, err := common.GetUserPhotoMasterPreviewDir(u.HomeDir, y, mon, d, m.h.conf.FolderPerm)
		if err != nil {
			return nil, err
		}
		return m.openLivePhoto(assetName, filepath.Join(assetDir, assetName))
	}
	if len(parts) == 4 {
		return nil, common.ErrNotImplementedFormat
	}

	// step 3: check if the 2nd last part is live photo
	assetName = parts[len(parts) - 2]
	if !isLivePhoto(assetName) {
		return nil, common.ErrNotImplementedFormat
	}
	_, albumID = m.isDirInAlbum(parts[0], strings.Join(parts[2:len(parts) - 2], "/"))
	if albumID < 0 {
		return nil, common.ErrNotImplementedFormat
	}
	// current path is valid album name, and current asset is one inside live photo asset
	y, mon, d, _, err := ext.ParseNormalizedAssetName(assetName)
	if err != nil {
		return nil, err
	}
	return m.openLivePhotoSingleFile(parts[0], strconv.Itoa(y), strconv.Itoa(mon), strconv.Itoa(d),
		assetName, parts[len(parts) - 1])
}

func (m *myFS) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (f webdav.File, e error) {
	//fmt.Printf("---- OpenFile: %s, flag: %d, perm: %s\n", name, flag, perm)
	if name == "/" {
		return m.openRoot()
	}
	parts := strings.Split(strings.TrimPrefix(strings.TrimSuffix(name, "/"), "/"), "/")
	if len(parts) == 0 {
		return m.openRoot()
	}
	if strings.HasPrefix(parts[len(parts) - 1], ".") {
		return nil, errors.Wrapf(common.ErrNotImplementedFormat, "OpenFiles %s", name)
	}
	_, ok := knownOSFiles[parts[len(parts) - 1]]
	if ok {
		return nil, errors.Wrapf(common.ErrNotImplementedFormat, "OpenFiles %s", name)
	}

	switch len(parts) {
	case 1:
		userFile := &lomoFile{isDir: true, name: parts[0],
			files: []os.FileInfo{
				&fileInfo{name: dirPhotos},
				&fileInfo{name: dirAlbums},
			},
		}
		return userFile, nil
	case 2:
		switch parts[1] {
		case dirPhotos:
			return m.openUserPhotos(parts[0])
		case dirAlbums:
			return m.openUserAlbums(parts[0])
		}
	}
	if parts[1] == dirAlbums {
		f, e = m.handleOpenFileAlbum(parts)
	} else if parts[1] == dirPhotos {
		switch m.h.webdavDirLayout {
		case dirLayoutYyyy:
			f, e = m.handleOpenFileYyyy(parts)
		case dirLayoutYyyymm:
			f, e = m.handleOpenFileYyyymm(parts)
		default:
			f, e = m.handleOpenFileYyyymmdd(parts)
		}
	} else {
		e = common.ErrNotImplementedFormat
	}
	if e != nil {
		e = errors.Wrapf(e, "OpenFiles %s", name)
	}
	return
}

func (m *myFS) statUserPhotoDayAsset(username, year, month, day, name string) (*fileInfo, error) {
	u, ok := m.users[username]
	if !ok {
		return nil, errors.Errorf("user %s not found", username)
	}

	y, err := strconv.Atoi(year)
	if err != nil {
		return nil, err
	}

	mon, err := strconv.Atoi(month)
	if err != nil {
		return nil, err
	}

	d, err := strconv.Atoi(day)
	if err != nil {
		return nil, err
	}

	assetDir, _, err := common.GetUserPhotoMasterPreviewDir(u.HomeDir, y, mon, d, m.h.conf.FolderPerm)
	if err != nil {
		return nil, err
	}
	stat, err := os.Stat(filepath.Join(assetDir, name))
	if err != nil {
		return nil, err
	}

	return &fileInfo{info: stat}, nil
}

func (m *myFS) statLivePhotoSingleFile(username, year, month, day, parent, name string) (*fileInfo, error) {
	u, ok := m.users[username]
	if !ok {
		return nil, errors.Errorf("user %s not found", username)
	}

	y, err := strconv.Atoi(year)
	if err != nil {
		return nil, err
	}

	mon, err := strconv.Atoi(month)
	if err != nil {
		return nil, err
	}

	d, err := strconv.Atoi(day)
	if err != nil {
		return nil, err
	}

	assetDir, _, err := common.GetUserPhotoMasterPreviewDir(u.HomeDir, y, mon, d, m.h.conf.FolderPerm)
	if err != nil {
		return nil, err
	}

	parentFile, err := zip.OpenReader(filepath.Join(assetDir, parent))
	if err != nil {
		return nil, err
	}
	defer parentFile.Close()

	for _, f := range parentFile.File {
		//fmt.Printf("--- %s -> %s\n", name, f.Name)
		if f.Name != name {
			continue
		}

		fi := f.FileInfo()
		info := &fileInfo{name: name, size: fi.Size(), modTime: fi.ModTime()}
		//fmt.Printf("---- return live photo single file: %+v\n", *info)
		return info, nil
	}
	return nil, common.ErrAssetNotExistForUser
}

func (m *myFS) handleStatYyyy(parts []string) (os.FileInfo, error) {
	switch len(parts) {
	case 4:
		if isLivePhoto(parts[3]) {
			return &fileInfo{name: parts[len(parts)-1]}, nil
		}
		y, mon, d, _, err := ext.ParseNormalizedAssetName(parts[3])
		if err != nil {
			return nil, err
		}
		return m.statUserPhotoDayAsset(parts[0], strconv.Itoa(y), strconv.Itoa(mon), strconv.Itoa(d), parts[3])
	case 5:
		if isLivePhoto(parts[3]){
			y, mon, d, _, err := ext.ParseNormalizedAssetName(parts[3])
			if err != nil {
				return nil, err
			}
			return m.statLivePhotoSingleFile(parts[0],
					strconv.Itoa(y), strconv.Itoa(mon), strconv.Itoa(d),
					parts[3], parts[4])
		}
	}
	return nil, common.ErrNotImplementedFormat
}

func (m *myFS) handleStatYyyymm(parts []string) (os.FileInfo, error) {
	switch len(parts) {
	case 4: // month or asset name (including zip)
		return &fileInfo{name: parts[len(parts) - 1]}, nil
	case 5: // asset name
		if isLivePhoto(parts[4]) {
			return &fileInfo{name: parts[len(parts) - 1]}, nil
		}
		_, _, d, _, err := ext.ParseNormalizedAssetName(parts[4])
		if err != nil {
			return nil, err
		}
		return m.statUserPhotoDayAsset(parts[0], parts[2], parts[3], strconv.Itoa(d), parts[4])
	case 6:
		if isLivePhoto(parts[4]) {
			_, _, d, _, err := ext.ParseNormalizedAssetName(parts[4])
			if err != nil {
				return nil, err
			}
			return m.statLivePhotoSingleFile(parts[0], parts[2], parts[3], strconv.Itoa(d), parts[4], parts[5])
		}
	}
	return nil, common.ErrNotImplementedFormat
}

func (m *myFS) handleStatYyyymmdd(parts []string) (os.FileInfo, error) {
	switch len(parts) {
	case 4:
		return &fileInfo{name: parts[len(parts) - 1]}, nil
	case 5:
		return &fileInfo{name: parts[len(parts) - 1]}, nil
	case 6:
		if isLivePhoto(parts[5]) {
			return &fileInfo{name: parts[len(parts) - 1]}, nil
		}
		return m.statUserPhotoDayAsset(parts[0], parts[2], parts[3], parts[4], parts[5])
	case 7:
		if isLivePhoto(parts[5]) {
			//fmt.Printf("----- sub zip file: %+v\n", m.openFiles)
			return m.statLivePhotoSingleFile(parts[0], parts[2], parts[3], parts[4], parts[5], parts[6])
		}
	}
	return nil, common.ErrNotImplementedFormat
}

func (m *myFS) handleStatAlbum(parts []string) (fi os.FileInfo, e error) {
	albumName := strings.Join(parts[2:], "/")
	dirs, albumID := m.isDirInAlbum(parts[0], albumName)
	if albumID > 0 || len(dirs) > 0 {
		return &fileInfo{name: parts[len(parts) - 1]}, nil
	}

	// normal asset should be 1: username, 2: Albums, 3: Album name, 4: asset name
	// normal live photo should be 1: username, 2: Albums, 3: Album name, 4: asset zip, 5: single file inside zip
	endAssetName := parts[len(parts) - 1]
	parentAssetName := parts[len(parts) - 2]
	if isLivePhoto(endAssetName) {
		return &fileInfo{name: parts[len(parts)-1]}, nil
	}
	if isLivePhoto(parentAssetName) {
		y, mon, d, _, err := ext.ParseNormalizedAssetName(parentAssetName)
		if err != nil {
			return nil, err
		}
		return m.statLivePhotoSingleFile(parts[0],
			strconv.Itoa(y), strconv.Itoa(mon), strconv.Itoa(d),
			parentAssetName, endAssetName)
	}
	y, mon, d, _, err := ext.ParseNormalizedAssetName(endAssetName)
	if err != nil {
		return nil, err
	}
	return m.statUserPhotoDayAsset(parts[0], strconv.Itoa(y), strconv.Itoa(mon), strconv.Itoa(d), endAssetName)
}

func (m *myFS) Stat(ctx context.Context, name string) (fi os.FileInfo, e error) {
	if name == "/" {
		//fmt.Printf("stat: %s\n", name)
		return m.rootFileInfo, nil
	}
	parts := strings.Split(strings.TrimPrefix(strings.TrimSuffix(name, "/"), "/"), "/")
	if len(parts) == 0 {
		return m.rootFileInfo, nil
	}
	if strings.HasPrefix(parts[len(parts) - 1], ".") {
		return nil, errors.Wrapf(common.ErrNotImplementedFormat, "Stat %s", name)
	}
	_, ok := knownOSFiles[parts[len(parts) - 1]]
	if ok {
		return nil, errors.Wrapf(common.ErrNotImplementedFormat, "Stat %s", name)
	}
	//fmt.Printf("Stat: %s - %v\n", name, parts)
	switch len(parts) {
	case 1: // username
		return &fileInfo{name: parts[len(parts) - 1]}, nil
	case 2: // photo or albums
		return &fileInfo{name: parts[len(parts) - 1]}, nil
	case 3: // yyyy or album name
		return &fileInfo{name: parts[len(parts) - 1]}, nil
	}
	switch parts[1] {
	case dirAlbums:
		fi, e = m.handleStatAlbum(parts)
	case dirPhotos:
		switch m.h.webdavDirLayout {
		case dirLayoutYyyy:
			fi, e = m.handleStatYyyy(parts)
		case dirLayoutYyyymm:
			fi, e = m.handleStatYyyymm(parts)
		default:
			fi, e = m.handleStatYyyymmdd(parts)
		}
	default:
		e = common.ErrNotImplementedFormat
	}
	if e != nil {
		e = errors.Wrapf(e, "Stat %s", name)
	}
	return
}

func (m *myFS) RemoveAll(ctx context.Context, name string) error {
	//fmt.Printf("remove all: %s\n", name)
	return errors.New("not implemented")
}

func (m *myFS) Rename(ctx context.Context, oldName, newName string) error {
	//fmt.Printf("rename: %s->%s\n", oldName, newName)
	return errors.New("not implemented")
}

type fileInfo struct {
	name string
	size int64
	modTime time.Time
	info os.FileInfo
}

// Name is base name of the file
func (fi *fileInfo) Name() string {
	//fmt.Printf("fileinfo: name: %+v\n", *fi)
	if fi.info != nil {
		return fi.info.Name()
	}
	return fi.name
}

// length in bytes for regular files; system-dependent for others
func (fi *fileInfo) Size() int64 {
	//fmt.Printf("fileinfo: size: %+v\n", *fi)
	if fi.info != nil {
		return fi.info.Size()
	}
	return fi.size
}

// file mode bits
func (fi *fileInfo) Mode() os.FileMode {
	//fmt.Printf("fileinfo: mode: %+v\n", *fi)
	if fi.info != nil {
		return fi.info.Mode()
	}
	return 0655
}

// modification time
func (fi *fileInfo) ModTime() time.Time {
	//fmt.Printf("fileinfo: ModTime: %+v\n", *fi)
	if fi.info != nil {
		return fi.info.ModTime()
	}
	return fi.modTime
}

// abbreviation for Mode().IsDir()
func (fi *fileInfo) IsDir() bool {
	//fmt.Printf("fileinfo: IsDir: %+v\n", *fi)
	if fi.info != nil {
		return false
	}
	// could be file inside zip file
	return fi.size == 0
}

func (fi *fileInfo) Sys() interface{} {
	return nil
}

type lomoFile struct {
	isDir bool
	name string
	path string  // webdev path
	file *os.File
	fileInfo *fileInfo
	files []os.FileInfo
	parent *zip.ReadCloser
	subReadCloser io.ReadCloser
	subDataBuffer []byte  // tmp buffer for seek
	subSeekPos int
}

func (f *lomoFile) Close() error {
	//fmt.Printf("---- lomofile: close: %s\n", f.name)
	if f.isDir {
		return nil
	} else if f.file != nil {
		err := f.file.Close()
		f.file = nil
		//fmt.Printf("---- lomofile: close result: %+v, %s\n", *f, err)
		return err
	} else if f.subReadCloser != nil {
		f.subDataBuffer = nil
		err := f.subReadCloser.Close()
		f.subReadCloser = nil
		if err != nil {
			logrus.Warnf("close zip subfile %s.%s fail: %s", f.path, f.name, err)
		}
	}
	if f.parent != nil {
		err := f.parent.Close()
		f.parent = nil
		return err
	}
	return nil
}

func (f *lomoFile) Read(p []byte) (n int, err error) {
	//fmt.Printf("---- lomofile: read: %s, %v\n", f.name, f.subReadCloser)
	if f.isDir {
		return 0, nil
	} else if f.subReadCloser != nil {
		/*
		if f.subSeekPos >= 0 {
			start := f.subSeekPos
			size := len(f.subDataBuffer) - start
			if size > len(p) {
				size = len(p)
			}
			end := f.subSeekPos + size
			if end > len(f.subDataBuffer) {
				end = len(f.subDataBuffer)
			}
			if start < end {
				copy(p, f.subDataBuffer[start:end])
				f.subSeekPos += end - start
				for i, d := range p {
					fmt.Printf("%02x ", d)
					if (i+1) % 16 == 0 {
						fmt.Print("\n")
					}
				}
				fmt.Print("\n")
				return end - start, nil
			}
		}
			 */
		size, err := f.subReadCloser.Read(p)
		if err != nil {
			//fmt.Printf("---- lomofile: live photo read length expect %d, actual %d: %+v\n", len(p), size, err)
		} else {
			//fmt.Printf("---- lomofile: live photo read length expect %d, actual %d\n", len(p), size)
		}
		// copy read data to tmp buffer in case client want to seek
			/*
		if f.subDataBuffer == nil {
			f.subDataBuffer = []byte{}
		}
		f.subDataBuffer = append(f.subDataBuffer, p[:size]...)
		for i, d := range p {
			fmt.Printf("%02x ", d)
			if (i+1) % 16 == 0 {
				fmt.Print("\n")
			}
		}
		fmt.Print("\n")
		 */
		return size, err
	} else if f.file != nil {
		size, err := f.file.Read(p)
		if err != nil {
			//fmt.Printf("---- lomofile: read length expect %d, actual %d: %+v\n", len(p), size, err)
		} else {
			//fmt.Printf("---- lomofile: read length expect %d, actual %d\n", len(p), size)
		}
		return size, err
	}
	return 0, errors.Errorf("%s not opened yet", f.name)
}

func (f *lomoFile) Seek(offset int64, whence int) (int64, error) {
	//fmt.Printf("---- lomofile: seek request (%d, %d) for %s, %v\n", offset, whence, f.name, f.subReadCloser)
	if f.isDir {
		return 0, nil
	} else if f.file != nil {
		return f.file.Seek(offset, whence)
	} else if f.subReadCloser != nil {
		if offset == 0 {
			switch whence {
			case io.SeekStart:
				f.subSeekPos = 0
				fallthrough
			case io.SeekCurrent:
				return 0, nil
			case io.SeekEnd:
				return f.fileInfo.size, nil
			}
		}
		logrus.Warnf("not supported seek request (%d, %d) for %+v", offset, whence, *f)
		return 0, nil
	}
	return 0, errors.Errorf("%s not opened yet", f.name)
}

func (f *lomoFile) Readdir(count int) ([]os.FileInfo, error) {
	//fmt.Printf("lomofile: Readdir: %+v\n", *f)
	if !f.isDir {
		return nil, nil
	}
	return f.files, nil
}

func (f *lomoFile) Stat() (os.FileInfo, error) {
	//fmt.Printf("lomofile: stat: %s, %v\n", f.name, f.subReadCloser)
	if f.file != nil {
		return f.file.Stat()
	} else if f.fileInfo != nil {
		return f.fileInfo, nil
	}
	return &fileInfo{name: f.name}, nil
}

func (f *lomoFile) Write(p []byte) (n int, err error) {
	//fmt.Printf("lomofile: write: %+v\n", *f)
	return 0, errors.Errorf("dir (%s) not writable", f.name)
}