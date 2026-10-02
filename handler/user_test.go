package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/client"
	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/album"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/pkg/errors"

	. "gopkg.in/check.v1"
)

func (ts *mainSuite) createUser(u user.User, adminToken string, expectOK bool) error {
	content, err := json.Marshal(&u)
	if err != nil {
		return err
	}

	url := "/user"
	if adminToken != "" {
		url = fmt.Sprintf("%s?token=%s", url, adminToken)
	}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(content))
	if err != nil {
		return err
	}

	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)

	// Check the status code is what we expect.
	res := rr.Result()
	defer res.Body.Close()
	if expectOK {
		if res.StatusCode != http.StatusOK {
			txt, _ := ioutil.ReadAll(res.Body)
			return errors.Errorf("got non-200 status code on user create: %s - %v", string(txt), res)
		}
		return nil
	}

	errReply := common.ErrResponse{}
	if err := json.NewDecoder(res.Body).Decode(&errReply); err != nil {
		return err
	}
	return errors.New(errReply.Text)
}

func (ts *mainSuite) login(username, password string) (string, error) {
	// login and get token
	url := fmt.Sprintf("/login?username=%s&password=%s&device=iphonex", username, password)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)

	res := rr.Result()
	if res.StatusCode != http.StatusOK {
		return "", errors.New("got non-200 status code on login")
	}

	defer res.Body.Close()
	token := user.LoginResp{}
	err = json.NewDecoder(res.Body).Decode(&token)

	return token.Token, err
}

func (ts *mainSuite) TestUserDuplicate(c *C) {
	u := user.User{
		Name:     "charlie",
		Password: "charlie123",
		Phone:    "charlie_phone",
		Email:    "charlie_email",
		NickName: "charlie_nick",
		HomeDir:  photodir + "/charlie",
	}
	err := ts.createUser(u, ts.token, true)
	c.Assert(err, IsNil)

	// duplicate should fail
	err = ts.createUser(u, ts.token, true)
	c.Assert(err, NotNil)

	// capital username should be successful
	u.Name = "Charlie"
	u.HomeDir = photodir + "/" + u.Name
	err = ts.createUser(u, ts.token, true)
	c.Assert(err, IsNil)
}

func (ts *mainSuite) listUsers(c *C, token string) []user.User {
	url := fmt.Sprintf("/user?token=%s", token)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	c.Assert(err, IsNil)

	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	validateStatusCode(c, url, rr, http.StatusOK)

	res := rr.Result()
	defer res.Body.Close()

	result := &user.Users{}
	json.NewDecoder(res.Body).Decode(result)

	users := []user.User{}
	for _, u := range result.Users {
		users = append(users, *u)
	}

	return users
}

func (ts *mainSuite) TestUserUpdate(c *C) {
	u := user.User{
		Name:  ts.alice.Name,
		Phone: "111",
	}
	url := fmt.Sprintf("/user?token=%s", ts.token)
	content, err := json.Marshal(&u)
	c.Assert(err, IsNil)
	ts.requestWithMethodBody(c, url, http.MethodPut, http.StatusOK, bytes.NewBuffer(content), nil, nil, nil)

	result := ts.listUsers(c, ts.token)
	c.Assert(len(result), Equals, 2)
	c.Assert(result[0].Name, Equals, ts.alice.Name)
	c.Assert(result[0].Phone, Equals, "111")
	c.Assert(result[0].Email, Equals, ts.alice.Email)
	c.Assert(result[0].Status, Equals, common.UserStatusUnknown)
}

func (ts *mainSuite) TestUserUpdatePassword(c *C) {
	u := user.User{
		Name:     ts.alice.Name,
		Password: "111",
	}
	url := fmt.Sprintf("/user?token=%s", ts.token)
	content, err := json.Marshal(&u)
	c.Assert(err, IsNil)
	ts.requestWithMethodBody(c, url, http.MethodPut, http.StatusOK, bytes.NewBuffer(content), nil, nil, nil)

	_, err = ts.login(ts.alice.Name, ts.alice.Password)
	c.Assert(err, NotNil)

	token, err := ts.login(ts.alice.Name, "111")
	c.Assert(err, IsNil)
	c.Assert(token, Not(Equals), "")
}

