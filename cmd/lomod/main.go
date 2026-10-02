package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/migrator"
	"bitbucket.org/lomoware/lomo-backend/common/release"
	"bitbucket.org/lomoware/lomo-backend/handler"
	"bitbucket.org/lomoware/lomo-backend/migrations/sqls/lomod"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
	"golang.org/x/crypto/acme"
)

func main() {
	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("%s\n", c.App.Version)
	}

	app := cli.NewApp()

	app.Version = release.Version
	app.Usage = "personal photo backup solution backend daemon"
	app.Email = "support@lomorage.com"

	dir, err := os.Getwd()
	if err != nil {
		logrus.Fatal(err)
		os.Exit(1)
	}
	hostname, err := os.Hostname()
	if err != nil {
		logrus.Fatal(err)
		os.Exit(1)
	}
	hostname = strings.Split(hostname, ".")[0]
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "base, b",
			Usage: "base directory to store db file",
			Value: dir,
		},
		cli.StringFlag{
			Name:  "mount-dir",
			Usage: "mount directory to find out mounted usb disk",
			Value: "/media",
		},
		cli.StringFlag{
			Name:   "exe-dir",
			Usage:  "executable directory for tools like avconv",
			Hidden: true,
		},
		cli.StringFlag{
			Name:   "log-dir",
			Usage:  "logfile directory",
			Hidden: true,
		},
		cli.UintFlag{
			Name:  "port, p",
			Value: common.LomodHTTPPort,
		},
		cli.UintFlag{
			Name:   "port-https",
			Hidden: true,
			Value:  common.LomodHTTPSPort,
		},
		cli.UintFlag{
			Name:  "port-webdev",
			Value: common.LomodWebdevPort,
		},
		cli.UintFlag{
			Name:   "max-upload",
			Usage:  "max concurrent request for asset upload request",
			Value:  3,
			Hidden: true,
		},
		cli.UintFlag{
			Name:   "max-fetch-preview",
			Usage:  "max concurrent request for fetch preview",
			Value:  5,
			Hidden: true,
		},
		cli.UintFlag{
			Name:   "preview-gen-workers",
			Usage:  "number of background workers generating default-size previews (see preview-size) after upload/scan",
			Value:  2,
			Hidden: true,
		},
		cli.UintFlag{
			Name:   "max-file-size",
			Usage:  "max capture file size in MB",
			Value:  50,
			Hidden: true,
		},
		cli.DurationFlag{
			Name:   "max-capture-duration",
			Usage:  "max capture duration",
			Value:  5 * time.Minute,
			Hidden: true,
		},
		cli.BoolFlag{
			Name:   "debug",
			Usage:  "debug mode for easy development",
			Hidden: true,
		},
		cli.BoolFlag{
			Name:   "use-jpg",
			Usage:  "use jpg instead of webp for thumbnail",
			Hidden: true,
		},
		cli.BoolFlag{
			Name:   "db-clean",
			Usage:  "clean non-existing asset db record automatically in consistence check",
			Hidden: true,
		},
		cli.BoolFlag{
			Name:   "no-memdb",
			Usage:  "disable memdb and read from db directly",
			Hidden: true,
		},
		cli.StringFlag{
			Name:  "preview-size",
			Usage: "list of image preview size. Multiple resolution is supported, and each is separated with ';'. Format is like <width1>x<height1>;<width2>x<height2>;...",
			Value: common.DefaultImagePreviewDims,
		},
		cli.StringFlag{
			Name:  "preview-size-video",
			Usage: "list of video preview size. 0 means disable video preview generation. Multiple resolution is supported, and each is separated with ';'. Format is like <width1>x<height1>;<width2>x<height2>;...",
			Value: common.DefaultVideoPreviewDims,
		},
		cli.StringFlag{
			Name:  "backup-time",
			Usage: "daily local backup time. Format is like hh:mm:ss. Timezone is local running machine timezone",
			Value: "02:00:00",
		},
		cli.UintFlag{
			Name:  "check-interval",
			Usage: "interval to run consistent check. Unit in day",
			Value: 7,
		},
		cli.BoolFlag{
			Name:  "no-mdns",
			Usage: "disable mdns for service discovery",
		},
		cli.StringFlag{
			Name:  "mdns-domain",
			Usage: "mdns search domain name",
			Value: common.MdnsLomodDomain,
		},
		cli.StringFlag{
			Name:  "mdns-service",
			Usage: "mdns service type",
			Value: common.MdnsLomodService,
		},
		cli.StringFlag{
			Name:  "mdns-name",
			Usage: "mdns service name",
			Value: hostname,
		},
		cli.BoolFlag{
			Name:   "no-stdout",
			Usage:  "disable stdout, which is mainly used at daemon mode at production",
			Hidden: true,
		},
		cli.StringFlag{
			Name:  "lomocloud",
			Usage: "host name of lomocloud",
			//Value:  "cloud1.lomorage.com,cloud2.lomorage.com,cloud3.lomorage.com",
			Hidden: true,
		},
		cli.BoolFlag{
			Name:   "lets-staging",
			Usage:  "use lets encrypt staging API endpoint",
			Hidden: true,
		},
		cli.StringFlag{
			Name:   "admin-token",
			Usage:  "admin token for some APIs",
			Hidden: true,
		},
		cli.StringFlag{
			Name:   "samba-conf",
			Usage:  "samba configuration file and lomorage will append",
			Hidden: true,
		},
		cli.DurationFlag{
			Name:  "chromecast-poll-duration",
			Usage: "chromecast device poll duration",
			Value: 10 * time.Minute,
		},
		cli.DurationFlag{
			Name:  "replica-duration",
			Usage: "db replicate interval. 0 means disable replicate",
		},
		cli.StringFlag{
			Name:  "file-perm",
			Usage: "file permission when creating asset file",
			Value: strconv.FormatInt(common.DefaultFilePermission, 8),
		},
		cli.StringFlag{
			Name:  "dir-perm",
			Usage: "dir permission when creating asset file",
			Value: strconv.FormatInt(common.DefaultFolderPermission, 8),
		},
		cli.StringFlag{
			Name:   "remote-ping",
			Usage:  "public ping target IP in case of debugging",
			Value:  "8.8.8.8",
			Hidden: true,
		},
		cli.BoolFlag{
			Name:  "with-mount-mon",
			Usage: "enable mount monitor utility",
		},
	}

	sort.Sort(cli.FlagsByName(app.Flags))

	app.Action = bootService

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
		os.Exit(1)
	}
}

