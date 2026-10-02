package lomocloud

import (
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/logger"
	lnet "bitbucket.org/lomoware/lomo-backend/common/net"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	username, password, device, err := user.GetUserLogin(r)
	if err != nil {
		q := r.URL.Query()
		username = q.Get("username")
		password = q.Get("password")
		device = q.Get("device")
	}

	var (
		token  string
		userid int
	)
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		token, userid, err = user.Login(ctx, tx, username, password, device, h.tokenDuration)
		return err
	}); err != nil {
		common.WriteError(w, err)
		return
	}
	common.WriteBody(w, user.LoginResp{Token: token, Userid: userid})
}

func (h *Handler) createAccount(w http.ResponseWriter, r *http.Request) {
	u := &user.User{}
	// create one username for the account
	u.Password = common.RandomString(8)
	err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		for {
			u.Name = common.RandomString(8)
			_, err := user.GetUserID(ctx, tx, u.Name)
			if err == nil {
				// duplicate, and retry
				continue
			} else if err != common.ErrNotExistUser {
				return err
			}
			break
		}
		stmt, err := tx.Prepare(`
insert into user(user_name, password, phone, email, nick_name, status, create_time, last_modified_time, 
last_login_time) values(?, ?, ?, ?, ?, ?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		defer stmt.Close()

		_, err = stmt.ExecContext(ctx, u.Name, u.Password, u.Password, u.Email, u.NickName, 0,
			time.Now(), time.Now(), time.Now())
		return err
	})
	if err != nil {
		common.WriteError(w, err)
		return
	}
	u.SubDomain = u.Name + "." + h.domainName
	common.WriteBody(w, u)
}

func (h *Handler) getAccountIPMap(w http.ResponseWriter, r *http.Request) {
	pip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		logrus.Errorf("userip: %v is not IP:port", r.RemoteAddr)
		common.WriteError(w, err)
		return
	}
	publicIP := net.ParseIP(pip)
	if publicIP == nil {
		err = errors.Errorf("unable to parse remote address: %s", r.RemoteAddr)
		logrus.Error(err)
		common.WriteError(w, err)
		return
	}
	reply, err := h.handleGetIPMap(publicIP)
	if err != nil {
		common.WriteError(w, err)
		return
	}
	common.WriteBody(w, &reply)
}

func (h *Handler) handleGetIPMap(publicIP net.IP) ([]*types.IPMapReply, error) {
	reply := []*types.IPMapReply{}
	return reply, dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		stmt, err := tx.Prepare("select private_ip, mac_address, name, uuid, port from ip_map where public_ip = ? and last_modified_time >= ?")
		if err != nil {
			return err
		}
		defer stmt.Close()

		rows, err := stmt.QueryContext(ctx, publicIP.String(), time.Now().Add(-1*h.conf.DeadTimeout))
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			pip := ""
			mac := ""
			r := &types.IPMapReply{}
			err := rows.Scan(&pip, &mac, &r.Name, &r.UUID, &r.Port)
			if err != nil {
				return err
			}
			r.PrivateIP = net.ParseIP(pip)
			if r.PrivateIP == nil {
				logrus.Errorf("private IP in DB is wrong: %s", pip)
				continue
			}
			r.MAC, err = net.ParseMAC(mac)
			if err != nil {
				logrus.Errorf("mac in DB (%s) is wrong: %s", mac, err)
				continue
			}
			reply = append(reply, r)
		}
		return rows.Err()
	})
}

func (h *Handler) createAccountIPMap(w http.ResponseWriter, r *http.Request) {
	pip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		logrus.Errorf("userip: %v is not IP:port", r.RemoteAddr)
		common.WriteError(w, err)
		return
	}
	publicIP := net.ParseIP(pip)
	if publicIP == nil {
		err = errors.Errorf("unable to parse remote address: %s", r.RemoteAddr)
		logrus.Error(err)
		common.WriteError(w, err)
		return
	}
	request := []*types.IPMapRequest{}
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		logrus.Errorf("Decode ip mapping request: %v", err)
		common.WriteError(w, err)
		return
	}

	for _, req := range request {
		if err := req.Validate(); err != nil {
			common.WriteError(w, err)
			return
		}
	}

	if err := h.handleCreateOrUpdateIPMapRequest(publicIP.String(), true, request); err != nil {
		common.WriteError(w, err)
	}
}

func (h *Handler) handleCreateOrUpdateIPMapRequest(publicIP string, skipErr bool,
	request []*types.IPMapRequest) error {
	return dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		for _, req := range request {
			if err := h.handleCreateOrUpdateIPMap(ctx, tx, publicIP, req); err != nil {
				logrus.Warnf("handle IP map (%s, %v): %s", publicIP, req, err)
				if !skipErr {
					return err
				}
			}
		}
		return nil
	})
}

func (h *Handler) handleCreateOrUpdateIPMap(ctx context.Context, tx *sql.Tx, publicIP string,
	r *types.IPMapRequest) error {
	// if same ip/port was bind to different ip before, replace with new ip
	var (
		oldPublicIP  string
		oldPrivateIP string
		oldMap       types.IPMapRequest
	)
	if err := tx.QueryRowContext(ctx, "select public_ip, private_ip, name, uuid, port, os, arch, lomod_ver from ip_map where mac_address = ?",
		r.MAC.String()).Scan(&oldPublicIP, &oldPrivateIP, &oldMap.Name, &oldMap.UUID, &oldMap.Port, &oldMap.OS, &oldMap.Arch, &oldMap.LomodVersion); err != nil {
		if !common.IsErrNoRows(err) {
			return err
		}
		logrus.Infof("inserting new ip map (%s, %s, %s, %s, %s, %d)", r.MAC, publicIP, r.PrivateIP, r.Name, r.UUID, r.Port)
		_, err = tx.ExecContext(ctx,
			`insert into ip_map(mac_address, public_ip, private_ip, name, uuid, port, os, arch, lomod_ver, create_time, last_modified_time) values(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			r.MAC.String(), publicIP, r.PrivateIP.String(), r.Name, r.UUID, r.Port, r.OS, r.Arch, r.LomodVersion, time.Now(), time.Now())
		return err
	}
	updateArgs := []string{"last_modified_time=?"}
	updateParams := []interface{}{time.Now()}
	changes := []string{}
	if oldPublicIP != publicIP {
		updateArgs = append(updateArgs, "public_ip=?")
		updateParams = append(updateParams, publicIP)
		changes = append(changes, oldPublicIP+"->"+publicIP)
	}
	if oldPrivateIP != r.PrivateIP.String() {
		updateArgs = append(updateArgs, "private_ip=?")
		updateParams = append(updateParams, r.PrivateIP.String())
		changes = append(changes, oldPrivateIP+"->"+r.PrivateIP.String())
	}
	if oldMap.UUID != r.UUID {
		updateArgs = append(updateArgs, "uuid=?")
		updateParams = append(updateParams, r.UUID)
		changes = append(changes, oldMap.UUID+"->"+r.UUID)
	}
	if oldMap.Name != r.Name {
		updateArgs = append(updateArgs, "name=?")
		updateParams = append(updateParams, r.Name)
		changes = append(changes, oldMap.Name+"->"+r.Name)
	}
	if oldMap.Port != r.Port {
		updateArgs = append(updateArgs, "port=?")
		updateParams = append(updateParams, r.Port)
		changes = append(changes, strconv.Itoa(oldMap.Port)+"->"+
			strconv.Itoa(r.Port))
	}
	if oldMap.OS != r.OS {
		updateArgs = append(updateArgs, "os=?")
		updateParams = append(updateParams, r.OS)
		changes = append(changes, oldMap.OS+"->"+r.OS)
	}
	if oldMap.Arch != r.Arch {
		updateArgs = append(updateArgs, "arch=?")
		updateParams = append(updateParams, r.Arch)
		changes = append(changes, oldMap.Arch+"->"+r.Arch)
	}
	if oldMap.LomodVersion != r.LomodVersion {
		updateArgs = append(updateArgs, "lomod_ver=?")
		updateParams = append(updateParams, r.LomodVersion)
		changes = append(changes, oldMap.LomodVersion+"->"+r.LomodVersion)
	}

	logrus.Infof("update %s: %v", r.MAC, changes)

	_, err := tx.ExecContext(ctx,
		"update ip_map set "+strings.Join(updateArgs, ",")+" where mac_address = ?",
		append(updateParams, r.MAC.String())...)
	return err
}

