package handler

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/asset"
	"bitbucket.org/lomoware/lomo-backend/common/check"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/logger"
	lnet "bitbucket.org/lomoware/lomo-backend/common/net"
	"bitbucket.org/lomoware/lomo-backend/common/preview"
	"bitbucket.org/lomoware/lomo-backend/common/scan"
	"bitbucket.org/lomoware/lomo-backend/common/security/cert"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/google/gops/agent"
	"github.com/gorilla/mux"
	"github.com/leslie-wang/govips/pkg/vips"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/net/webdav"
)

const (
	loginURI     = "/login"
	shareURI     = "/receive/asset/"
	defaultLimit = 100
)

// Config is the configuration parameters for the handler.
type Config struct {
	BaseDir            string
	CertDir            string
	MountDir           string
	BackupDir          string
	ExeDir             string
	LogDir             string
	DbFilename         string
	PreviewSizes       string
	PreviewVideoSizes  string
	MdnsName           string
	MdnsService        string
	MdnsDomain         string
	LomodCloudDomain   string
	LetsEncryptAPI     string
	AdminToken         string
	SambaConf          string
	WebFootHtml        string
	ListenPort         int
	ListenPortHTTPS    int
	ListenPortWebdev   int
	DisableMountMon    bool
	UseMdns            bool
	UseMemdb           bool
	HasStdout          bool
	Debug              bool
	UseJpg             bool
	DbClean            bool
	MaxUpload          uint
	MaxGetPreview      uint
	PreviewGenWorkers  uint
	MaxFileSize        uint
	BackupTime         []int
	CheckInterval      int
	CheckLeftDays      int
	MaxCaptureDuration time.Duration
	CastPollDuration   time.Duration
	ReplicaDuration    time.Duration
	FilePerm           os.FileMode
	FolderPerm         os.FileMode
	RemotePing         *net.IPAddr
}

type lomoCloudConf struct {
	host       string
	username   string
	password   string
	SubDomain  string
	token      string
	publicIP   string
	publicPort int
	localPort  int
}

// Handler is structure to handle http request.
type Handler struct {
	conf             *Config
	uuid             string
	transcodeApp     string
	exiftool         string
	ffprobe          string
	lomocloudConf    lomoCloudConf
	useCloudIPHelper bool
	tokenDuration    time.Duration
	db               *sql.DB
	dbtrace          *dbx.DBTrace
	memdb            *asset.Memdb
	previewDims      []types.Dimension
	previewVideoDims []types.Dimension
	gCtx             context.Context

	// maintenance
	mu                 *sync.Mutex
	lastMaintStartTime time.Time
	lastMaintEndTime   time.Time
	lastBackup         map[string]types.BackupResult

	uploadCh         chan struct{}
	previewCh        chan struct{}
	previewWaitCh    chan struct{}
	backupCh         chan struct{}
	replicaCh        chan replicateUser
	restoreCh        chan string
	restoreErrCh     chan error
	ccheckCh         chan struct{}
	ccheckRunner     *check.Runner
	userMountLock    *sync.Mutex
	userMountStatus  map[string]*userMountInfo
	userStatus       map[int]map[int]user.KeepaliveStatus
	userAssetSummary map[int]map[string]types.AssetSummary
	userAssetLock    *sync.Mutex
	castUsers        map[int]*castUser
	castLock         *sync.Mutex

	debugLock       *sync.Mutex
	debugCollectors map[string]*debugCollector

	previewPendingJobCheckTimeout time.Duration
	previewRunner                 *preview.Runner
	scanRunner                    *scan.Runner
	scanImportResult              importResult

	accessLogger *logger.AccessLogger
	backupLogger *logrus.Logger
	checkLogger  *logger.CheckLogger
	accessFile   *logger.RotateFileHook
	logFile      *logger.RotateFileHook
	backupFile   *logger.RotateFileHook
	checkFile    *logger.RotateFileHook
	listenIPs    []net.IP
	//	tunnel        *localtunnel.LocalTunnel
	cert *cert.Cert

	webdavFS        *myFS
	webdavDirLayout int

	Router *mux.Router
}

