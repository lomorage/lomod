package lomoframe

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net"
	"net/http"
	_ "net/http/pprof" // pprof for run time debug
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/client"
	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/logger"
	lnet "bitbucket.org/lomoware/lomo-backend/common/net"
	"bitbucket.org/lomoware/lomo-backend/common/release"
	"bitbucket.org/lomoware/lomo-backend/common/security"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	qrcode "github.com/skip2/go-qrcode"
)

const (
	confFile   = "lomoframe.json"
	qrFile     = "qrcode.png"
	namePrefix = "lomoframe-"
)

const (
	retryMountInterval = 30 * time.Second
	retryMountLoop     = -1
)

type qrinfo struct {
	Name      string
	Password  string
	ListenIPs []net.IP
	Port      uint
}

// Config is the configuration parameters for the handler.
type Config struct {
	Port     uint
	BaseDir  string
	LogDir   string
	MntDir   string
	ConfDir  string
	Username string
	Password string
	LomodURL string
	Token    string
	Interval time.Duration
}

// Handler is structure to handle http request.
type Handler struct {
	gCtx            context.Context
	accessLogger    *logger.AccessLogger
	accessFile      *logger.RotateFileHook
	logFile         *logger.RotateFileHook
	conf            *Config
	lomodClient     *client.Lomod
	status          common.SystemStatus
	statusLog       string
	mountStatus     common.TaskStatus
	mountLog        string
	mountCh         chan struct{}
	keepaliveStatus common.TaskStatus
	keepaliveLog    string
	keepaliveCh     chan struct{}
	listenIPs       []net.IP
}

// NewHandler construct handler object.
func NewHandler(ctx context.Context, c *Config) (*Handler, error) {
	h := &Handler{
		gCtx:        ctx,
		conf:        c,
		status:      common.SystemStatusAbnormal,
		mountCh:     make(chan struct{}),
		keepaliveCh: make(chan struct{}),
	}

	go func() {
		err := http.ListenAndServe("localhost:6060", nil)
		if err != nil {
			logrus.Errorf("Failed to start web server: %v\n", err)
		}
	}()

	err := h.initLog()
	if err != nil {
		return nil, err
	}

	h.listenIPs, err = lnet.ListIPs()
	if err != nil {
		logrus.Warnf("while listing ips: %v", err)
	}

	if err := h.loadConf(confFile); err != nil {
		return nil, err
	}

	go h.monitorLocalIP()

	if h.status == common.SystemStatusNew {
		return h, nil
	}

	go func() {
		err := h.mountRemote(retryMountLoop)
		if err != nil {
			h.statusLog = err.Error()
		}
	}()

	go h.keepalive()
	return h, nil
}

func (h *Handler) loadConf(cfile string) error {
	tmpConf := &Config{}
	filename := filepath.Join(h.conf.ConfDir, cfile)
	_, err := os.Stat(filename)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err := h.initConf(); err != nil {
			return err
		}
		if err := h.saveConf(cfile); err != nil {
			return err
		}
		h.status = common.SystemStatusNew
		return nil
	}

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, tmpConf); err != nil {
		return err
	}

	h.conf.Username = tmpConf.Username
	h.conf.Password = tmpConf.Password

	if _, err := os.Stat(filepath.Join(h.conf.ConfDir, qrFile)); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err := h.initQrcode(); err != nil {
			return err
		}
	} else if len(h.listenIPs) != 0 {
		if err := h.initQrcode(); err != nil {
			return err
		}
	}

	if tmpConf.LomodURL == "" {
		h.status = common.SystemStatusNew
		return nil
	}

	h.conf.LomodURL = tmpConf.LomodURL
	h.conf.Token = tmpConf.Token
	h.status = common.SystemStatusInited

	return nil
}

