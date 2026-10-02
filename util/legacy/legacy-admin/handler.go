package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"path"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"github.com/sirupsen/logrus"

	"github.com/gorilla/mux"

	_ "github.com/mattn/go-sqlite3"
)

const (
	assetdb = "assets.db"
)

// Handler is structure to handle http request.
type Handler struct {
	basedir       string
	mountdir      string
	tokenDuration time.Duration
}

// NewHandler construct handler object.
func NewHandler(dir, mdir string) *Handler {
	// 30 days token
	return &Handler{basedir: dir, mountdir: mdir, tokenDuration: 720 * time.Hour}
}

// CreateRouter associates handler with URL endpoints.
func (h *Handler) CreateRouter() *mux.Router {
	r := mux.NewRouter()
	r.HandleFunc("/init/{deviceID}", h.init).Methods("POST")
	r.HandleFunc("/login", h.login).Methods("GET")
	r.HandleFunc("/user", h.listUsers).Methods("GET")
	r.HandleFunc("/user/{userID}/{deviceID}/{password}", h.createUser).Methods("POST")
	r.HandleFunc("/user/{userID}", h.deleteUser).Methods("DELETE")
	r.HandleFunc("/mount/usb", h.listMounts).Methods("GET")
	r.HandleFunc("/mount/usb/{deviceID}", h.mount).Methods("POST")
	r.HandleFunc("/mount/smb", h.connectSamba).Methods("POST")
	r.HandleFunc("/umount/{path}", h.umount).Methods("DELETE")
	r.HandleFunc("/scan", h.scan).Methods("GET")
	r.PathPrefix("/browse").Handler(http.StripPrefix("/browse/", StaticFileServer(h.mountdir))).Methods("GET")
	r.PathPrefix("/import").Handler(http.StripPrefix("/import/", Importer(h.basedir, h.mountdir, true))).Methods("GET")

	return r
}

func (h *Handler) checkToken(db *sql.DB, userid, token string) error {
	stmt, err := db.Prepare("select token, expiretime from token where userid = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()
	var tokenQuery string
	var expiretime string
	err = stmt.QueryRow(userid).Scan(&tokenQuery, &expiretime)
	if err != nil {
		return err
	}

	// TODO: check expire time
	logrus.Println("TODO: not check token expire date yet")

	////////////
	if token != tokenQuery {
		return common.ErrInvalidToken
	}

	return nil
}

// FIXME: to be removed after authentication complete
func initToken(tx *sql.Tx, userid string) error {
	stmt, err := tx.Prepare("insert into token (token, device, userid, createtime, expiretime) values (573944500, 'iphonex', ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(userid, time.Now(), time.Now())
	return err
}

/*
// err is not nil, means user is not in DB, otherwise, it is in db
func (h *Handler) checkUser(tx *sql.Tx, userid string) error {
	stmt, err := tx.Prepare("select userid from user where userid = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()
	var name string
	err = stmt.QueryRow(userid).Scan(&name)

	return err

}
*/
func (h *Handler) validate(username, token string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", path.Join(h.basedir, assetdb))
	if err != nil {
		return nil, err
	}

	if err := h.checkToken(db, username, token); err != nil {
		return nil, err
	}

	return db, nil
}

func (h *Handler) init(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	username := q.Get("username")
	password := q.Get("password")

	// get device mount path
	devicename := fmt.Sprintf("/dev/%s", mux.Vars(r)["deviceID"])
	device, err := readDevice(devicename)
	if err != nil {
		common.WriteError(w, err)
		return
	}
	if device.Path == "" {
		common.WriteError(w, common.ErrDeviceNotMount)
		return
	}

	// create home folder for the user
	homedir := path.Join(device.Path, username)
	if err := os.MkdirAll(homedir, 0600); err != nil {
		common.WriteError(w, err)
	}

	db, err := sql.Open("sqlite3", path.Join(h.basedir, assetdb))
	if err != nil {
		common.WriteError(w, err)
		return
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		common.WriteError(w, err)
		return
	}
	defer tx.Rollback()

	sqlStmt := `
CREATE TABLE IF NOT EXISTS user (
 userid TEXT PRIMARY KEY,
 password TEXT NOT NULL,
 phone TEXT,
 email TEXT,
 nickname TEXT,
 homedir TEXT,
 admin INTEGER,
 status INTEGER,
 createtime INTEGER NOT NULL,
 lastmodifiedtime INTEGER NOT NULL,
 lastlogintime INTEGER NULL
);

CREATE TABLE IF NOT EXISTS groups (
 groupid INTEGER PRIMARY KEY AUTOINCREMENT,
 groupname TEXT,
 ownerid TEXT,
 userid TEXT,
 createtime INTEGER NOT NULL,
 lastmodifiedtime INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS asset (
 assetid INTEGER PRIMARY KEY AUTOINCREMENT,
 userid TEXT,
 path TEXT,
 ext TEXT,
 hash TEXT NOT NULL,
 device TEXT NOT NULL,
 createtime DATETIME NOT NULL,
 uploadtime INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS share(
 sharedid TEXT PRIMARY KEY,
 userid TEXT NOT NULL,
 assetid TEXT NOT NULL,
 createtime INTEGER NOT NULL,
 expiretime INTEGER NOT NULL,
 lastmodifiedtime INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS token(
 token TEXT NOT NULL,
 userid TEXT NOT NULL,
 device TEXT NOT NULL,
 createtime INTEGER NOT NULL,
 expiretime INTEGER NOT NULL
);`
	if _, err = tx.Exec(sqlStmt); err != nil {
		common.WriteError(w, err)
		return
	}

	stmt, err := tx.Prepare("insert into user (userid, password, homedir, admin, createtime, lastmodifiedtime) values(?, ?, ?, ?, ?, ?)")
	if err != nil {
		common.WriteError(w, err)
		return
	}
	defer stmt.Close()

	_, err = stmt.Exec(username, password, homedir, 1, time.Now(), time.Now())
	if err != nil {
		common.WriteError(w, err)
		return
	}

	// FIXME: add init token for easy integration
	if err := initToken(tx, username); err != nil {
		common.WriteError(w, err)
		return
	}

	if err := tx.Commit(); err != nil {
		common.WriteError(w, err)
		return
	}
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	/*
		q := r.URL.Query()
		username := q.Get("username")
		device := q.Get("device")
			token, _, err := user.Login(h.basedir, assetdb, username, q.Get("password"), device, h.tokenDuration)
			if err != nil {
				common.WriteError(w, err)
				return
			}

			w.Write([]byte(token))
	*/
}
