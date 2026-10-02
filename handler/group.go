package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/group"
	"bitbucket.org/lomoware/lomo-backend/common/logger"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

func (h *Handler) listGroup(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}
	var groups *group.Groups
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		groups, err = group.ListGroup(ctx, tx, wl.Userid)
		return err
	}); err != nil {
		common.WriteError(w, err)
		return
	}

	common.WriteBody(w, groups)
}

func (h *Handler) createGroupByJSON(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	defer r.Body.Close()
	g := &group.Group{}
	if err := json.NewDecoder(r.Body).Decode(g); err != nil {
		common.WriteError(w, err)
		return
	}

	var reply *group.Group
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		if g.OwnerID == 0 {
			g.OwnerID = wl.Userid
		} else if g.OwnerID != wl.Userid {
			return common.ErrDifferentUsername
		}
		var err error
		reply, err = group.CreateGroup(ctx, tx, g.Name, g.OwnerID, g.Members)
		return err
	}); err != nil {
		common.WriteError(w, err)
		return
	}

	common.WriteBody(w, reply)
}

func (h *Handler) createGroup(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	g := &group.Group{}
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		g, err = group.CreateGroup(ctx, tx, mux.Vars(r)["groupID"], wl.Userid, nil)
		return err
	}); err != nil {
		common.WriteError(w, err)
		return
	}

	common.WriteBody(w, g)
}

func (h *Handler) listGroupMembers(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	groupID, err := strconv.Atoi(mux.Vars(r)["groupID"])
	if err != nil {
		logrus.Warnf("invalid groupID: %s\n", mux.Vars(r)["groupID"])
		common.WriteError(w, common.ErrBadRequest)
		return
	}

	var members []*user.User
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		members, err = group.ListGroupMembers(ctx, tx, groupID)
		return err
	}); err != nil {
		if common.IsErrNoRows(err) {
			common.WriteError(w, common.ErrBadRequest)
		} else {
			common.WriteError(w, err)
		}
		return
	}

	common.WriteBody(w, members)
}

func (h *Handler) deleteMemberFromGroup(w http.ResponseWriter, r *http.Request) {
	groupID, err := strconv.Atoi(mux.Vars(r)["groupID"])
	if err != nil {
		common.WriteError(w, err)
		return
	}
	memberID, err := strconv.Atoi(mux.Vars(r)["userID"])
	if err != nil {
		common.WriteError(w, err)
		return
	}
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		return group.DeleteMember(ctx, tx, groupID, wl.Userid, memberID)
	}); err != nil {
		if common.IsErrNoRows(err) {
			common.WriteError(w, common.ErrBadRequest)
		} else {
			common.WriteError(w, err)
		}
		return
	}
}

func (h *Handler) addMemberInGroup(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}
	groupID, err := strconv.Atoi(mux.Vars(r)["groupID"])
	if err != nil {
		common.WriteError(w, err)
		return
	}
	memberID, err := strconv.Atoi(mux.Vars(r)["userID"])
	if err != nil {
		common.WriteError(w, err)
		return
	}
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		return group.AddMember(ctx, tx, groupID, wl.Userid, memberID)
	}); err != nil {
		if common.IsErrNoRows(err) {
			common.WriteError(w, common.ErrBadRequest)
		} else {
			common.WriteError(w, err)
		}
		return
	}
}
