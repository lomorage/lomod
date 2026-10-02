package handler

/*
comment out to prevent go check
import (
	"context"
	"database/sql"

	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"github.com/localtunnel/go-localtunnel"
	"github.com/sirupsen/logrus"
	"bitbucket.org/lomoware/lomo-backend/common/conf"
	"bitbucket.org/lomoware/lomo-backend/common"
	"strings"
	"fmt"
	"net/url"
)

type logWrapper struct {
}

func (l logWrapper) Println(v ...interface{}) {
	logrus.Info(v...)
}

func (h *Handler) startTunnel(port int) error {
	subDomain := ""
	err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		subDomain, err = conf.GetConfValue(ctx, tx, common.ConfLocalTunnelSubDomain)
		return err
	})
	if err != nil {
		logrus.Warnf("query local tunnel sub-domain got error: %v", err)
	}

	lt, err := localtunnel.New(port, "",
		localtunnel.Options{
			Subdomain:      subDomain,
			Log:            logWrapper{},
			MaxConnections: 1,
			BaseURL:        "",
		})
	if err != nil {
		return err
	}

	h.tunnel = lt

	logrus.Infof("local tunnel is listening at %s", lt.URL())

	if subDomain == "" {
		u, err := url.Parse(lt.URL())
		if err != nil {
			logrus.Warnf("parse local tunnel url got error: %v", err)
			goto sleep
		}
		parts := strings.Split(strings.TrimSpace(u.Host), ".")
		if len(parts) > 2 {
			//The subdomain exists, we store it as the first element
			//in a new array
			subDomain = strings.Join(parts[0: len(parts) - 2], ".")
		}
		fmt.Println(subDomain)
		err = dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
			return conf.SetConfValue(ctx, tx, common.ConfLocalTunnelSubDomain, subDomain)
		})
		if err != nil {
			logrus.Warnf("set local tunnel sub-domain got error: %v", err)
		}
	}

sleep:
	for {
		select {
		case <-h.gCtx.Done():
			break
		}
	}

	logrus.Infof("closing local tunnel")

	return lt.Close()
}
*/
