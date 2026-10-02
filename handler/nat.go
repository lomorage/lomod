package handler

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/conf"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	nat "github.com/leslie-wang/go-nat"
	"github.com/sirupsen/logrus"
)

func (h *Handler) probeNatPortMapping(internalPort int) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	nat, err := nat.DiscoverGateway(ctx)
	if err != nil {
		return 0, err
	}
	logrus.Infof("nat type: %s", nat.Type())

	daddr, err := nat.GetDeviceAddress()
	if err != nil {
		return 0, err
	}
	logrus.Infof("device address: %s", daddr)

	iaddr, err := nat.GetInternalAddress()
	if err != nil {
		return 0, err
	}
	logrus.Infof("internal address: %s", iaddr)

	eaddr, err := nat.GetExternalAddress()
	if err != nil {
		return 0, err
	}
	logrus.Infof("external address: %s", eaddr)

	go h.refreshNatPortMapping(nat)

	h.lomocloudConf.publicIP = eaddr.String()
	return nat.AddPortMapping("tcp", internalPort, "http", 60*time.Second)
}

func (h *Handler) refreshNatPortMapping(nat nat.NAT) {
	for {
		time.Sleep(30 * time.Second)

		publicPort, err := nat.AddPortMapping("tcp", h.lomocloudConf.localPort, "http", 60*time.Second)
		if err != nil {
			logrus.Warnf("add port mapping: %v", err)
			continue
		}
		if publicPort == h.lomocloudConf.publicPort {
			// port is not changed, continue
			continue
		}

		h.lomocloudConf.publicPort = publicPort
		if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
			return conf.SetConfValue(ctx, tx, common.ConfPortMapPublicPort, strconv.Itoa(publicPort))
		}); err != nil {
			logrus.Warnf("set cloud port mapping got %v", err)
		}
	}
}