func (ts *mainSuite) TestUserUpdateAdminToken(c *C) {
	u := user.User{
		Name:  ts.alice.Name,
		Phone: "111",
	}
	url := fmt.Sprintf("/user?token=%s", adminToken)
	ts.h.accessLogger.AdminToken = adminToken
	content, err := json.Marshal(&u)
	c.Assert(err, IsNil)
	ts.requestWithMethodBody(c, url, http.MethodPut, http.StatusOK, bytes.NewBuffer(content), nil, nil, nil)

	// admin token should be able to update password too
	result := ts.listUsers(c, adminToken)
	c.Assert(len(result), Equals, 2)
	c.Assert(result[0].Name, Equals, ts.alice.Name)
	c.Assert(result[0].Phone, Equals, "111")
	c.Assert(result[0].Email, Equals, ts.alice.Email)
}

func (ts *mainSuite) TestUserUpdatePasswordAdminToken(c *C) {
	_, err := ts.login(ts.alice.Name, ts.alice.Password)
	c.Assert(err, IsNil)

	u := user.User{
		Name:     ts.alice.Name,
		Password: "111",
	}
	ts.h.accessLogger.AdminToken = adminToken
	url := fmt.Sprintf("/user?token=%s", adminToken)
	content, err := json.Marshal(&u)
	c.Assert(err, IsNil)
	ts.requestWithMethodBody(c, url, http.MethodPut, http.StatusOK, bytes.NewBuffer(content), nil, nil, nil)

	_, err = ts.login(ts.alice.Name, ts.alice.Password)
	c.Assert(err, NotNil)

	token, err := ts.login(ts.alice.Name, "111")
	c.Assert(err, IsNil)
	c.Assert(token, Not(Equals), "")
}

func (ts *mainSuite) TestUserKeepalive(c *C) {
	pingInterval := time.Second

	var cancel context.CancelFunc
	ts.h.gCtx, cancel = context.WithCancel(context.Background())
	defer cancel()

	t := httptest.NewServer(ts.h.CreateRouter())
	defer t.Close()

	cli := client.NewLomodWithToken(t.URL, ts.token)
	ctx, cancel2 := context.WithCancel(context.Background())
	go func() {
		c.Assert(cli.Keepalive(ctx, pingInterval, 443), Equals, context.Canceled)
	}()

	time.Sleep(2 * pingInterval)

	result := ts.listUsers(c, ts.token)
	c.Assert(len(result), Equals, 2)
	c.Assert(result[0].Name, Equals, ts.alice.Name)
	c.Assert(result[0].Status, Equals, common.UserStatusOnline)

	cancel2()

	// sleep ping interval and shoule be offline now
	time.Sleep(2 * pingInterval)

	result = ts.listUsers(c, ts.token)
	c.Assert(len(result), Equals, 2)
	c.Assert(result[0].Name, Equals, ts.alice.Name)
	c.Assert(result[0].Status, Equals, common.UserStatusOffline)

	// keep alive again, and should be online now
	ctx, cancel2 = context.WithCancel(context.Background())
	go func() {
		c.Assert(cli.Keepalive(ctx, pingInterval, 443), Equals, context.Canceled)
	}()
	defer cancel2()

	time.Sleep(2 * pingInterval)

	result = ts.listUsers(c, ts.token)
	c.Assert(len(result), Equals, 2)
	c.Assert(result[0].Name, Equals, ts.alice.Name)
	c.Assert(result[0].Status, Equals, common.UserStatusOnline)
}

func (ts *mainSuite) deleteUser(username string) (*http.Response, error) {
	url := fmt.Sprintf("/user/%s?token=%s", username, ts.token)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return nil, err
	}

	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)

	return rr.Result(), nil
}

