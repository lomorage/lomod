package logger

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/sirupsen/logrus"
)

const (
	txnTypeHTTP   = "HTTP"
	unknownInt    = -1
	unknownString = "-"

	adminUserID = -1
)

// AllowedPath is to identify allowed path without token, bool means allow prefix
var AllowedPath = map[string]map[string]bool{
	"/user": {
		http.MethodPut: false, http.MethodGet: false, http.MethodPost: false, http.MethodDelete: true,
	},
	"/system":                    {http.MethodGet: false},
	"/system/backup":             {http.MethodDelete: false},
	"/system/conf":               {http.MethodGet: false},
	"/system/conf/webdav_layout": {http.MethodPost: true},
	"/system/mount":              {http.MethodGet: false},
	"/api/rqavailable/":          {http.MethodGet: false},
}

var skipFlushPath = []string{"/user/ping/"}

// AccessLogger is for common access logger.
type AccessLogger struct {
	*logrus.Logger
	DB            *sql.DB
	AdminToken    string
	isMaintenance func(string) bool
	skipLogURLs   map[string]map[string]struct{}
}

// NewAccessLogger creates logger.
func NewAccessLogger(l *logrus.Logger, db *sql.DB, adminToken string, skipLogURLs map[string]map[string]struct{},
	isMaintenance func(string) bool) *AccessLogger {
	return &AccessLogger{
		Logger:        l,
		DB:            db,
		AdminToken:    adminToken,
		isMaintenance: isMaintenance,
		skipLogURLs:   skipLogURLs,
	}
}

// ResponseLogger is wrapper of http.ResponseWriter that keeps track of its HTTP
// status code and body size.
type ResponseLogger struct {
	w          http.ResponseWriter
	skipFlush  bool
	status     int
	size       int
	Userid     int
	Deviceid   int
	Username   string
	Devicename string
	Err        error
}

// NewResponseLogger create instance of response logger
func NewResponseLogger(w http.ResponseWriter) *ResponseLogger {
	return &ResponseLogger{w: w, status: http.StatusOK}
}

// Header returns http header.
func (l *ResponseLogger) Header() http.Header {
	return l.w.Header()
}

// Write write bytes.
func (l *ResponseLogger) Write(b []byte) (int, error) {
	size, err := l.w.Write(b)
	l.size += size
	return size, err
}

// WriteHeader writes header.
func (l *ResponseLogger) WriteHeader(s int) {
	l.w.WriteHeader(s)
	l.status = s
}

// Hijack is to allow websocket library pass.
func (l *ResponseLogger) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := l.w.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("websocket: response does not implement http.Hijacker")
	}

	return h.Hijack()
}

// Status return status.
func (l *ResponseLogger) Status() int {
	return l.status
}

// Size returns current content size.
func (l *ResponseLogger) Size() int {
	return l.size
}

// Flush flush all buffer.
func (l *ResponseLogger) Flush() {
	if l.skipFlush {
		return
	}

	f, ok := l.w.(http.Flusher)
	if ok {
		f.Flush()
	}
}

// IsAdminUser return if user is admin or not
func (l *ResponseLogger) IsAdminUser() bool {
	return l.Userid == adminUserID
}

func isURLAllowAdminToken(p, m string) bool {
	methods, ok := AllowedPath[p]
	if ok {
		// check method also
		_, ok = methods[m]
		return ok
	}
	// check if it is based on prefix
	for u, methods := range AllowedPath {
		if !strings.HasPrefix(p, u) {
			continue
		}
		// check if the mothod is allowed to this prefix
		if methods[m] {
			return true
		}
	}
	return false
}

func (al *AccessLogger) isURLSkipLog(p, m string) bool {
	if al.skipLogURLs == nil {
		return false
	}
	methods, ok := al.skipLogURLs[p]
	if !ok {
		return false
	}
	_, ok = methods[m]
	return ok
}

// LogHTTPBegin logs http request begin
func (al *AccessLogger) LogHTTP(r *http.Request, deviceName, userName, url, suffix string, t time.Time, begin, trace bool) {
	prefix := "begin"
	if !begin {
		prefix = "end"
	}
	if trace {
		al.Logger.Tracef("%s %s: %s %s %s [%s] \"%s %s %s\"%s", txnTypeHTTP, prefix, common.GetRequestAddress(r),
			deviceName, userName, t.Format(common.TimeFormatLog), r.Method, url, r.Proto, suffix)
	} else {
		al.Logger.Infof("%s %s: %s %s %s [%s] \"%s %s %s\"%s", txnTypeHTTP, prefix, common.GetRequestAddress(r),
			deviceName, userName, t.Format(common.TimeFormatLog), r.Method, url, r.Proto, suffix)
	}
}

