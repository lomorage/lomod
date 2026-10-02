package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/release"
	"github.com/manifoldco/promptui"
	"github.com/urfave/cli"
)

const (
	localhost      = "127.0.0.1"
	lomoDir        = ".lomo"
	lomoLinkDir    = "links"
	scanFile       = "scan.json"
	defaultDBFile  = "/opt/lomorage/var/assets.db"
	filePermission = os.FileMode(0755)
)

func main() {
	app := cli.NewApp()

	app.Version = release.Version

	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name: "discover, d",
		},
		cli.StringFlag{
			Name: "host, s",
		},
		cli.UintFlag{
			Name:  "port, p",
			Value: common.LomodHTTPPort,
		},
	}
	app.Commands = []cli.Command{
		{
			Name:      "reset",
			ShortName: "r",
			Usage:     "reset password, or home directory",
			Subcommands: []cli.Command{
				{
					Name:      "password",
					ShortName: "p",
					Action:    resetPassword,
					ArgsUsage: "[user name] [password]",
					Usage:     "reset username with given password",
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "db",
							Usage: "db filename with full path",
							Value: defaultDBFile,
						},
					},
				},
				{
					Name:      "home-dir",
					ShortName: "d",
					ArgsUsage: "[new media dir] ([user name])",
					Usage:     "reset users' home directory in DB, and input dir must be media dir. If username is not specified, it will reset all users' home directory",
					Action:    resetHomeDir,
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "db",
							Usage: "db filename with full path",
							Value: defaultDBFile,
						},
					},
				},
				{
					Name:      "backup-dir",
					ShortName: "b",
					ArgsUsage: "[new backup dir] ([user name])",
					Usage:     "reset users' backup directory in DB, and input dir must be media backup dir. If username is not specified, it will reset all users' backup directory",
					Action:    resetBackupDir,
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "db",
							Usage: "db filename with full path",
							Value: defaultDBFile,
						},
					},
				},
				{
					Name:      "create-time",
					ShortName: "c",
					Usage:     "reset asset create time by create time in exif tag or file system",
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "db",
							Usage: "db filename with full path",
							Value: defaultDBFile,
						},
					},
					Subcommands: []cli.Command{
						{
							Name:      "by-exif",
							ShortName: "e",
							Action:    resetCreateTimeByEXIF,
							ArgsUsage: "[user name] [password] [original asset directory 1] ...",
							Usage:     "reset asset create time by original date in exif tag",
						},
						{
							Name:      "by-fs",
							ShortName: "f",
							Action:    resetCreateTimeByFS,
							ArgsUsage: "[user name] [password] [original asset directory 1] ...",
							Usage:     "reset asset create time by create time in file system",
						},
					},
				},
			},
		},
		{
			Name:      "discover",
			ShortName: "d",
			Action:    discover,
			Usage:     "Discover lomo backend",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "mdns-domain",
					Usage: "mdns search domain name",
					Value: "local.",
				},
				cli.StringFlag{
					Name:  "mdns-service",
					Usage: "mdns service type",
					Value: "_lomod._tcp",
				},
			},
		},
		{
			Name:      "scan",
			ShortName: "s",
			Action:    scan,
			ArgsUsage: "[directory]",
			Usage:     "Scan given directory to check if create time tag is exist or not",
			Hidden:    true,
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "no-exif-analysis, n",
					Usage: "skip exif tag extraction",
				},
				cli.BoolFlag{
					Name:   "ui",
					Usage:  "display ui in commandline to revise image/video video if previous scan was wrong. To use the option, one scan without the option must be done firstly",
					Hidden: true, //hidden until it is fully supported
				},
			},
		},
		{
			Name:      "import",
			ShortName: "i",
			Action:    importDir,
			ArgsUsage: "[username] [password] [directory]",
			Usage:     "Import all photos from given directory into lomo backend with given username and password",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "no-move, n",
					Usage: "not moving original photos/videos, and only insert record in db",
				},
				cli.BoolFlag{
					Name:  "no-video, nv",
					Usage: "not scan video files. This is to speed up the first import process",
				},
				cli.BoolFlag{
					Name:  "use-exif-time, et",
					Usage: "Use time in exif tags instead of file system time. Order: Data/Time Original, CreateDate, GPSDateTime",
				},
			},
		},
		{
			Name:      "consistency",
			ShortName: "c",
			Usage:     "consistent check, such as file create time btw exif tag and filesystem, asset info btw db and filesystem",
			Subcommands: []cli.Command{
				{
					Name:      "db-check",
					ShortName: "d",
					Action:    checkConsistencyDB,
					ArgsUsage: "[db file with path]",
					Usage:     "scan and check consistency on asset info btw db and file system",
					Subcommands: []cli.Command{
						{
							Name:      "gps",
							ShortName: "g",
							Action:    checkGPS,
							ArgsUsage: "[db file with path]",
							Usage:     "scan and check GPS",
						},
					},
				},
				{
					Name:      "time-check",
					ShortName: "t",
					Action:    checkConsistencyTime,
					ArgsUsage: "[dir1 with images/videos to check] [dir2] ...",
					Usage:     "scan and check consistency on asset create time btw exif tag and file system",
				},
			},
		},
		{
			Name:      "icloud",
			ShortName: "a",
			Usage:     "import icloud photos (experiment feature)",
			Hidden:    true,
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "debug-log, d",
					Usage: "print out debug log",
				},
			},
			Subcommands: []cli.Command{
				{
					Name:      "login",
					ArgsUsage: "[username] [password]",
					Usage:     "login icloud user",
					Action:    icloudLogin,
				},
				{
					Name:      "usage",
					ArgsUsage: "",
					Usage:     "icloud storage usage",
					Action:    icloudStorageUsageDump,
				},
			},
		},
		{
			Name:      "frame",
			ShortName: "f",
			Usage:     "lomo-frame related commands",
			Subcommands: []cli.Command{
				{
					Name:   "gen-link",
					Usage:  "generate symbol link for shared photos if not exist. It is dry-run mode by default unless --yes is specified",
					Action: sharePhotoRelink,
					Flags: []cli.Flag{cli.BoolFlag{
						Name: "yes, y",
					},
						cli.StringFlag{
							Name:  "db",
							Usage: "db filename with full path",
							Value: defaultDBFile,
						},
					},
				},
			},
		},
		{
			Name:   "dump-tree",
			Action: dumpTree,
			Hidden: true,
			Usage:  "dump merklet tree structure",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "db",
					Usage: "db filename with full path",
					Value: defaultDBFile,
				},
			},
		},
		{
			Name:   "migrate",
			Hidden: true,
			Usage:  "migrate related commands",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "db",
					Usage: "db filename with full path",
					Value: defaultDBFile,
				},
			},
			Subcommands: []cli.Command{
				{
					Name:   "preview",
					Usage:  "migrate all previews",
					Hidden: true,
					Action: migratePreview,
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "preview-size",
							Usage: "list of image preview size. Multiple resolution is supported, and each is separated with ';'. Format is like <width1>x<height1>;<width2>x<height2>;...",
							Value: common.DefaultImagePreviewDims,
						},
						cli.BoolFlag{
							Name:  "save-ppm",
							Usage: "save generated preview image in ppm loselessly",
						},
					},
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func getLomodHost(ctx *cli.Context) (string, string) {
	if ctx.GlobalString("host") != "" {
		return fmt.Sprintf("%s:%d", ctx.GlobalString("host"), ctx.GlobalUint("port")), ctx.GlobalString("host")
	}
	if !ctx.GlobalBool("discover") {
		return fmt.Sprintf("%s:%d", localhost, ctx.GlobalUint("port")), localhost
	}

	entries, err := discoverByMDNS(ctx)
	if err != nil || len(entries) == 0 {
		return "", ""
	}

	if len(entries) == 1 {
		return entries[0].AddrIPv4[0].String() + ":" + strconv.Itoa(entries[0].Port), entries[0].ServiceRecord.Instance
	}

	hosts := []string{}
	for _, entry := range entries {
		hosts = append(hosts, entry.ServiceRecord.Instance+" - "+findHostOS(entry.Text))
	}
	prompt := promptui.Select{
		Label: "Select Lomorage Backend Server",
		Items: hosts,
	}

	idx, _, err := prompt.Run()
	if err != nil {
		fmt.Printf("Prompt failed %v\n", err)
		return "", ""
	}
	return entries[idx].AddrIPv4[0].String() + ":" + strconv.Itoa(entries[idx].Port), entries[idx].ServiceRecord.Instance
}

func getDefaultLomoDir() (string, error) {
	hdir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	ldir := filepath.Join(hdir, lomoDir)
	return ldir, os.MkdirAll(ldir, filePermission)
}
