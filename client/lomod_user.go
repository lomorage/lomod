package client

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"

	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/pkg/errors"
)

// Login logins the lomo backend
func (ld *Lomod) Login(username, password string) (*user.LoginResp, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	headers := http.Header{
		"Authorization": []string{"Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password+":"+hostname))},
	}
	resp, err := requestReply(http.MethodGet, ld.host+"/login", headers, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusInternalServerError {
		return nil, makeStatusError(resp.Body)
	}
	err = json.NewDecoder(resp.Body).Decode(&ld.login)
	return &ld.login, err
}

// ValidateToken validate token
func (ld *Lomod) ValidateToken() error {
	resp, err := ld.requestReply(http.MethodHead, "/user/token", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}
	return errors.New("invalid token")
}

// AddUser adds new user
func (ld *Lomod) AddUser(u *user.User) error {
	resp, err := ld.requestReply(http.MethodPost, "/user", u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return makeStatusError(resp.Body)
	}
	return nil
}

// DeleteUser deletes user
func (ld *Lomod) DeleteUser(username string) error {
	resp, err := ld.requestReply(http.MethodDelete, "/user/"+username, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return checkAndReturnReply(resp)
}
