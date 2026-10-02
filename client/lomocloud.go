package client

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/sirupsen/logrus"
)

// LomoCloud is the client interface to access lomo cloud service
type LomoCloud struct {
	host string
	port int
}

// NewLomoCloud returns lomo cloud client
func NewLomoCloud(host string) *LomoCloud {
	return &LomoCloud{
		host: host,
		port: common.LomoCloudPort,
	}
}

// Register register new account
func (lc *LomoCloud) Register() (*user.User, error) {
	body, err := request(http.MethodPost, fmt.Sprintf("%s:%d/account", lc.host, lc.port), nil, nil)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	resp := &user.User{}

	err = json.NewDecoder(body).Decode(resp)
	return resp, err
}

// Login send login request to cloud, and return reply
func (lc *LomoCloud) Login(username, password, device string) (*user.LoginResp, error) {
	header := http.Header{
		"Authorization": []string{common.BasicAuthPrefix + base64.StdEncoding.EncodeToString([]byte(username+":"+password+":"+device))},
	}

	body, err := request(http.MethodPost, fmt.Sprintf("%s:%d/login", lc.host, lc.port), header, nil)
	if err != nil {
		return nil, err
	}

	defer body.Close()
	resp := &user.LoginResp{}

	err = json.NewDecoder(body).Decode(resp)
	return resp, err
}

// GetIPMap gets ip map from cloud
func (lc *LomoCloud) GetIPMap() ([]types.IPMapReply, error) {
	u := fmt.Sprintf("%s:%d/account/ip_map", lc.host, lc.port)
	resp, err := request(http.MethodGet, u, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Close()

	reply := []types.IPMapReply{}
	if err := json.NewDecoder(resp).Decode(&reply); err != nil {
		return nil, err
	}
	return reply, nil
}

// SetIPMap set ip map for list of <mac, public_ip, private_ip>.
func (lc *LomoCloud) SetIPMap(list []types.IPMapRequest) error {
	for _, r := range list {
		if err := r.Validate(); err != nil {
			return err
		}
	}
	u := fmt.Sprintf("%s:%d/account/ip_map", lc.host, lc.port)
	content, err := json.Marshal(list)
	if err != nil {
		return err
	}
	_, err = request(http.MethodPost, u, nil, bytes.NewBuffer(content))
	return err
}

// UpdatePortMap updates port mapping for given account
func (lc *LomoCloud) UpdatePortMap(token, ip string, in, out int) (string, error) {
	return lc.setPortMap(token, ip, in, out, false)
}

// CreatePortMap creates port mapping for given account
func (lc *LomoCloud) CreatePortMap(token, ip string, in, out int) (string, error) {
	return lc.setPortMap(token, ip, in, out, true)
}

func (lc *LomoCloud) setPortMap(token, ip string, in, out int, create bool) (string, error) {
	if ip == "" && in == 0 {
		logrus.Info("public ip / port is not set")
		return "", nil
	}
	m := http.MethodPut
	if create {
		m = http.MethodPost
	}
	u := fmt.Sprintf("%s:%d/account/portmap/%d/%d", lc.host, lc.port, in, out)
	if ip != "" {
		u = u + "?ip=" + ip
	}

	resp, err := request(m, u, getHeaders(token), nil)
	if err != nil {
		return "", err
	}
	defer resp.Close()

	reply := &types.PortMapReply{}
	if err := json.NewDecoder(resp).Decode(reply); err != nil {
		return "", err
	}
	return reply.ClientIP, nil
}