func (h *Handler) createAccountPortMap(w http.ResponseWriter, r *http.Request) {
	reply, err := h.handlePortMap(w, r, true)
	if err != nil {
		common.WriteError(w, err)
		return
	}
	common.WriteBody(w, &reply)
}

func (h *Handler) updateAccountPortMap(w http.ResponseWriter, r *http.Request) {
	reply, err := h.handlePortMap(w, r, false)
	if err != nil {
		common.WriteError(w, err)
		return
	}
	common.WriteBody(w, reply)
}

func (h *Handler) handlePortMap(w http.ResponseWriter, r *http.Request, create bool) (*types.PortMapReply, error) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		return nil, wl.Err
	}

	vars := mux.Vars(r)
	in, err := strconv.Atoi(vars["in"])
	if err != nil {
		return nil, err
	}
	out, err := strconv.Atoi(vars["out"])
	if err != nil {
		return nil, err
	}

	reply := &types.PortMapReply{}
	reply.ClientIP, _, err = net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		logrus.Errorf("userip: %v is not IP:port", r.RemoteAddr)
	}
	ip := r.URL.Query().Get("ip")
	if ip == "" {
		if reply.ClientIP == "127.0.0.1" {
			ip, err = lnet.GetPublicIP()
			if err != nil {
				return nil, err
			}
		} else {
			ip = reply.ClientIP
		}
	}
	if net.ParseIP(ip) == nil {
		return nil, errors.Errorf("userip: %v is not IP:port", r.RemoteAddr)
	}

	if create {
		return reply, dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
			stmt, err := tx.Prepare(`
insert into port_map(user_id, subdomain, port_in, port_out, ip, create_time, last_modified_time) 
values(?, ?, ?, ?, ?, ?, ?)`)
			if err != nil {
				return err
			}
			defer stmt.Close()

			_, err = stmt.ExecContext(ctx, wl.Userid, wl.Username, in, out, ip, time.Now(), time.Now())
			return err
		})
	}
	// if same ip/port was bind to different ip before, replace with new ip
	return reply, dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		oldIP := ""
		if err := tx.QueryRowContext(ctx, "select ip from port_map where user_id = ? and port_in = ? and port_out = ?",
			wl.Userid, in, out).Scan(&oldIP); err != nil {
			return err
		}
		if oldIP == ip {
			return nil
		}

		logrus.Infof("%d's %d:%d has changed from %s to %s", wl.Userid, in, out, oldIP, ip)

		_, err = tx.ExecContext(ctx,
			"update port_map set ip = ?, last_modified_time = ? where user_id = ? and port_in = ? and port_out = ?",
			ip, time.Now(), wl.Userid, in, out)
		return err
	})
}