func probeTranscodeApp(exedir string) (string, error) {
	if exedir == "" {
		d, err := os.Executable()
		if err != nil {
			return "", err
		}
		d = filepath.Dir(d)

		// same directory firstly
		if d != "" {
			nd, err := probeTranscodeApp(d)
			if err == nil {
				return nd, nil
			}
			logrus.Infof("probe transcode app at %s got %v", d, err)
		}

		p, err := exec.LookPath(ffmpegBin)
		if err == nil {
			return p, nil
		}
		logrus.Infof("probe ffmpeg got %v", err)
		return exec.LookPath(avconvBin)
	}
	ffmpeg := filepath.Join(exedir, ffmpegBin)
	exist, err := common.IsFileExist(ffmpeg)
	if err == nil && exist {
		return ffmpeg, nil
	} else if err != nil {
		logrus.Infof("probe ffmpeg got %v", err)
	}
	avconv := filepath.Join(exedir, avconvBin)
	exist, err = common.IsFileExist(avconv)
	if err != nil {
		return "", err
	} else if !exist {
		return "", fmt.Errorf("neither ffmpeg nor avconv are found at %s", exedir)
	}
	return avconv, nil
}

func probeExiftool(exedir string) (string, error) {
	if exedir == "" {
		d, err := os.Executable()
		if err != nil {
			return "", err
		}
		d = filepath.Dir(d)

		// same directory firstly
		if d != "" {
			nd, err := probeExiftool(d)
			if err == nil {
				return nd, nil
			}
			logrus.Infof("probe exiftool at %s got %v", d, err)
		}

		p, err := exec.LookPath(exiftoolBin)
		if err == nil {
			return p, nil
		}
		logrus.Infof("probe exiftool got %v", err)
		return exec.LookPath(exiftoolBin)
	}
	exiftool := filepath.Join(exedir, exiftoolBin)
	_, err := os.Stat(exiftool)
	return exiftool, err
}

func probeFFProbe(exedir string) (string, error) {
	if exedir == "" {
		d, err := os.Executable()
		if err != nil {
			return "", err
		}
		d = filepath.Dir(d)

		// same directory firstly
		if d != "" {
			nd, err := probeFFProbe(d)
			if err == nil {
				return nd, nil
			}
			logrus.Infof("probe ffprobe at %s got %v", d, err)
		}

		p, err := exec.LookPath(ffprobeBin)
		if err == nil {
			return p, nil
		}
		logrus.Infof("probe ffprobe got %v", err)
		return exec.LookPath(ffprobeBin)
	}
	ffprobe := filepath.Join(exedir, ffprobeBin)
	_, err := os.Stat(ffprobe)
	return ffprobe, err
}

// previewQueueDepth bounds how many requests can be waiting for a preview
// generation slot at once. A generous multiple of the concurrency cap so an
// ordinary burst (a grid of tiles loading/scrolling at once) still gets
// queued and served, but a genuine flood (hundreds of requests within
// seconds, as seen from a freshly-launched or fast-scrolling mobile gallery)
// gets turned away immediately via ErrPreviewBusy instead of piling up
// hundreds of goroutines blocked on previewCh -- each blocked
// request/response/goroutine holds real memory, and on hardware this tight
// on RAM that pile-up is what actually exhausts it, not the generation work
// itself (which stays capped at MaxGetPreview the whole time).
func previewQueueDepth(maxConcurrent uint) uint {
	return maxConcurrent * 4
}

// acquirePreviewSlot reserves a spot in the (small, bounded) wait room
// first -- if that's already full, it returns false immediately rather than
// blocking. Once past the wait room, it blocks for an actual generation
// slot on previewCh as before; the wait-room reservation is released as
// soon as that happens, freeing room for the next waiter.
func (h *Handler) acquirePreviewSlot() bool {
	select {
	case h.previewWaitCh <- struct{}{}:
	default:
		return false
	}
	h.previewCh <- struct{}{}
	<-h.previewWaitCh
	return true
}

// releasePreviewSlot frees a generation slot acquired via acquirePreviewSlot.
func (h *Handler) releasePreviewSlot() {
	<-h.previewCh
}

