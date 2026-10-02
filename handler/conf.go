package handler

import (
	"context"
	"database/sql"
	"strconv"

	"github.com/sirupsen/logrus"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/conf"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/google/uuid"
)

func (h *Handler) loadUUID(ctx context.Context, tx *sql.Tx) (err error) {
	h.uuid, err = conf.GetConfValue(ctx, tx, common.ConfUUID)
	if err == nil {
		return
	}
	if !common.IsErrNoRows(err) {
		return
	}
	h.uuid = uuid.New().String()
	return conf.SetConfValue(ctx, tx, common.ConfUUID, h.uuid)
}

func (h *Handler) loadConf() error {
	return dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		useCloudIP, err := conf.GetConfValue(ctx, tx, common.ConfCloudIPHelper)
		if err == nil && useCloudIP == "0" {
			h.useCloudIPHelper = false
		} else if h.conf.LomodCloudDomain != "" {
			// enable cloudIPHelper if cloud domain is configured
			h.useCloudIPHelper = true
		}
		layout, err := conf.GetConfValue(ctx, tx, common.ConfWebdavDirLayout)
		if err == nil {
			h.webdavDirLayout, err = strconv.Atoi(layout)
			if err != nil {
				logrus.Warnf("invalid webdav directory layout: %s", layout)
			}
		}
		if err := user.LoadJWTSecret(ctx, tx); err != nil {
			return err
		}
		return h.loadUUID(ctx, tx)
	})

}