func initConfig(ctx *cli.Context) *handler.Config {
	config := &handler.Config{
		BackupTime:         []int{},
		BaseDir:            ctx.GlobalString("base"),
		MountDir:           ctx.GlobalString("mount-dir"),
		ExeDir:             ctx.GlobalString("exe-dir"),
		ListenPort:         ctx.GlobalInt("port"),
		ListenPortHTTPS:    ctx.GlobalInt("port-https"),
		ListenPortWebdev:   ctx.GlobalInt("port-webdev"),
		CastPollDuration:   ctx.GlobalDuration("chromecast-poll-duration"),
		PreviewSizes:       ctx.GlobalString("preview-size"),
		PreviewVideoSizes:  ctx.GlobalString("preview-size-video"),
		MdnsName:           ctx.GlobalString("mdns-name"),
		MdnsService:        ctx.GlobalString("mdns-service"),
		MdnsDomain:         ctx.GlobalString("mdns-domain"),
		LomodCloudDomain:   ctx.GlobalString("lomocloud"),
		Debug:              ctx.GlobalBool("debug"),
		UseJpg:             ctx.GlobalBool("use-jpg"),
		DbClean:            ctx.GlobalBool("db-clean"),
		UseMemdb:           !ctx.GlobalBool("no-memdb"),
		UseMdns:            !ctx.GlobalBool("no-mdns"),
		HasStdout:          !ctx.GlobalBool("no-stdout"),
		DisableMountMon:    !ctx.GlobalBool("with-mount-mon"),
		MaxUpload:          ctx.GlobalUint("max-upload"),
		MaxGetPreview:      ctx.GlobalUint("max-fetch-preview"),
		PreviewGenWorkers:  ctx.GlobalUint("preview-gen-workers"),
		MaxFileSize:        ctx.GlobalUint("max-file-size") * 1024 * 1024,
		MaxCaptureDuration: ctx.GlobalDuration("max-capture-duration"),
		ReplicaDuration:    ctx.GlobalDuration("replica-duration"),
		CheckInterval:      int(ctx.GlobalUint("check-interval")),
		CheckLeftDays:      int(ctx.GlobalUint("check-interval")),
		WebFootHtml:        "",
	}

	// Windows has no "/media"-style USB auto-mount staging directory (the --mount-dir default),
	// so /mount would otherwise always report ErrDeviceNotMount there. Treat --base itself as
	// the mount pool instead, unless the user explicitly passed --mount-dir.
	if runtime.GOOS == "windows" && !ctx.GlobalIsSet("mount-dir") {
		config.MountDir = config.BaseDir
	}

	// environment variable will override specified value, this is to give flexibility to define dirs
	if os.Getenv("LOMOD_BASE_DIR") != "" {
		config.BaseDir = os.Getenv("LOMOD_BASE_DIR")
	}
	if os.Getenv("LOMOD_MOUNT_DIR") != "" {
		config.MountDir = os.Getenv("LOMOD_MOUNT_DIR")
	}
	if os.Getenv("LOMOD_PORT_HTTP") != "" {
		p, err := strconv.Atoi(os.Getenv("LOMOD_PORT_HTTP"))
		if err != nil {
			logrus.Warnf("invalid LOMOD_PORT_HTTP environment variable: %s", os.Getenv("LOMOD_PORT_HTTP"))
		} else {
			config.ListenPort = p
		}
	}
	if os.Getenv("LOMOD_PORT_HTTPS") != "" {
		p, err := strconv.Atoi(os.Getenv("LOMOD_PORT_HTTPS"))
		if err != nil {
			logrus.Warnf("invalid LOMOD_PORT_HTTPS environment variable: %s", os.Getenv("LOMOD_PORT_HTTPS"))
		} else {
			config.ListenPortHTTPS = p
		}
	}

	if os.Getenv("LOMOW_FOOT_HTML") != "" {
		config.WebFootHtml = os.Getenv("LOMOW_FOOT_HTML")
	}

	if os.Getenv("LOMOD_DISABLE_MOUNT_MONITOR") != "" {
		config.DisableMountMon = true
	}
	return config
}