// NewHandler construct handler object.
func NewHandler(ctx context.Context, c *Config) (*Handler, error) {
	previewDims, err := types.NewDimensions(c.PreviewSizes)
	if err != nil {
		return nil, err
	}
	previewVideoDims, err := types.NewDimensions(c.PreviewVideoSizes)
	if err != nil {
		return nil, err
	}

	h := &Handler{
		conf:             c,
		gCtx:             ctx,
		tokenDuration:    720 * time.Hour, // 30 days token
		previewDims:      previewDims,
		previewVideoDims: previewVideoDims,
		lastBackup:       map[string]types.BackupResult{},
		mu:               &sync.Mutex{},
		uploadCh:         make(chan struct{}, c.MaxUpload),
		previewCh:        make(chan struct{}, c.MaxGetPreview),
		previewWaitCh:    make(chan struct{}, previewQueueDepth(c.MaxGetPreview)),
		backupCh:         make(chan struct{}),
		ccheckCh:         make(chan struct{}),
		cert:             &cert.Cert{},
		lomocloudConf:    lomoCloudConf{host: c.LomodCloudDomain},
		userMountLock:    &sync.Mutex{},
		userMountStatus:  map[string]*userMountInfo{},
		userStatus:       map[int]map[int]user.KeepaliveStatus{},
		userAssetSummary: map[int]map[string]types.AssetSummary{},
		userAssetLock:    &sync.Mutex{},
		castUsers:        map[int]*castUser{},
		castLock:         &sync.Mutex{},
		scanImportResult: importResult{},

		// pcap capture
		debugLock:       &sync.Mutex{},
		debugCollectors: map[string]*debugCollector{},
	}

	// create samba user directory
	usrDir := common.GetSambaUserDir(h.conf.BaseDir)
	if _, err := os.Stat(usrDir); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		if err := os.Mkdir(usrDir, h.conf.FolderPerm); err != nil {
			return nil, err
		}
	}

	h.listenIPs, err = lnet.ListIPs()
	if err != nil {
		logrus.Warnf("while listing listening ips: %v", err)
	}

	h.transcodeApp, err = probeTranscodeApp(h.conf.ExeDir)
	if err != nil {
		return nil, err
	}

	h.ffprobe, err = probeFFProbe(h.conf.ExeDir)
	if err != nil {
		return nil, err
	}

	h.exiftool, err = probeExiftool(h.conf.ExeDir)
	if err != nil {
		return nil, err
	}

	if err := h.openSqliteDB(); err != nil {
		return nil, err
	}

	if err := h.initLog(); err != nil {
		return nil, err
	}

	h.ccheckRunner = check.NewRunner(h.exiftool, h.checkLogger)
	h.scanRunner = scan.NewRunner(logrus.StandardLogger())

	// router need access middle ware, so init after log
	h.Router = h.CreateRouter()

	if err := h.loadConf(); err != nil {
		logrus.Warnf("load conf get: %v", err)
	}

	if err := h.resetBotUserState(); err != nil {
		logrus.Warnf("reset bot user offline status got: %v", err)
	}

	vips.Startup(&vips.Config{ReportLeaks: true, MaxCacheFiles: 0, MaxCacheMem: 0, MaxCacheSize: 0})

	go h.processMDNS(h.gCtx, c.ListenPort, c.MdnsName, c.MdnsService, c.MdnsDomain, c.UseMdns)

	//go h.startTunnel(port)

	h.fixBadAssetSize()
	h.fixBadAssetAspectRatio()

	if h.conf.UseMemdb {
		if err := h.openMemDB(); err != nil {
			return h, err
		}
	}

	// The preview runner must exist before maintenance starts: its startup
	// consistency check calls h.previewRunner.PendingJobCount().
	postPreviewNotify := make(chan types.PreviewRequest)
	go h.postPreviewGeneration(ctx, postPreviewNotify, common.MaxPreviewWidth, common.DefaultVideoPreviewWidth)
	h.previewPendingJobCheckTimeout = 5 * time.Minute
	h.previewRunner = preview.NewRunner(h.gCtx, h.transcodeApp, h.ffprobe, h.exiftool,
		h.conf.FolderPerm, h.previewDims, h.previewVideoDims, int(h.conf.PreviewGenWorkers), &preview.NotifyConfig{
			ImageWidth: common.MaxPreviewWidth,
			VideoWidth: common.DefaultVideoPreviewWidth,
			Chan:       postPreviewNotify,
		})
	go h.previewRunner.Start()

	done := make(chan struct{})
	if len(c.BackupTime) == 3 {
		// run full consistence check in 7 days
		h.conf.CheckLeftDays = h.conf.CheckInterval
		go h.maintenance(c.BackupTime[0], c.BackupTime[1], c.BackupTime[2], 24*time.Hour, done, false)

		go func() {
			// always trigger consistency check when it starts firstly
			time.Sleep(time.Second)
			h.ccheckCh <- struct{}{}
		}()
	}

	go func(ctx context.Context) {
		if err := agent.Listen(agent.Options{}); err != nil {
			logrus.Warnf("gops listen got %v", err)
			return
		}
		<-ctx.Done()
		logrus.Info("stop gops")
	}(ctx)

	go h.refreshPortMapping(ctx)

	if err := h.loadChromeCastUsers(); err != nil {
		logrus.Warnf("load chromecast users get: %v", err)
	}
	go h.discoverChromeCastLoop()
	// TODO: disable auto start chromecast because not sure if user like to auto play
	// go h.startChromecast()

	go h.migrate()

	if h.conf.ReplicaDuration != time.Duration(0) {
		go h.replicateLoop(h.gCtx, h.conf.DbFilename, h.conf.ReplicaDuration)
	}

	go h.logCurrStatus()

	if !h.conf.DisableMountMon {
		logrus.Info("mount monitor enabled!")
		go h.monitorMount()
	} else {
		logrus.Info("mount monitor disabled!")
	}

	go func() {
		h.StartWebdevListener()
	}()

	return h, nil
}