func (h *Handler) loginRemote(newLomodURL bool) error {
	// use old token to get system status firstly. If fail, try to login and get new token
	if h.conf.Token != "" {
		h.lomodClient = client.NewLomodWithToken(h.conf.LomodURL, h.conf.Token)
		if err := h.lomodClient.ValidateToken(); err == nil {
			logrus.Info("existing token is valid")
			if !newLomodURL {
				return nil
			}
			return h.saveConf(confFile)
		} else if common.IsErrServiceDown(err) {
			logrus.Info("peer is not up yet")
			return err
		}
		logrus.Info("existing token is not valid")
	}
	h.lomodClient = client.NewLomod(h.conf.LomodURL)
	passwd := security.EncryptPassword(h.conf.Username, h.conf.Password)
	resp, err := h.lomodClient.Login(h.conf.Username, passwd)
	if err != nil {
		return err
	} else if resp == nil {
		return errors.New("empty response")
	}

	h.conf.Token = resp.Token
	return h.saveConf(confFile)
}

func (h *Handler) saveConf(cfile string) error {
	f, err := os.Create(filepath.Join(h.conf.ConfDir, cfile))
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("  ", "  ")
	return encoder.Encode(h.conf)
}

func (h *Handler) initConf() error {
	h.conf.Username = namePrefix + common.RandomString(4)
	h.conf.Password = common.RandomString(8)

	return h.initQrcode()
}

func (h *Handler) initQrcode() error {
	qrc := qrinfo{
		Port:      h.conf.Port,
		Name:      h.conf.Username,
		ListenIPs: h.listenIPs,
		Password:  security.EncryptPassword(h.conf.Username, h.conf.Password),
	}

	data, err := json.Marshal(&qrc)
	if err != nil {
		return err
	}

	return qrcode.WriteFile(string(data), qrcode.Medium, 256, filepath.Join(h.conf.ConfDir, qrFile))
}

func (h *Handler) umountRemote() {
	out, err := exec.Command("sudo", "umount", "-f", h.conf.MntDir).CombinedOutput()
	if err != nil {
		logrus.Warn(string(out) + ": " + err.Error())
	}
}

func (h *Handler) mountRemote(retryCount int) error {
	// unmount firstly, warn only if failure
	h.umountRemote()

	var err error

	lomodHost := h.conf.LomodURL
	if strings.Contains(lomodHost, ":") {
		lomodHost, _, err = net.SplitHostPort(lomodHost)
		if err != nil {
			logrus.Warnf("invalid lomod host: %s", h.conf.LomodURL)
			h.mountStatus = common.TaskFail
			h.mountLog = err.Error()
			return err
		}
	}
	mountPasswd := security.LomoPasswdToOSPasswd(security.EncryptPassword(h.conf.Username, h.conf.Password))
	mountUser := "username=" + h.conf.Username
	cmds := []string{"mount",
		"-o", mountUser + ",password=" + mountPasswd,
		"//" + lomodHost + "/" + h.conf.Username,
		h.conf.MntDir}
	count := 0
	for {
		h.mountStatus = common.TaskInProgress
		out, err2 := exec.Command("sudo", cmds...).CombinedOutput()
		// remove password in log
		if err2 == nil {
			cmds[2] = mountUser
			logrus.Info(cmds)
			h.mountStatus = common.TaskSuccess
			h.mountLog = ""
			go h.monitorMount()
			return nil
		}
		err2 = errors.Wrap(err2, cmds[3]+":"+string(out))
		if err == nil {
			err = err2
		} else if err.Error() != err2.Error() {
			err = errors.Wrap(err, err2.Error())
		}
		count++
		h.mountStatus = common.TaskFail
		h.mountLog = err2.Error()
		logrus.Warnf("#%d mount got: %v", count, err2)
		if retryCount != retryMountLoop && count > retryCount {
			break
		}
		after := time.After(retryMountInterval)
		select {
		case <-after:
		case <-h.mountCh:
			logrus.Warnf("cancel mount retry")
			return nil
		}
	}
	return err
}

func (h *Handler) monitorMount() {
	// monitor mount can have longer interval
	interval := 10 * h.conf.Interval
	logrus.Info("start monitor mount")
	for {
		after := time.After(interval)
		select {
		case <-after:
			_, err := os.Stat(common.GetUserPhotoSharedDir(h.conf.MntDir))
			if err != nil {
				h.mountStatus = common.TaskFail
				h.mountLog = err.Error()
				logrus.Warnf("monitor mount got %v. restarting mount", err)
				go h.mountRemote(retryMountLoop)
				return
			}
			h.mountStatus = common.TaskSuccess
			h.mountLog = ""
		case <-h.mountCh:
			logrus.Warnf("cancel monitor mount")
			return
		}
	}
}