// Middleware intercepts requests and log request.
func (al *AccessLogger) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		if r.Method == http.MethodOptions {
			al.LogHTTP(r, "", "", "", "", start, true, false)
			defer func() {
				al.LogHTTP(r, "", "", "", "", time.Now(), false, false)
			}()
			next.ServeHTTP(w, r)
			return
		}
		_, u, l := al.preProcessReq(w, r, start.Format(common.TimeFormatLog))

		suffix := ""
		logLevelTrace := al.isURLSkipLog(r.URL.Path, r.Method)
		al.LogHTTP(r, l.Devicename, l.Username, u, suffix, start, true, logLevelTrace)

		defer func() {
			al.LogHTTP(r, l.Devicename, l.Username, u, suffix, time.Now(), false, logLevelTrace)
		}()
		if al.isMaintenance(r.URL.Path) {
			suffix = fmt.Sprintf("%d 0 %s", http.StatusServiceUnavailable, time.Since(start).Truncate(time.Second))
			http.Error(w, common.ErrMaintenance.Error(), http.StatusServiceUnavailable)
			return
		}

		// Call the next handler, which can be another middleware in the chain, or the final handler.
		next.ServeHTTP(l, r)

		suffix = fmt.Sprintf(" %d %d %s", l.status, l.size, time.Since(start).Truncate(time.Second))

		if l.size == 0 && l.status == 200 {
			w.Header().Add("Content-Length", "0")
		}

		if r.URL.String() != "/user/status" && l.status == 200 {
			l.Flush()
		}
	})
}

func (al *AccessLogger) preProcessReq(w http.ResponseWriter, r *http.Request, start string) (string,
	string, *ResponseLogger) {
	l := NewResponseLogger(w)
	addr := common.GetRequestAddress(r)
	// will skip local request if no token is supplied
	token, err := getUserToken(r)
	if err == nil && al.DB != nil {
		l.Err = dbx.InQuery(al.DB, func(ctx context.Context, tx *sql.Tx) error {
			l.Userid, l.Username, l.Deviceid, l.Devicename, err = user.ValidateTokenAll(ctx, tx, token)
			if err != nil {
				l.Userid = unknownInt
				l.Username = unknownString
				l.Deviceid = unknownInt
				l.Devicename = unknownString
			}
			return err
		})
	} else {
		l.Err = err
	}

	if l.Err != nil {
		// ok if token is admin token
		if token == al.AdminToken {
			if isURLAllowAdminToken(r.URL.Path, r.Method) {
				l.Userid = adminUserID
				l.Err = nil
				logrus.Debugf("%s-%s: use admin token access %s", addr, l.Devicename, r.URL.Path)
			} else {
				l.Err = common.ErrInvalidAdminToken
				logrus.Warnf("%s-%s-%s: admin token try to access unauthorized url %s", addr, l.Username,
					l.Devicename, r.URL.Path)
			}
		} else if common.IsErrNoRows(err) {
			l.Err = common.ErrInvalidToken
			logrus.Warnf("%s-%s-%s: invalid token access [%s] %s", addr, l.Username, l.Devicename, r.Method, r.URL.Path)
		} else if !isURLAllowAdminToken(r.URL.Path, r.Method) {
			logrus.Warnf("%s-%s-%s: while validating token access [%s] %s: %s", addr, l.Username, l.Devicename,
				r.Method, r.URL.Path, l.Err)
		}
	} else {
		for _, p := range skipFlushPath {
			if strings.HasPrefix(r.URL.Path, p) {
				l.skipFlush = true
			}
		}
	}

	u, _ := url.Parse(r.URL.String())
	q := u.Query()
	if q != nil {
		delete(q, "token")
		delete(q, "password")
		u.RawQuery = encodeQuery(q)
	}

	return addr, u.String(), l
}

func getUserToken(r *http.Request) (string, error) {
	// try header firstly
	auth := r.Header.Get("Authorization")
	parts := strings.Split(r.Header.Get("Authorization"), "=")
	if len(parts) == 2 && parts[0] == "token" {
		return parts[1], nil
	}

	token := r.URL.Query().Get("token")
	if token != "" {
		return token, nil
	}

	// probably jwt token
	claims, err := user.DecodeJWTClaim(strings.TrimPrefix(auth, "Bearer"))
	if err != nil {
		//logrus.Warnf("%v decode jwt token %s failure: %v", r.URL, auth, err)
		return "", common.ErrInvalidToken
	}
	return claims.Token, nil
}

func encodeQuery(v url.Values) string {
	if v == nil || len(v) == 0 {
		return ""
	}
	var buf strings.Builder
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		vs := v[k]
		for _, v := range vs {
			if buf.Len() > 0 {
				buf.WriteByte('&')
			}
			buf.WriteString(k)
			buf.WriteByte('=')
			buf.WriteString(v)
		}
	}
	return buf.String()
}