func (h *Handler) logCurrStatus() {
	logrus.Infof("base dir: %s, exe dir: %s, mount dir: %s", h.conf.BaseDir, h.conf.ExeDir, h.conf.MountDir)
	info := h.getSystemInfo(true)
	content, err := json.Marshal(info)
	if err != nil {
		logrus.Warnf("marshal %v: %v", info, err)
	} else {
		logrus.Infof("system info: %s", content)
	}
}

func (h *Handler) openMemDB() error {
	h.memdb = asset.NewMemDB()

	return h.memdb.Build(h.db)
}

// Close closes handler.
func (h *Handler) Close() error {
	vips.Shutdown()
	for _, f := range []*logger.RotateFileHook{h.accessFile, h.logFile, h.backupFile, h.checkFile} {
		if f != nil {
			logrus.Infof("closing log file %s", f.Config.Filename)
			if err := f.Close(); err != nil {
				logrus.Warnf("while closing log file, got error: %v", err)
			}
		}
	}
	h.previewRunner.Stop()
	return h.db.Close()
}

// StartHTTPSListener starts to serve http request.
func (h *Handler) StartHTTPSListener() error {
	if h.lomocloudConf.SubDomain == "" {
		return errors.New("no sub domain allocated yet")
	}

	logrus.Infof("start listen at %s with lets encrypt URL: %s", h.lomocloudConf.SubDomain, h.conf.LetsEncryptAPI)

	hostPolicy := func(ctx context.Context, host string) error {
		if host == h.lomocloudConf.SubDomain {
			return nil
		}
		return fmt.Errorf("receive %s request, but only %s host is allowed", host, h.lomocloudConf.SubDomain)
	}

	m := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: hostPolicy,
		Cache:      autocert.DirCache(h.conf.CertDir),
		Client:     &acme.Client{DirectoryURL: h.conf.LetsEncryptAPI},
	}
	f := func(helloInfo *tls.ClientHelloInfo) (*tls.Certificate, error) {
		return m.GetCertificate(helloInfo)
	}
	tc := &tls.Config{
		GetCertificate: f,
		NextProtos: []string{
			acme.ALPNProto, // enable tls-alpn ACME challenges
		},
	}
	httpsSrv := &http.Server{
		Handler:   h.Router,
		Addr:      ":" + strconv.Itoa(h.conf.ListenPortHTTPS),
		TLSConfig: tc,
	}

	return httpsSrv.ListenAndServeTLS("", "")
}

// StartWebdevListener start serving webdev request
func (h *Handler) StartWebdevListener() {
	logrus.Infof("start listen webdev at :%d", h.conf.ListenPortWebdev)

	h.webdavFS = newMyFS(h)
	s := &srv{
		lomodHandler: h,
		h: &webdav.Handler{
			Prefix:     "",
			FileSystem: h.webdavFS,
			LockSystem: webdav.NewMemLS(),
		},
	}

	http.HandleFunc("/", s.ServeHTTP)
	if err := http.ListenAndServe(":"+strconv.Itoa(h.conf.ListenPortWebdev), s); err != nil {
		logrus.Warnf("webdev serve: %s", err)
	}
}

