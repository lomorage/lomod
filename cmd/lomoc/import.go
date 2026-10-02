package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
	"unicode"

	"bitbucket.org/lomoware/lomo-backend/client"
	"bitbucket.org/lomoware/lomo-backend/cmd/lomoc/ui"
	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	lnet "bitbucket.org/lomoware/lomo-backend/common/net"
	scann "bitbucket.org/lomoware/lomo-backend/common/scan"
	"bitbucket.org/lomoware/lomo-backend/common/security"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"github.com/leslie-wang/zeroconf"
	"github.com/manifoldco/promptui"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

const (
	classifiedDir   = "classfied"
	unclassifiedDir = "unclassfied"
)

type loginInfo struct {
	UserID int
	Host   string
	Token  string
}

func login(ctx *cli.Context) (*loginInfo, error) {
	if len(ctx.Args()) < 2 {
		return nil, errors.New("invalid input, and should be 'lomoc import [username] [password]'")
	}

	host, name := getLomodHost(ctx)
	if host == "" {
		return nil, errors.New("not found lomorage host")
	}

	cli := client.NewLomod(host)
	passwd := security.EncryptPassword(ctx.Args()[0], ctx.Args()[1])
	l, err := cli.Login(ctx.Args()[0], passwd)
	if err != nil {
		return nil, err
	}

	fmt.Printf("Login %s successfully\n", name)
	return &loginInfo{Host: host, Token: l.Token, UserID: l.Userid}, nil
}

func probeLomodStatus(ip net.IP, port int) bool {
	u := "http://" + ip.String() + ":" + strconv.Itoa(port) + "/system"
	resp, err := http.Get(u)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return true
}

func discoverByMDNS(ctx *cli.Context) ([]*zeroconf.ServiceEntry, error) {
	fmt.Printf("Discovering .")
	done := make(chan struct{})
	go func() {
		for {
			after := time.After(time.Second)
			select {
			case <-done:
				fmt.Println()
				done <- struct{}{}
				return
			case <-after:
				fmt.Printf(".")
			}
		}
	}()
	entries, err := lnet.DiscoverMDNSServices(context.Background(), ctx.String("mdns-service"),
		ctx.String("mdns-domain"), 10*time.Second)
	done <- struct{}{}

	// wait ack
	<-done
	return entries, err
}

func findHostOS(texts []string) string {
	for _, t := range texts {
		parts := strings.Split(t, "=")
		if parts[0] != "os" {
			continue
		}
		switch parts[1] {
		case "linux":
			return "Raspberry Pi"
		case "windows":
			return "Windows"
		case "darwin":
			return "MAC"
		}
		break
	}
	return ""
}

func discover(ctx *cli.Context) error {
	entries, err := discoverByMDNS(ctx)
	if err != nil {
		return err
	}

	writer := tabwriter.NewWriter(os.Stdout, 4, 2, 2, ' ', 0)
	defer writer.Flush()

	writer.Write([]byte("Index\tName\tRun On\tIP:Port\tStatus\n"))
	writer.Write([]byte("-----\t----\t------\t-------\t------\n"))
	for i, entry := range entries {
		host := findHostOS(entry.Text)
		status := "bad"
		ip := "not present"
		if len(entry.AddrIPv4) != 0 {
			ip = fmt.Sprintf("%v:%d", entry.AddrIPv4[0], entry.Port)
			if probeLomodStatus(entry.AddrIPv4[0], entry.Port) {
				status = "ok"
			}
		}
		buf := fmt.Sprintf("%d\t%s\t%s\t%s\t%s\n", i, entry.ServiceRecord.Instance, host, ip, status)
		for i := 1; i < len(entry.AddrIPv4); i++ {
			if probeLomodStatus(entry.AddrIPv4[i], entry.Port) {
				status = "ok"
			} else {
				status = "bad"
			}
			buf += fmt.Sprintf("%s\t%s\t%s\t%v:%d\t%s\n", "  ", "  ", "  ", entry.AddrIPv4[i], entry.Port, status)
		}
		writer.Write([]byte(buf))
	}
	writer.Write([]byte("----------------------------------------------------------------\n"))
	return nil
}

func getLinkInfo() (string, string, error) {
	dir, err := getDefaultLomoDir()
	if err != nil {
		return "", "", err
	}

	linkDir := filepath.Join(dir, lomoLinkDir)
	if _, err := os.Stat(linkDir); err != nil {
		if !os.IsNotExist(err) {
			return "", "", err
		}
	}
	return linkDir, filepath.Join(linkDir, scanFile), nil
}

func saveScanResult(sf string, rootFolder *scann.File) error {
	f, err := os.Create(sf)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("  ", "  ")
	return encoder.Encode(rootFolder)
}

