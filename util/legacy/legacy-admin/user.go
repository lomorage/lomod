package main

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"

	"github.com/gorilla/mux"

	_ "github.com/mattn/go-sqlite3"
)

// Users is response structure for listusers request
type Users struct {
	Name []string
}

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	username := q.Get("username")

	db, err := h.validate(username, q.Get("token"))
	if err != nil {
		common.WriteError(w, err)
		return
	}
	defer func() {
		if db != nil {
			db.Close()
		}
	}()

	stmt, err := db.Prepare("select userid from user")
	if err != nil {
		common.WriteError(w, err)
		return
	}
	defer stmt.Close()

	rows, err := stmt.Query()
	if err != nil {
		common.WriteError(w, err)
		return
	}
	defer rows.Close()

	users := &Users{Name: []string{}}
	for rows.Next() {
		var name string
		err = rows.Scan(&name)
		if err != nil {
			common.WriteError(w, err)
			return
		}
		users.Name = append(users.Name, name)
	}
	if err := rows.Err(); err != nil {
		common.WriteError(w, err)
		return
	}

	common.WriteBody(w, users)
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	userID := mux.Vars(r)["userID"]
	deviceID := mux.Vars(r)["deviceID"]
	password := mux.Vars(r)["password"]

	db, err := h.validate(q.Get("username"), q.Get("token"))
	defer func() {
		if db != nil {
			db.Close()
		}
	}()
	if err != nil {
		common.WriteError(w, err)
		return
	}

	// get device mount path
	devicename := fmt.Sprintf("/dev/%s", deviceID)
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
	homedir := path.Join(device.Path, userID)
	if err := os.MkdirAll(homedir, 0600); err != nil {
		common.WriteError(w, err)
	}

	// insert new group
	tx, err := db.Begin()
	if err != nil {
		common.WriteError(w, err)
		return
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("insert into user(userid, password, homedir, createtime, lastmodifiedtime) values(?, ?, ?, ?, ?)")
	if err != nil {
		common.WriteError(w, err)
		return
	}
	defer stmt.Close()

	_, err = stmt.Exec(userID, password, homedir, time.Now(), time.Now())
	if err != nil {
		common.WriteError(w, err)
		return
	}

	if err := initToken(tx, userID); err != nil {
		common.WriteError(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		common.WriteError(w, err)
		return
	}
}

func (h *Handler) deleteUser(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	username := q.Get("username")
	userID := mux.Vars(r)["userID"]

	db, err := h.validate(username, q.Get("token"))
	defer func() {
		if db != nil {
			db.Close()
		}
	}()
	if err != nil {
		common.WriteError(w, err)
		return
	}

	// delete new group
	tx, err := db.Begin()
	if err != nil {
		common.WriteError(w, err)
		return
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("delete from user where userid = ?")
	if err != nil {
		common.WriteError(w, err)
		return
	}
	defer stmt.Close()

	_, err = stmt.Exec(userID)
	if err != nil {
		common.WriteError(w, err)
		return
	}

	if err := tx.Commit(); err != nil {
		common.WriteError(w, err)
		return
	}
}