// CreateRouter associates handler with URL endpoints.
func (h *Handler) CreateRouter() *mux.Router {
	r := mux.NewRouter()

	// lomo-web
	h.loadUIHandler(r)

	// api backend
	r.HandleFunc(loginURI, h.login).Methods("GET")

	// deprecated
	r.HandleFunc("/category", h.listAssetsYearMonth).Methods("GET")
	r.HandleFunc("/category/{y}", h.listAssetsByYear).Methods("GET")
	r.HandleFunc("/category/{y}/{m}", h.listAssetsByMonth).Methods("GET")
	r.HandleFunc("/category/{y}/{m}/{d}", h.listAssetsByDay).Methods("GET")
	r.HandleFunc("/preview/{assetID}", h.getAssetPreview).Methods("GET")
	r.HandleFunc("/status", h.status).Methods("GET")
	r.HandleFunc("/mount", h.listMountedDir).Methods("GET")
	r.HandleFunc("/nw/", h.setEthNW).Methods("POST")
	r.HandleFunc("/wifi", h.setWifiAP).Methods("POST")
	r.HandleFunc("/wifi/{auth}/{ssid}/{password}", h.setWifiClient).Methods("POST")
	r.HandleFunc("/log", h.downloadLogs).Methods("GET")
	r.HandleFunc("/log", h.uploadLogs).Methods("POST")
	r.HandleFunc("/log/access", h.downloadAccessLog).Methods("GET")
	r.HandleFunc("/log/{level}", h.setLogLevel).Methods("POST")

	// asset
	r.HandleFunc("/asset", h.createAssetWithQuery).Methods("POST")
	r.HandleFunc("/asset", h.deleteAssetByJSON).Methods("DELETE")
	r.HandleFunc("/asset/{sha1}", h.createAsset).Methods("POST")
	r.HandleFunc("/asset/{sha1}", h.patchAsset).Methods("PATCH")
	r.HandleFunc("/asset/{sha1}", h.getAssetHashInfo).Methods("HEAD")
	r.HandleFunc("/asset/{sha1}/{year}/{month}/{day}", h.updateAssetTime).Methods("PUT")
	r.HandleFunc("/asset/{assetID}", h.getAsset).Methods("GET")
	r.HandleFunc("/asset/{assetID}", h.deleteAsset).Methods("DELETE")
	r.HandleFunc("/asset/preview/{assetID}", h.getAssetPreview).Methods("GET")
	r.HandleFunc("/asset/metadata/{assetID}", h.getAssetMetadatas).Methods("GET")
	r.HandleFunc("/asset/album/{assetID}", h.getAssetAlbums).Methods("GET")
	r.HandleFunc("/asset/label/{assetID}", h.getAssetLabels).Methods("GET")

	// search
	r.HandleFunc("/assets", h.searchAssets).Methods("GET")

	// hidden and favorite
	r.HandleFunc("/assets/hide", h.hideAssets).Methods("POST")
	r.HandleFunc("/assets/hide", h.unhideAssets).Methods("DELETE")
	r.HandleFunc("/assets/favorite", h.setAssetsFavorite).Methods("POST")
	r.HandleFunc("/assets/favorite", h.unsetAssetsFavorite).Methods("DELETE")

	// merkle tree
	r.HandleFunc("/assets/merkletree", h.listAssetsYearMonth).Methods("GET")
	r.HandleFunc("/assets/merkletree/{y}", h.listAssetsByYear).Methods("GET")
	r.HandleFunc("/assets/merkletree/{y}/{m}", h.listAssetsByMonth).Methods("GET")
	r.HandleFunc("/assets/merkletree/{y}/{m}/{d}", h.listAssetsByDay).Methods("GET")

	// metadata
	r.HandleFunc("/assets/metadata", h.addAssetMetadatas).Methods("POST")
	r.HandleFunc("/assets/metadata", h.getAssetsForMetadata).Methods("GET")
	r.HandleFunc("/assets/metadata/byid", h.getMetadataByIDs).Methods("POST")
	r.HandleFunc("/assets/metadata/places", h.getMetadataPlaces).Methods("GET")
	r.HandleFunc("/assets/metadata/places/{id}", h.updateMetadataPlace).Methods("PUT")
	r.HandleFunc("/assets/metadata/category", h.listAssetMetadataCategories).Methods("GET")
	r.HandleFunc("/assets/metadata/{category}/names", h.listAssetMetadataNames).Methods("GET")
	r.HandleFunc("/assets/metadata/{category}/{name}/values", h.listAssetMetadataValues).Methods("GET")

	// add/remove label, add/remove asset in label
	r.HandleFunc("/assets/label", h.listAssetLabels).Methods("GET")
	r.HandleFunc("/assets/label", h.addAssetLabel).Methods("POST")
	r.HandleFunc("/assets/label/{id}", h.listAssetsInLabel).Methods("GET")
	r.HandleFunc("/assets/label/{id}", h.addAssetsInLabel).Methods("POST")
	r.HandleFunc("/assets/label/{id}", h.deleteAssetsFromLabel).Methods("DELETE")

	// scan & import
	r.HandleFunc("/assets/scan", h.scan).Methods("POST")
	r.HandleFunc("/assets/scan", h.getScanResult).Methods("GET")
	r.HandleFunc("/assets/scan/log", h.getScanLog).Methods("GET")
	r.HandleFunc("/assets/scan/status", h.getScanStatus).Methods("GET")
	r.HandleFunc("/assets/scan/import/{name}", h.scanImport).Methods("POST")
	r.HandleFunc("/assets/scan/import/{name}", h.getScanImportResult).Methods("GET")
	r.HandleFunc("/assets/scan/preview/{year}/{month}/{day}/{sha1}", h.getScanPreview).Methods("GET")
	r.HandleFunc("/assets/scan/browse", h.browseDir).Methods("GET")

	// album
	r.HandleFunc("/album", h.listAlbums).Methods("GET")
	r.HandleFunc("/album", h.createAlbum).Methods("POST")
	r.HandleFunc("/album", h.updateAlbum).Methods("PUT")
	r.HandleFunc("/album", h.deleteAlbums).Methods("DELETE")
	r.HandleFunc("/album/merge", h.mergeAlbum).Methods("POST")
	r.HandleFunc("/album/{id}", h.deleteAlbum).Methods("DELETE")
	r.HandleFunc("/album/{id}/assets", h.listAlbumAssetsSummary).Methods("HEAD")
	r.HandleFunc("/album/{id}/assets", h.listAlbumAssets).Methods("GET")
	r.HandleFunc("/album/{id}/assets", h.addAssetsInAlbum).Methods("POST")
	r.HandleFunc("/album/{id}/assets", h.deleteAssetsFromAlbum).Methods("DELETE")

	// user
	r.HandleFunc("/user", h.listUsers).Methods("GET")
	r.HandleFunc("/user", h.createUser).Methods("POST")
	r.HandleFunc("/user", h.updateUser).Methods("PUT")
	r.HandleFunc("/user/{userName}", h.deleteUser).Methods("DELETE")
	r.HandleFunc("/user/token", h.validateToken).Methods("HEAD")
	r.HandleFunc("/user/space", h.getSpace).Methods("GET")
	r.HandleFunc("/user/conf", h.getUserConf).Methods("GET") // opaque data for client to store any conf
	r.HandleFunc("/user/conf", h.putUserConf).Methods("POST")
	r.HandleFunc("/user/ping/{interval}", h.userKeepalive).Methods("GET") //websocket library only supports GET method

	// group
	r.HandleFunc("/group", h.listGroup).Methods("GET")
	r.HandleFunc("/group", h.createGroupByJSON).Methods("POST")
	r.HandleFunc("/group/{groupID}", h.createGroup).Methods("POST")
	r.HandleFunc("/group/{groupID}", h.listGroupMembers).Methods("GET")
	r.HandleFunc("/group/{groupID}/{userID}", h.deleteMemberFromGroup).Methods("DELETE")
	r.HandleFunc("/group/{groupID}/{userID}", h.addMemberInGroup).Methods("POST")

	// share
	r.HandleFunc("/send", h.sendMultiple).Methods("POST")
	r.HandleFunc("/send/{shareID}", h.unshareAsset).Methods("DELETE")
	r.HandleFunc("/send/user/{userID}/{assetID}", h.sendToUser).Methods("POST")
	r.HandleFunc("/send/group/{groupID}/{assetID}", h.sendToGroup).Methods("POST")
	r.HandleFunc("/receive/user/{userID}", h.receiveMetaFromUser).Methods("GET")
	r.HandleFunc("/receive/group/{groupID}", h.receiveMetaFromGroup).Methods("GET")
	r.HandleFunc(shareURI+"{shareID}", h.receiveAsset).Methods("GET")
	r.HandleFunc("/receive/{shareID}", h.hideReceivedAsset).Methods("DELETE")
	r.HandleFunc("/receive/preview/{shareID}", h.receiveAssetPreview).Methods("GET")
	r.HandleFunc("/receive", h.listSharedAssets).Methods("GET")

	// cast
	r.HandleFunc("/cast/{userID}/{assetID}", h.castToUser).Methods("POST")

	// lomod cloud
	r.HandleFunc("/cloud/portmap", h.setupCloudPortMappingAuto).Methods("POST")
	r.HandleFunc("/cloud/portmap/{eport}", h.setupCloudPortMappingIn).Methods("POST")
	r.HandleFunc("/cloud", h.removeCloudAccount).Methods("DELETE")

	// cert
	//r.HandleFunc("/cert", h.createCert).Methods("POST")
	//r.HandleFunc("/cert", h.getCert).Methods("GET")

	// system
	r.HandleFunc("/system", h.system).Methods("GET")
	r.HandleFunc("/system/conf", h.getSystemConf).Methods("GET")
	r.HandleFunc("/system/conf/{key}/{value}", h.setSystemConf).Methods("POST")
	r.HandleFunc("/system/timezone", h.setTimeZone).Methods("POST")
	r.HandleFunc("/system/timezone", h.getTimeZone).Methods("GET")
	r.HandleFunc("/system/status", h.status).Methods("GET")
	r.HandleFunc("/system/factoryreset", h.factoryReset).Methods("POST")
	r.HandleFunc("/system/poweroff", h.poweroff).Methods("POST")
	r.HandleFunc("/system/iphelper", h.enableCloudIPHelper).Methods("POST")
	r.HandleFunc("/system/iphelper", h.disableCloudIPHelper).Methods("DELETE")
	r.HandleFunc("/system/mount", h.listMountedDir).Methods("GET")
	r.HandleFunc("/system/mount", h.umountDir).Methods("DELETE")
	r.HandleFunc("/system/nw/", h.setEthNW).Methods("POST")
	r.HandleFunc("/system/wifi", h.setWifiAP).Methods("POST")
	r.HandleFunc("/system/wifi/{auth}/{ssid}/{password}", h.setWifiClient).Methods("POST")

	r.HandleFunc("/system/log", h.downloadLogs).Methods("GET")
	r.HandleFunc("/system/log", h.uploadLogs).Methods("POST")
	r.HandleFunc("/system/log/access", h.downloadAccessLog).Methods("GET")
	r.HandleFunc("/system/log/{level}", h.setLogLevel).Methods("POST")

	// upgrade and backup
	r.HandleFunc("/system/upgrade", h.upgrade).Methods("POST")
	r.HandleFunc("/system/backup/{typ}/{user}/{code}", h.backupResult).Methods("POST")
	r.HandleFunc("/system/backup", h.setBackup).Methods("POST")
	r.HandleFunc("/system/backup", h.startBackup).Methods("PUT")
	r.HandleFunc("/system/backup", h.deleteBackup).Methods("DELETE")

	// restore
	r.HandleFunc("/system/restore/db/{userName}", h.restoreDBBackup).Methods("POST")

	// consistent check
	r.HandleFunc("/system/ccheck", h.startCCheck).Methods("POST")
	r.HandleFunc("/system/ccheck", h.getCCheckResult).Methods("GET")

	// debug capture packets
	r.HandleFunc("/debug/collect/start", h.startDebugCollect).Methods("POST")
	r.HandleFunc("/debug/collect/stop", h.stopDebugCollect).Methods("POST")

	// debug vips
	r.HandleFunc("/debug/report/vips", h.reportVIPS).Methods("POST")

	// Register pprof handlers
	r.HandleFunc("/debug/pprof/", pprof.Index)
	r.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	r.HandleFunc("/debug/pprof/profile", pprof.Profile)
	r.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	r.HandleFunc("/debug/pprof/trace", pprof.Trace)

	r.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))
	r.Handle("/debug/pprof/block", pprof.Handler("block"))
	r.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	r.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	r.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
	r.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))

	r.PathPrefix("/").HandlerFunc(h.CORS).Methods("OPTIONS")
	r.Use(h.accessLogger.Middleware)

	h.installLibrePhotoAPI(r)
	return r
}

