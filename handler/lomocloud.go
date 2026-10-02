package handler

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"strconv"
	"time"

	"bitbucket.org/lomoware/lomo-backend/client"
	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/conf"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

/*
func (h *Handler) loadUsername(ctx context.Context, tx *sql.Tx) error {
	var err error
	h.lomocloudConf.username, err = conf.GetConfValue(ctx, tx, common.ConfCloudUsername)
	if err != nil {
		return err
	}
	h.lomocloudConf.password, err = conf.GetConfValue(ctx, tx, common.ConfCloudPassword)
	if err != nil {
		return err
	}
	h.lomocloudConf.SubDomain, err = conf.GetConfValue(ctx, tx, common.ConfCloudSubDomain)
	if err != nil {
		return err
	}
	if err != nil {
		return err
	}
	port, err := conf.GetConfValue(ctx, tx, common.ConfPortMapPublicPort)
	if err != nil {
		return err
	}

	h.lomocloudConf.publicPort, err = strconv.Atoi(port)
	return err
}
*/

func (h *Handler) register(cli *client.LomoCloud) error {
	u, err := cli.Register()
	if err != nil {
		logrus.Warnf("register cloud got %v", err)
		return err
	}
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		if err := conf.SetConfValue(ctx, tx, common.ConfCloudUsername, u.Name); err != nil {
			return err
		}
		if err := conf.SetConfValue(ctx, tx, common.ConfCloudPassword, u.Password); err != nil {
			return err
		}
		return conf.SetConfValue(ctx, tx, common.ConfCloudSubDomain, u.SubDomain)
	}); err != nil {
		logrus.Warnf("set cloud username/password conf got %v", err)
		return err
	}

	h.lomocloudConf.username = u.Name
	h.lomocloudConf.password = u.Password
	h.lomocloudConf.SubDomain = u.SubDomain
	return nil
}

func (h *Handler) loginCloud(cli *client.LomoCloud) error {
	if h.lomocloudConf.username == "" {
		if err := h.register(cli); err != nil {
			return err
		}
	}
	host, err := os.Hostname()
	if err != nil {
		host = "unknown"
		logrus.Warnf("while getting hostname got %v", err)
	}
	resp, err := cli.Login(h.lomocloudConf.username, h.lomocloudConf.password, host)
	if err != nil {
		return err
	}
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		return conf.SetConfValue(ctx, tx, common.ConfCloudToken, resp.Token)
	}); err != nil {
		return err
	}
	h.lomocloudConf.token = resp.Token
	return nil
}

func (h *Handler) setupCloudPortMappingIn(w http.ResponseWriter, r *http.Request) {
	port := mux.Vars(r)["eport"]
	out, err := strconv.Atoi(port)
	if err != nil {
		common.WriteError(w, err)
		return
	}
	in := h.conf.ListenPort
	if out == 443 {
		in = h.conf.ListenPortHTTPS
	}
	if err := h.setupCloudPortMapping(in, out); err != nil {
		common.WriteError(w, err)
		return
	}

	h.lomocloudConf.publicPort = out
	h.lomocloudConf.localPort = in
	common.WriteBody(w, &h.lomocloudConf)
}

func (h *Handler) setupCloudPortMappingAuto(w http.ResponseWriter, r *http.Request) {
	port, err := h.probeNatPortMapping(h.conf.ListenPort)
	if err != nil {
		common.WriteError(w, err)
		return
	}

	if err := h.setupCloudPortMapping(h.conf.ListenPort, port); err != nil {
		common.WriteError(w, err)
		return
	}

	h.lomocloudConf.publicPort = port
	h.lomocloudConf.localPort = h.conf.ListenPort
	common.WriteBody(w, &h.lomocloudConf)
}