func scan(ctx *cli.Context) error {
	linkDir, sf, err := getLinkInfo()
	if err != nil {
		return err
	}
	if ctx.Bool("ui") {
		rootFolder, err := scann.ParseFile(sf)
		if err != nil {
			if os.IsNotExist(err) {
				return errors.New("'--ui' option is to revise photo/video create time from last scan. please run one scan without --ui option firstly")
			}
			return err
		}
		scann.FilterFolder(rootFolder)
		return ui.Interactive(rootFolder)
	}
	if len(ctx.Args()) != 1 {
		return errors.New("invalid input, and should be 'lomoc scan [directory]'")
	}

	// recreate link folder
	if err := os.RemoveAll(linkDir); err != nil {
		return err
	}
	if err := os.MkdirAll(linkDir, filePermission); err != nil {
		return err
	}

	r := scann.NewRunner(logrus.StandardLogger())
	go func(r *scann.Runner) {
		i := 0
		spinner := []string{"-", "/", "-", "\\"}

		for {
			after := time.After(time.Second)
			select {
			case <-after:
				fmt.Printf("\r[elapsed: %v | dir: %d(%d), files: %d images, %d videos(%d) %s", time.Since(r.Begin),
					r.Stats.TotalMediaDirs(), r.Stats.TotalDirs(),
					r.Stats.TotalImageFiles(), r.Stats.TotalVideoFiles(), r.Stats.TotalFiles(),
					spinner[i%len(spinner)])
				i++
			}
		}
	}(r)

	conf := scann.Config{RootFolderName: ctx.Args()[0], NoExifAnalysis: ctx.Bool("no-exif-analysis")}
	files, err := r.Start(conf, nil)
	if err != nil {
		return err
	}

	if len(files.Children) == 0 {
		fmt.Println("No media files or all media files have create time")
		return nil
	}

	if err := scann.Link(linkDir, classifiedDir, unclassifiedDir, files, filePermission); err != nil {
		return err
	}

	return saveScanResult(sf, files)
}

func importDir(ctx *cli.Context) error {
	if len(ctx.Args()) != 3 {
		return errors.New("Usage: [username] [password] [directory]")
	}

	ll, err := login(ctx)
	if err != nil {
		return err
	}

	if !strings.HasPrefix(ll.Host, localhost) {
		return errors.New("import only works for localhost now, not remote server")
	}

	cli := client.NewLomodWithToken(ll.Host, ll.Token)

	top, sf, err := getLinkInfo()
	if err != nil {
		return err
	}

	if _, err = os.Stat(sf); err != nil {
		if os.IsNotExist(err) {
			dir, err := filepath.Abs(ctx.Args()[2])
			if err != nil {
				return err
			}
			fmt.Println("import photos/videos from " + dir)
			err = importDirByAPI(cli, dir, !ctx.Bool("no-move"), !ctx.Bool("no-video"), ctx.Bool("use-exif-time"))
			if err != nil {
				return err
			}
			fmt.Println("please check import log " + common.GetScanImportLogFilename("/opt/lomorage/var/log", dir))
			return nil
		}
	}

	//return importDirV1(cli, sf, askConfirm)
	askConfirm := !ctx.Bool("yes")
	return importDirByTime(cli, filepath.Join(top, classifiedDir), askConfirm)
}

func importDirByAPI(cli *client.Lomod, top string, move, scanVideo, useExifTime bool) error {
	return cli.ScanAndImportDir(top, move, scanVideo, useExifTime)
}

func importDirByTime(cli *client.Lomod, top string, askConfirm bool) error {
	years, err := ioutil.ReadDir(top)
	if err != nil {
		return err
	}

	for _, year := range years {
		// skip hidden folders
		if strings.HasPrefix(year.Name(), ".") || common.IsSystemHiddenFile(year.Name()) {
			continue
		}
		y, err := strconv.Atoi(year.Name())
		if err != nil {
			return errors.Errorf("%s has wrong year folder name: %s", top, year.Name())
		}
		yDir := filepath.Join(top, year.Name())
		months, err := ioutil.ReadDir(yDir)
		if err != nil {
			return err
		}
		for _, month := range months {
			// skip hidden folders
			if strings.HasPrefix(month.Name(), ".") || common.IsSystemHiddenFile(month.Name()) {
				continue
			}
			m, err := strconv.Atoi(month.Name())
			if err != nil {
				return errors.Errorf("%s has wrong month folder name: %s", yDir, month.Name())
			}
			mDir := filepath.Join(yDir, month.Name())
			days, err := ioutil.ReadDir(mDir)
			if err != nil {
				return err
			}
			for _, day := range days {
				// skip hidden folders
				if strings.HasPrefix(day.Name(), ".") || common.IsSystemHiddenFile(day.Name()) {
					continue
				}
				d, err := strconv.Atoi(day.Name())
				if err != nil {
					return errors.Errorf("%s has wrong day folder name: %s", mDir, day.Name())
				}
				dDir := filepath.Join(mDir, day.Name())
				t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.Local)
				files, err := ioutil.ReadDir(dDir)
				if err != nil {
					return err
				}
				for _, file := range files {
					// skip hidden folders
					if strings.HasPrefix(file.Name(), ".") || common.IsSystemHiddenFile(file.Name()) {
						continue
					}
					// TODO: cache hash to avoid calculation everytime
					filename := filepath.Join(dDir, file.Name())
					exist, err := importFile(cli, filename, "", t, askConfirm)
					if err != nil {
						fmt.Printf("upload %s got %v\n", filename, err)
					} else if exist {
						fmt.Printf("already exist %s\n", filename)
					} else {
						fmt.Printf("success upload %s\n", filename)
					}
				}
			}
		}
	}
	return nil
}