// Close closes handler.
func (h *Handler) Close() error {
	cmds := []string{"umount", h.conf.MntDir}
	out, err := exec.Command("sudo", cmds...).CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "%v : %s", cmds, string(out))
	}
	return h.accessFile.Close()
}

// CreateRouter associates handler with URL endpoints.
func (h *Handler) CreateRouter() *mux.Router {
	r := mux.NewRouter()
	r.HandleFunc("/", h.readme).Methods(http.MethodGet)

	r.HandleFunc("/start/{host}/{port}", h.start).Methods(http.MethodPost)
	r.HandleFunc("/reset", h.reset).Methods(http.MethodPost)
	r.HandleFunc("/reset/{host}/{port}", h.reset).Methods(http.MethodPost)

	r.HandleFunc("/log", h.downloadLogs).Methods("GET")
	r.HandleFunc("/log/{level}", h.setLogLevel).Methods("POST")

	r.HandleFunc("/system", h.system).Methods(http.MethodGet)

	r.Use(h.accessLogger.Middleware)
	return r
}

func (h *Handler) readme(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(`
<!DOCTYPE html>
<html>
  <head>
	  <title>https://lomorage.com/</title>
		<link rel="canonical" href="https://lomorage.com/"/>
		<meta name="robots" content="noindex">
		<meta charset="utf-8" />
		<meta http-equiv="refresh" content="0; url=https://lomorage.com/" />
	</head>
</html>
`))
}

// CORS is the header response to an OPTIONS request.
func (h *Handler) CORS(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.Header().Add("Access-Control-Allow-Methods", "PUT,GET,POST,HEAD,PATCH")
	w.Header().Add("Access-Control-Allow-Headers", "Content-Type")
}

func (h *Handler) reset(w http.ResponseWriter, r *http.Request) {
	if err := h.recreate(mux.Vars(r)["host"], mux.Vars(r)["port"]); err != nil {
		common.WriteError(w, err)
	}
}

func (h *Handler) recreate(host, port string) error {
	// always try umount
	h.umountRemote()

	if h.status > common.SystemStatusNew {
		// cancel ongoing keep alive if system status is not new
		go func() { h.keepaliveCh <- struct{}{} }()
		// cancel ongoing mount monitor if system status is not new
		go func() { h.mountCh <- struct{}{} }()
	}

	if host != "" {
		h.conf.LomodURL = host
	}
	if h.conf.LomodURL != "" {
		if port != "" {
			h.conf.LomodURL += ":" + port
		}
		if h.lomodClient == nil {
			h.lomodClient = client.NewLomodWithToken(h.conf.LomodURL, h.conf.Token)
		}
		if h.conf.Token == "" {
			// try login before delete. If login fail, no need delete
			passwd := security.EncryptPassword(h.conf.Username, h.conf.Password)
			resp, err := h.lomodClient.Login(h.conf.Username, passwd)
			if err != nil {
				logrus.Warnf("while reset, user %s login got %v", h.conf.Username, err)
			} else if resp == nil {
				logrus.Warnf("while reset, user %s login got empty response", h.conf.Username)
			} else {
				h.conf.Token = resp.Token
			}
		}

		if h.conf.Token != "" {
			if err := h.lomodClient.DeleteUser(h.conf.Username); err != nil {
				logrus.Warnf("delete user %s got %v", h.conf.Username, err)
			}
		}
	}

	if err := h.initConf(); err != nil {
		h.status = common.SystemStatusAbnormal
		h.statusLog = err.Error()
		return err
	}
	h.conf.LomodURL = ""
	h.conf.Token = ""
	if err := h.saveConf(confFile); err != nil {
		h.status = common.SystemStatusAbnormal
		h.statusLog = err.Error()
		return err
	}
	h.status = common.SystemStatusNew
	return nil
}