func (h *Handler) setupCloudPortMapping(in, out int) error {
	cli := client.NewLomoCloud(h.lomocloudConf.host)
	if h.lomocloudConf.token == "" {
		if err := h.loginCloud(cli); err != nil {
			return err
		}
	}
	if _, err := cli.CreatePortMap(h.lomocloudConf.token, "", in, out); err != nil {
		return err
	}
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		if err := conf.SetConfValue(ctx, tx, common.ConfPortMapLocalPort, strconv.Itoa(in)); err != nil {
			return err
		}
		return conf.SetConfValue(ctx, tx, common.ConfPortMapPublicPort, strconv.Itoa(out))
	}); err != nil {
		logrus.Warnf("set cloud port mapping got %v", err)
		return err
	}

	if in == h.conf.ListenPortHTTPS {
		go h.StartHTTPSListener()
	}

	return nil
}

func (h *Handler) removeCloudAccount(w http.ResponseWriter, r *http.Request) {
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		if err := conf.RemoveConfValue(ctx, tx, common.ConfCloudUsername); err != nil {
			return err
		}
		if err := conf.RemoveConfValue(ctx, tx, common.ConfCloudPassword); err != nil {
			return err
		}
		if err := conf.RemoveConfValue(ctx, tx, common.ConfPortMapPublicIP); err != nil {
			return err
		}
		if err := conf.RemoveConfValue(ctx, tx, common.ConfPortMapPublicPort); err != nil {
			return err
		}
		return conf.RemoveConfValue(ctx, tx, common.ConfCloudToken)
	}); err != nil {
		common.WriteError(w, err)
		return
	}
}

/* TODO: no port mapping for now
func (h *Handler) getPortMapping(ctx context.Context, tx *sql.Tx) (int, error) {
	pp, err := conf.GetConfValue(ctx, tx, common.ConfPortMapPublicPort)
	if err != nil {
		if common.IsErrNoRows(err) {
			// not set yet, return
			return 0, nil
		}
		return 0, err
	}
	return strconv.Atoi(pp)
}
*/

func (h *Handler) refreshPortMapping(ctx context.Context) {
	if h.lomocloudConf.host == "" {
		return
	}

	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		port, err := conf.GetConfValue(ctx, tx, common.ConfPortMapPublicPort)
		if err != nil {
			if !common.IsErrNoRows(err) {
				logrus.Warnf("read cloud public port got %v", err)
			}
			return nil
		}
		h.lomocloudConf.publicPort, err = strconv.Atoi(port)
		if err != nil {
			if !common.IsErrNoRows(err) {
				logrus.Warnf("convert public port got %v", err)
			}
			return nil
		}

		port, err = conf.GetConfValue(ctx, tx, common.ConfPortMapLocalPort)
		if err != nil {
			if !common.IsErrNoRows(err) {
				logrus.Warnf("read cloud local port got %v", err)
			}
			return nil
		}
		h.lomocloudConf.localPort, err = strconv.Atoi(port)
		if err != nil {
			if !common.IsErrNoRows(err) {
				logrus.Warnf("convert local port got %v", err)
			}
			return nil
		}

		h.lomocloudConf.token, err = conf.GetConfValue(ctx, tx, common.ConfCloudToken)
		if err != nil && !common.IsErrNoRows(err) {
			logrus.Warnf("read cloud token got %v", err)
		}
		return nil
	}); err != nil {
		logrus.Warnf("read port mapping conf got %v", err)
	}

	if h.lomocloudConf.localPort != 0 {
		h.probeNatPortMapping(h.lomocloudConf.localPort)
	}

	cli := client.NewLomoCloud(h.lomocloudConf.host)
	for {
		after := time.After(5 * time.Minute)
		select {
		case <-ctx.Done():
			logrus.Infof("context is done. %v", ctx.Err())
			return
		case <-after:
		}

		if h.lomocloudConf.token == "" {
			// not set port mapping yet
			continue
		}

		// checkin to update cloud with new ip address
		_, err := cli.UpdatePortMap(h.lomocloudConf.token, "", h.lomocloudConf.localPort, h.lomocloudConf.publicPort)
		if err != nil {
			logrus.Warnf("update cloud port mapping got %v", err)
		}
	}
}