func (ts *mainSuite) TestUserDelete(c *C) {
	u := user.User{
		Name:     "charlie",
		Password: "charlie123",
		Phone:    "charlie_phone",
		Email:    "charlie_email",
		NickName: "charlie_nick",
		HomeDir:  photodir + "/charlie",
	}
	err := ts.createUser(u, ts.token, true)
	c.Assert(err, IsNil)

	resp, err := ts.deleteUser(u.Name)
	c.Assert(err, IsNil)
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, Equals, http.StatusOK)

	u.Name = "denny"
	u.BotUser = true
	c.Assert(ts.createUser(u, ts.token, true), IsNil)

	// validate home dir
	_, err = os.Stat(filepath.Join(common.GetSambaUserDir(ts.h.conf.BaseDir), u.Name))
	c.Assert(err, IsNil)

	resp, err = ts.deleteUser(u.Name)
	c.Assert(err, IsNil)
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, Equals, http.StatusOK)

	_, err = os.Stat(filepath.Join(common.GetSambaUserDir(ts.h.conf.BaseDir), u.Name))
	c.Assert(err, NotNil)
	c.Assert(os.IsNotExist(err), Equals, true)

	// validate album
	a1 := &album.Album{Title: "hello", Description: "test album", Author: "test"}
	ts.createAlbum(c, a1)

	list := ts.listAlbums(c)
	c.Assert(len(list.Albums), Equals, 1)
	c.Assert(list.Albums[0].Title, Equals, a1.Title)

	resp, err = ts.deleteUser(ts.alice.Name)
	c.Assert(err, IsNil)
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, Equals, http.StatusOK)

	count := 0
	err = dbx.InQuery(ts.h.db, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "select count(*) from album").Scan(&count)
	})
	c.Assert(err, IsNil)
	c.Assert(count, Equals, 0)
}

func (ts *mainSuite) TestUsername(c *C) {
	for _, u := range []string{"David", "David123", "David123_TT", "David-123t"} {
		c.Assert(ts.createUser(user.User{Name: u}, ts.token, true), IsNil)
	}
	for _, u := range []string{"1david", ".David123", "$David123_TT", "David 123t"} {
		err := ts.createUser(user.User{Name: u}, ts.token, false)
		c.Assert(err, NotNil)
		fmt.Println(err)
		c.Assert(strings.Contains(err.Error(), common.ErrInvalidName.Error()), Equals, true)
	}
}

func (ts *mainSuite) TestUserDocs(c *C) {
	// without app doc directory, no user doc directory
	_, err := os.Stat(filepath.Join(ts.alice.HomeDir, common.AppDoc))
	c.Assert(os.IsNotExist(err), Equals, true, Commentf("%s", err))

	// create app doc directory and put one document
	docdir := common.GetDocDir(ts.h.conf.BaseDir)
	c.Assert(os.MkdirAll(docdir, 0744), IsNil)
	defer os.RemoveAll(docdir) // clean up after test

	filename := "readme"
	content := "hello world"
	c.Assert(ioutil.WriteFile(filepath.Join(docdir, filename), []byte(content), 0644), IsNil)

	u := user.User{
		Name:     "charlie",
		Password: "charlie123",
		Phone:    "charlie_phone",
		Email:    "charlie_email",
		NickName: "charlie_nick",
		HomeDir:  photodir + "/charlie",
	}
	err = ts.createUser(u, ts.token, true)
	c.Assert(err, IsNil)

	charlieDoc := filepath.Join(u.HomeDir, common.AppDoc)
	_, err = os.Stat(charlieDoc)
	c.Assert(err, IsNil)
	fis, err := ioutil.ReadDir(charlieDoc)
	c.Assert(err, IsNil)
	c.Assert(len(fis), Equals, 1)
	c.Assert(fis[0].Name(), Equals, filename)

	data, err := ioutil.ReadFile(filepath.Join(charlieDoc, filename))
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, content)
}