func (h *Handler) start(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)
	h.conf.LomodURL = mux.Vars(r)["host"]
	if h.conf.LomodURL == "" {
		common.WriteError(w, errors.New("no host specified to start"))
		return
	}
	if mux.Vars(r)["port"] != "" {
		h.conf.LomodURL += ":" + mux.Vars(r)["port"]
	}

	if err := h.loginRemote(true); err != nil {
		h.status = common.SystemStatusLoginFail
		common.WriteError(w, err)
		return
	}

	h.status = common.SystemStatusLoginSuccess
	if err := h.mountRemote(2); err != nil {
		h.status = common.SystemStatusAbnormal
		h.statusLog = err.Error()
		common.WriteError(w, err)
		return
	}

	go h.keepalive()
	go h.monitorMount()
	go h.monitorLocalIP()
}

func (h *Handler) keepalive() {
	lomodDown := true
	for {
		h.keepaliveStatus = common.TaskInProgress
		if lomodDown {
			// try login if lomod is down
			if err := h.loginRemote(false); err != nil {
				logrus.Warnf("connect remote: %s", err)
				lomodDown = common.IsErrServiceDown(err)
				h.keepaliveStatus = common.TaskFail
				h.keepaliveLog = err.Error()
				if lomodDown {
					h.status = common.SystemStatusNoLomod
				} else {
					h.status = common.SystemStatusLoginFail
					logrus.Warnf("login fail, no keepalive")
					return
				}
			} else {
				lomodDown = false
				h.status = common.SystemStatusLoginSuccess
				h.keepaliveStatus = common.TaskSuccess
			}
		}
		if !lomodDown {
			if err := h.lomodClient.Keepalive(h.gCtx, h.conf.Interval, int(h.conf.Port)); err != nil {
				logrus.Warnf("keep alive got %v", err)
				h.status = common.SystemStatusNoLomod
				h.keepaliveStatus = common.TaskFail
				h.keepaliveLog = err.Error()
				lomodDown = common.IsErrServiceDown(err)
			}
		}
		after := time.After(h.conf.Interval)
		select {
		case <-after:
		case <-h.keepaliveCh:
			logrus.Warnf("cancel current keep alive")
			return
		}
	}
}

func (h *Handler) monitorLocalIP() {
	ch := make(chan struct{})
	l, err := lnet.NewIPAddrListener()
	if err != nil {
		logrus.Errorf("while creating ip addr listener, got %v", err)
	}
	go l.MonitorIPChange(ch)

	// before monitor IP change, check if listen IPs were exist or not because it is
	// possible that network is not started when service starts. Thus, we need re-init
	// QR code before monitor IP change, otherwise, it will never get IP
	if len(h.listenIPs) == 0 {
		logrus.Info("no listen IP during init, retry qrcode generation")
	}
	for {
		h.listenIPs, err = lnet.ListIPs()
		if err != nil {
			logrus.Warnf("while listing ips: %v, sleep 10 seconds and retry", err)
			time.Sleep(10 * time.Second)
			continue
		}
		if err := h.initQrcode(); err != nil {
			logrus.Errorf("while saving qrcode, got %v", err)
		}
		select {
		case <-ch:
			logrus.Info("IP address changed, re-generate qr codes")
		case <-h.gCtx.Done():
			return
		}
	}
}

type systemInfo struct {
	OS               string
	APIVersion       string
	LomoFrameVersion string
	SystemStatus     common.SystemStatus
	SystemStatusLog  string
	MountStatus      common.TaskStatus
	MountLog         string
	KeepaliveStatus  common.TaskStatus
	KeepaliveLog     string
	ListenIPs        map[string][]net.IP
}

func (h *Handler) system(w http.ResponseWriter, r *http.Request) {
	info := &systemInfo{OS: runtime.GOOS, APIVersion: "1.0", LomoFrameVersion: release.Version,
		SystemStatus: h.status, SystemStatusLog: h.statusLog,
		MountStatus: h.mountStatus, MountLog: h.mountLog,
		KeepaliveStatus: h.keepaliveStatus, KeepaliveLog: h.keepaliveLog,
	}
	ips, err := lnet.ListIPsAttrs()
	if err != nil {
		logrus.Warnf("error to get IP list")
	} else {
		info.ListenIPs = ips
	}

	common.WriteBody(w, info)
}