func importDirV1(cli *client.Lomod, sf string, dryrun bool) error {
	rootFolder, err := scann.ParseFile(sf)
	if err != nil {
		return err
	}
	defer saveScanResult(sf, rootFolder)

	for _, folder := range rootFolder.Children {
		if err := importSubdirV1(cli, folder, dryrun); err != nil {
			return err
		}
	}
	return nil
}

func importSubdirV1(cli *client.Lomod, folder *scann.File, dryrun bool) error {
	if len(folder.Children) == 0 {
		exist, err := importFileV1(cli, folder, dryrun)
		if err != nil {
			fmt.Printf("upload %s got %v\n", folder.Path(), err)
		} else if exist {
			fmt.Printf("already exist %s\n", folder.Path())
		} else if dryrun {
			fmt.Printf("not exist %s\n", folder.Path())
		} else {
			fmt.Printf("success upload %s\n", folder.Path())
		}
		return err
	}
	for _, f := range folder.Children {
		if err := importSubdirV1(cli, f, dryrun); err != nil {
			return err
		}
	}
	return nil
}

func importFileV1(cli *client.Lomod, file *scann.File, dryrun bool) (bool, error) {
	var sha1 string
	if file.SHA1 == nil {
		var err error
		sha1, err = common.GetFileSHA(file.Path())
		if err != nil {
			return false, err
		}
		file.SHA1 = &sha1
	}
	ok, err := importFile(cli, file.Path(), *file.SHA1, file.CreateTime, dryrun)
	if err != nil {
		return ok, err
	}
	return ok, nil
}

func importFile(cli *client.Lomod, filename, hash string, createTime time.Time, askConfirm bool) (bool, error) {
	var metadatas []types.Metadata
	comment := mkComment(filename)
	if comment != "" {
		metadatas = append(metadatas, types.Metadata{Name: types.MetadataKeyComment, Value: comment})
	}

	f, err := os.Open(filename)
	if err != nil {
		return false, err
	}
	defer f.Close()

	// check exist or not
	exist, partial, err := cli.IsAssetExist(hash)
	if err != nil {
		return false, err
	} else if exist {
		return true, nil
	}

	if askConfirm {
		prompt := promptui.Select{
			Label: "Upload the asset or not",
			Items: []string{"Yes", "No"},
		}

		idx, _, err := prompt.Run()
		if err != nil {
			fmt.Printf("Prompt failed %v\n", err)
			return false, err
		}
		if idx == 1 {
			return false, nil
		}
	}

	// reset and start upload
	_, err = cli.UploadAsset(f, partial, hash, createTime, true, metadatas)
	return false, err
}

func mkComment(filename string) string {
	name := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	hasChinese := false
	for _, str := range name {
		if !unicode.Is(unicode.Han, str) {
			continue
		}
		hasChinese = true
		break
	}
	if !hasChinese {
		return ""
	}
	return name
}

func resetCreateTimeByEXIF(ctx *cli.Context) error {
	return resetCreateTime(ctx, true)
}

func resetCreateTimeByFS(ctx *cli.Context) error {
	return resetCreateTime(ctx, false)
}

func resetCreateTime(ctx *cli.Context, useEXIF bool) error {
	if len(ctx.Args()) < 3 {
		return errors.Errorf("invalid arguments. Usage: [user name] [password] [original asset directory 1] ...")
	}

	ll, err := login(ctx)
	if err != nil {
		return err
	}

	cli := client.NewLomodWithToken(ll.Host, ll.Token)
	for _, dir := range ctx.Args()[2:] {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if !ext.IsMediaFile(info.Name()) {
				return nil
			}
			sha1, err := common.GetFileSHA(path)
			if err != nil {
				log.Printf("ERROR: read SHA1 %s: %s", path, err)
				return nil
			}

			a, err := cli.GetAssetInfo(sha1)
			if err != nil {
				log.Printf("ERROR: pull asset %s info: %s", path, err)
				return nil
			}
			var t time.Time
			if useEXIF {
				t, err = getCreateTimeByEXIF(path)
			} else {
				t, err = getCreateTimeByFS(path)
			}
			if err != nil {
				log.Printf("ERROR: get %s's create time: %s", path, err)
				return nil
			}
			if t.Year() == a.Date.Time.Year() &&
				t.Month() == a.Date.Time.Month() &&
				t.Day() == a.Date.Time.Day() {
				log.Printf("INFO: %s has right create time in backend, skip", path)
				return nil
			}
			log.Printf("INFO: update %s from %d/%d/%d to %d/%d/%d", path,
				a.Date.Time.Year(), a.Date.Time.Month(), a.Date.Time.Day(),
				t.Year(), t.Month(), t.Day())
			err = cli.UpdateAssetCreateTime(sha1, t.Year(), int(t.Month()), t.Day())
			if err != nil {
				log.Printf("ERROR: update %s's create time: %s", path, err)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}