func (h *Handler) installLibrePhotoAPI(r *mux.Router) {
	r.HandleFunc("/me", h.getMe).Methods("GET")
	r.HandleFunc("/assets/searchlist", h.searchAssetsInAlbums).Methods("GET")
	// list
	r.HandleFunc("/assets/date", h.getAllAssetsByDate).Methods("GET")

	r.HandleFunc("/api/auth/token/obtain/", h.obtainToken).Methods("POST")
	r.HandleFunc("/api/auth/token/refresh/", h.refreshToken).Methods("POST")

	r.HandleFunc("/api/user/{userID}/", h.getUserLibre).Methods("GET")

	//r.HandleFunc("/api/albums/user", h.listAlbumUsers).Methods("GET")
	r.HandleFunc("/api/albums/user/edit/", h.createAlbumLibre).Methods("POST")
	r.HandleFunc("/api/albums/user/edit/{id}/", h.editAlbumLibre).Methods("PATCH")
	r.HandleFunc("/api/albums/user/{id}/", h.listAlbumUser).Methods("GET")
	//r.HandleFunc("/api/albums/thing", h.listAlbumThings).Methods("GET")
	r.HandleFunc("/api/albums/thing/{id}/", h.listAlbumThing).Methods("GET")
	//r.HandleFunc("/api/albums/place", h.listAlbumPlaces).Methods("GET")
	r.HandleFunc("/api/albums/place/{id}/", h.listAlbumPlace).Methods("GET")
	r.HandleFunc("/api/locclust/", h.listAlbumPlaceClusters).Methods("GET")

	r.HandleFunc("/api/albums/date/{date}/", h.getAllAssetsByDate).Methods("GET")

	// person
	r.HandleFunc("/api/persons", h.listPerson).Methods("GET")
	r.HandleFunc("/api/persons/face/{person_id}", h.getPersonFace).Methods("GET")
	r.HandleFunc("/api/persons/face_photo/{person_id}", h.getPersonFacePhoto).Methods("GET")

	// media
	r.HandleFunc("/api/photos/{assetID}/", h.getAssetDetail).Methods("GET")
	r.HandleFunc("/api/photos/download", h.downloadAssets).Methods("POST")
	r.HandleFunc("/api/photosedit/hide/", h.assetHide).Methods("POST")
	r.HandleFunc("/api/photosedit/favorite/", h.assetFavorite).Methods("POST")
	r.HandleFunc("/api/photosedit/makepublic/", h.assetPublic).Methods("POST")

	r.HandleFunc("/media/square_thumbnails/{assetID}", h.getThumbnailMedian).Methods("GET")
	r.HandleFunc("/media/square_thumbnails_small/{assetID}", h.getThumbnailSmall).Methods("GET")
	r.HandleFunc("/media/thumbnails_big/{assetID}", h.getThumbnailBig).Methods("GET")
	r.HandleFunc("/media/video/{assetID}", h.getThumbnailBig).Methods("GET")

	r.HandleFunc("/api/sitesettings/", h.getSiteSettings).Methods("GET")
	r.HandleFunc("/api/rqavailable/", h.getRQAvailable).Methods("GET")

	r.HandleFunc("/api/stats/", h.getStats).Methods("GET")
	r.HandleFunc("/api/searchtermexamples/", h.getSearchExample).Methods("GET")
	//r.HandleFunc("/photos/download/{assetID}/", h.getAsset).Methods("GET")

	// dashboard
	r.HandleFunc("/api/locationsunburst", h.locationsunburst).Methods("GET")
	r.HandleFunc("/api/wordcloud", h.wordcloud).Methods("GET")
	r.HandleFunc("/api/photomonthcounts", h.photomonthcounts).Methods("GET")
	r.HandleFunc("/api/locationtimeline", h.locationtimeline).Methods("GET")
	r.HandleFunc("/api/socialgraph", h.socialgraph).Methods("GET")
	r.HandleFunc("/api/clusterfaces", h.clusterfaces).Methods("GET")
	r.HandleFunc("/api/clusterfaces/{id}", h.clusterfacesFile).Methods("GET")
	r.HandleFunc("/api/faces/inferred", h.listInferredFace).Methods("GET")
	r.HandleFunc("/api/faces/labeled", h.listLabeledFace).Methods("GET")
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
	w.Header().Add("Access-Control-Allow-Methods", "PUT,GET,POST,HEAD,PATCH,DELETE")
	w.Header().Add("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Content-Range, Content-Disposition, Content-Description, Accept-Encoding, X-CSRF-Token, Authorization, Access-Control-Allow-Origin, Access-Control-Allow-Methods") // nolint: lll
}

func (h *Handler) isMaintenance(p string) bool {
	if p != "/system/ccheck" {
		return false
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	return h.lastMaintStartTime.After(h.lastMaintEndTime)
}
