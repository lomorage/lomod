package client

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Lomod is the client interface to access lomod home service.
type Lomod struct {
	host  string
	login user.LoginResp
}

// PartialContent is partial upload asset data.
type PartialContent struct {
	Size int
	Hash string
}

// NewLomod returns lomod client.
func NewLomod(host string) *Lomod {
	return NewLomodWithToken(host, "")
}

// NewLomodWithToken create lomod with given token.
func NewLomodWithToken(host, token string) *Lomod {
	if strings.HasPrefix(host, "http://") {
		host = strings.TrimPrefix(host, "http://")
	}
	host = strings.TrimSuffix(host, "/")
	if !strings.Contains(host, ":") {
		host += ":" + strconv.Itoa(common.LomodHTTPPort)
	}
	ld := &Lomod{host: host}
	if token != "" {
		ld.login = user.LoginResp{Token: token}
	}
	return ld
}

func (ld *Lomod) request(method, url string, q map[string]string, req interface{}) error {
	queries := make([]string, len(q))
	idx := 0
	for k, v := range q {
		queries[idx] = k + "=" + v
		idx++
	}
	if len(queries) > 0 {
		url += "?" + strings.Join(queries, "&")
	}
	resp, err := ld.requestReply(method, url, req)
	if err != nil {
		return err
	}
	return checkAndReturnReply(resp)
}

func (ld *Lomod) requestReply(method, url string, req interface{}) (*http.Response, error) {
	url = ld.host + url
	if req == nil {
		return requestReply(method, url, getHeaders(ld.login.Token), nil)
	}

	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(req); err != nil {
		return nil, err
	}

	return requestReply(method, url, getHeaders(ld.login.Token), buf)
}

func (ld *Lomod) get(url string, q map[string]string) (*http.Response, error) {
	queries := make([]string, len(q))
	idx := 0
	for k, v := range q {
		queries[idx] = k + "=" + v
		idx++
	}
	if len(queries) > 0 {
		url += "?" + strings.Join(queries, "&")
	}
	return ld.requestReply(http.MethodGet, url, nil)
}

func (ld *Lomod) getReplyJSON(url string, reply interface{}) error {
	resp, err := ld.get(url, nil)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return makeStatusError(resp.Body)
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(reply)
}

// Keepalive keeps websocket connection with lomod.
func (ld *Lomod) Keepalive(ctx context.Context, pingInterval time.Duration, port int) error {
	conn, err := net.Dial("tcp", ld.host)
	if err != nil {
		return err
	}
	defer conn.Close()

	u, err := url.Parse("http://" + ld.host + "/user/ping/" + pingInterval.String() +
		"?port=" + strconv.Itoa(port))
	if err != nil {
		return err
	}

	u.Scheme = "ws" // FIXME wss when we move to ssl

	d := websocket.Dialer{
		NetDial: func(net, addr string) (net.Conn, error) {
			return conn, nil
		},
	}

	webconn, resp, err := d.Dial(u.String(), getHeaders(ld.login.Token))
	if err != nil {
		return err
	}
	defer webconn.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		return common.MakeStatusError(resp.Body)
	}
	defer resp.Body.Close()

	closeChan := make(chan struct{})
	webconn.SetCloseHandler(func(code int, text string) error {
		logrus.Info("receive close control message")
		close(closeChan)
		return nil
	})

	ticker := time.NewTicker(pingInterval)
	closeData := []byte{0}
	pingData := []byte("ping")
	defer ticker.Stop()
	for {
		select {
		case <-closeChan:
			return webconn.WriteControl(websocket.CloseMessage, closeData, time.Now().Add(time.Second))
		case <-ctx.Done():
			logrus.Info("context is done, sending close control message")
			if err := webconn.WriteControl(websocket.CloseMessage, closeData, time.Now().Add(time.Second)); err != nil {
				return errors.Wrap(ctx.Err(), err.Error())
			}
			return ctx.Err()
		case <-ticker.C:
			start := time.Now()
			if err := webconn.WriteControl(websocket.PingMessage, pingData, time.Now().Add(time.Second)); err != nil {
				logrus.Warnf("keepalive spend %s: %v", time.Since(start), err)
				return err
			}
			logrus.Debugf("ping spend %s", time.Since(start))
		}
	}
}
