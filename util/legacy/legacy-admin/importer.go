package main

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
	"strings"

	"github.com/sirupsen/logrus"

	"bitbucket.org/lomoware/lomo-backend/common"
)

type importerHandler struct {
	dbDir    string
	mountDir string
	useUTC   bool
}

type failedAsset struct {
	Assetpath string
	Err       string
}

type failedAssets struct {
	Assets []failedAsset
}

// Importer serves static file import
func Importer(dbDir, mountDir string, useUTC bool) http.Handler {
	return &importerHandler{dbDir: dbDir, mountDir: mountDir, useUTC: useUTC}
}

func (b *importerHandler) validateUser(username string) (string, error) {
	db, err := sql.Open("sqlite3", path.Join(b.dbDir, assetdb))
	if err != nil {
		return "", err
	}
	defer db.Close()

	stmt, err := db.Prepare("select homedir from user where userid = ?")
	if err != nil {
		return "", err
	}
	defer stmt.Close()
	var homedir string
	if err := stmt.QueryRow(username).Scan(&homedir); err != nil {
		return "", err
	}

	return homedir, nil
}

func (b *importerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	userid := r.URL.Query().Get("userid")
	dir := path.Join(b.mountDir, r.URL.Path)

	userdir, err := b.validateUser(userid)
	if err != nil {
		common.WriteError(w, err)
		return
	}

	parts := strings.Split(r.URL.Path, "/")
	device := parts[0]
	fmt.Printf("device name %s, user dir %s\n", device, userdir)

	if assets, err := b.importAssets(dir); err != nil {
		logrus.Warn(err)
		common.WriteBody(w, assets)
	}
}

func (b *importerHandler) importAssets(srcdir string) (*failedAssets, error) {
	assets := &failedAssets{Assets: []failedAsset{}}
	files, err := ioutil.ReadDir(srcdir)
	if err != nil {
		return assets, err
	}

	// use path's mounted point as device name
	for _, file := range files {
		p := path.Join(srcdir, file.Name())
		if file.IsDir() {
			a, err := b.importAssets(p)
			if err != nil {
				assets.Assets = append(assets.Assets, a.Assets...)
				logrus.Warnf("while importing assets: %v", err)
			}
		}
		/*
			else {
				if _, _, _, _, err := asset.CreateAsset(db, device, username, p, strings.ToLower(strings.Trim(path.Ext(p), ".")), "", b.useUTC, false); err != nil {
					//continue next one
					logrus.Warn(err)
					assets.Assets = append(assets.Assets, failedAsset{Assetpath: p, Err: err.Error()})
				}
			}
		*/
	}
	return assets, nil
}