func initDirs(ctx *cli.Context, config *handler.Config) error {
	mode, err := strconv.ParseInt(ctx.GlobalString("file-perm"), 8, 64)
	if err != nil {
		return nil
	}
	config.FilePerm = os.FileMode(mode)
	mode, err = strconv.ParseInt(ctx.GlobalString("dir-perm"), 8, 64)
	if err != nil {
		return nil
	}
	config.FolderPerm = os.FileMode(mode)
	logrus.Infof("using file permision: %s, folder permission: %s\n", config.FilePerm, config.FolderPerm)

	config.CertDir = common.GetCertDir(config.BaseDir)
	if _, err := os.Stat(config.CertDir); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err := os.MkdirAll(config.CertDir, common.DefaultFolderPermission); err != nil {
			return err
		}
	}

	config.SambaConf = ctx.GlobalString("samba-conf")
	if config.SambaConf != "" {
		if !filepath.IsAbs(config.SambaConf) {
			config.SambaConf = filepath.Join(
				common.GetConfDir(config.BaseDir), config.SambaConf)
		}
		data, err := ioutil.ReadFile(config.SambaConf)
		if err != nil {
			return err
		}
		config.SambaConf = string(data)
	}
	config.LogDir = ctx.GlobalString("log-dir")
	if config.LogDir == "" {
		config.LogDir = common.GetLogDir(config.BaseDir)
		if err := os.MkdirAll(config.LogDir, common.DefaultFolderPermission); err != nil {
			return err
		}
	}

	dbFile, err := dbx.GetDBFile(config.BaseDir, common.LomodDbFilename, common.DefaultFolderPermission)
	if err != nil {
		return err
	}
	config.DbFilename = dbFile

	config.AdminToken, err = common.LoadAndSaveAdminToken(filepath.Join(
		common.GetConfDir(config.BaseDir), common.LomodAdminFilename),
		ctx.GlobalString("admin-token"), common.DefaultFilePermission)
	return err
}

func bootService(ctx *cli.Context) error {
	config := initConfig(ctx)
	err := initDirs(ctx, config)
	if err != nil {
		return err
	}
	if ctx.GlobalBool("lets-staging") {
		config.LetsEncryptAPI = "https://acme-staging-v02.api.letsencrypt.org/directory"
	} else {
		config.LetsEncryptAPI = acme.LetsEncryptURL
	}

	parts := strings.Split(ctx.GlobalString("backup-time"), ":")
	for _, part := range parts {
		t, err := strconv.Atoi(part)
		if err != nil {
			return err
		}
		config.BackupTime = append(config.BackupTime, t)
	}

	if err := migrator.StartLomod(config.DbFilename, common.GetDocDir(config.BaseDir), lomod.SchemaStatements,
		config.FolderPerm); err != nil {
		// skip error and still continue
		logrus.Errorf("migrating got %s", err)
	}

	config.RemotePing, err = net.ResolveIPAddr("ip", ctx.GlobalString("remote-ping"))
	if err != nil {
		return err
	}

	c, cancel := context.WithCancel(context.Background())
	defer cancel()
	h, err := handler.NewHandler(c, config)
	if err != nil {
		return err
	}
	defer func() {
		if err := h.Close(); err != nil {
			logrus.Fatal(err)
		}
	}()

	go signalHandler(h, cancel)

	logrus.Printf("Arguments: %v", os.Args[1:])
	logrus.Printf("Start serving at ::%d (Version: %s)", config.ListenPort, release.Version)
	/*
		  Not enable lomo cloud until client is fully ready
			go func() {
				if err := h.StartHTTPSListener(); err != nil {
					logrus.Warnf("while starting https listener, got %v", err)
				}
			}()
	*/
	return http.ListenAndServe(fmt.Sprintf(":%d", config.ListenPort), h.Router)
}
