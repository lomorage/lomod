package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/release"

	"path/filepath"

	"archive/zip"
	"crypto/sha256"
	"io/ioutil"

	"time"

	"os/exec"
	"regexp"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

type platform struct {
	URL      string
	SHA256   string
	Version  string
	PreCmds  []string
	PostCmds []string
}

type releases map[string]platform

// A process that was just killed by the pre command (or a child it left behind) can keep
// the app directory locked for a moment after it's gone, so the swap retries for a few
// seconds before giving up. Variables, not constants, so tests don't have to wait it out.
var (
	renameAttempts   = 20
	renameRetryDelay = 500 * time.Millisecond
)

func main() {
	if err := newApp().Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func newApp() *cli.App {
	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("%s\n", c.App.Version)
	}

	app := cli.NewApp()

	app.Version = release.Version
	app.Usage = "personal photo backup solution backend daemon"
	app.Email = "support@lomorage.com"

	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "app-dir, a",
			Usage: "app directory to uncompress lomorage zip",
			Value: dir,
		},
		cli.StringFlag{
			Name:  "backup-dir, b",
			Usage: "directory to back up downloaded zip file and old release",
		},
		cli.StringFlag{
			Name:  "curr-version, c",
			Usage: "current version of lomorage app",
			Value: dir,
		},
		cli.StringFlag{
			Name:  "url, u",
			Usage: "url for release json",
			Value: "http://lomorage.github.io/release.json",
		},

		cli.StringFlag{
			Name: "platform-key, k",
			Usage: "top-level key to read from the release manifest, e.g. \"windows-cli\" -- " +
				"defaults to runtime.GOOS (\"windows\"/\"darwin\"/...), which is LomoAgent's own " +
				"key, not this CLI's; pass this explicitly for any manifest that carries both " +
				"(see installers/windows/install.ps1's -ManifestKey)",
		},

		cli.StringFlag{
			Name:  "precmd, prc",
			Usage: "PreCmd for upgrading",
			Value: "c:/stopLomoagent.bat",
		},

		cli.StringFlag{
			Name:  "precmdarg, prca",
			Usage: "PreCmd args for upgrading",
			Value: "",
		},

		cli.StringFlag{
			Name:  "postcmd, psc",
			Usage: "PostCmd for upgrading",
			Value: "c:/startLomoagent.bat",
		},

		cli.StringFlag{
			Name:  "postcmdarg, psca",
			Usage: "PostCmdArgs for upgrading",
			Value: "c:/lomoagent.exe",
		},

		cli.StringFlag{
			Name: "version-file",
			Usage: "file to write the new release's manifest version to once it has been swapped " +
				"into app-dir -- the value to pass back as --curr-version next time. It can't be " +
				"derived from the installed binaries: their own --version is stamped at build " +
				"time and need not equal the manifest's Version",
		},

		cli.BoolFlag{
			Name: "only-newer",
			Usage: "skip the manifest's release unless it was built after curr-version. Without " +
				"it, any version other than curr-version is installed, so a manifest that " +
				"points at an older build (e.g. a machine switched from a nightly to the " +
				"stable channel) downgrades. Only applies when both are lomod build versions " +
				"(YYYY-MM-DD.HH-MM-SS...); anything else still updates whenever they differ",
		},
	}

	app.Action = bootService

	return app
}

func bootService(ctx *cli.Context) error {
	if ctx.String("app-dir") == "" {
		return errors.New("invalid app dir")
	}
	if ctx.String("curr-version") == "" {
		return errors.New("invalid current version")
	}
	if ctx.String("url") == "" {
		return errors.New("invalid url")
	}
	p, err := downloadReleaseMeta(ctx.String("url"), ctx.String("platform-key"))
	if err != nil {
		return err
	}
	if p.Version == ctx.String("curr-version") {
		fmt.Println("No new version, skip upgrade")
		return nil
	}
	if ctx.Bool("only-newer") {
		if newer, comparable := isNewerBuild(p.Version, ctx.String("curr-version")); comparable && !newer {
			fmt.Printf("Release %s is not newer than %s, skip upgrade\n", p.Version, ctx.String("curr-version"))
			return nil
		}
	}

	fmt.Println("Got new version, start upgrade")

	tempRoot := ctx.String("backup-dir")
	if tempRoot == "" {
		tempRoot, err = ioutil.TempDir("", "lomod-temp")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tempRoot)
	}

	f, err := downloadReleaseBin(p.URL, p.SHA256, tempRoot)
	if err != nil {
		return err
	}
	defer os.Remove(f)

	tempUncompress := filepath.Join(tempRoot, "uncompress")
	// A previous run that failed after this point may have left its unzipped release behind
	// (backup-dir is persistent); unzipping on top of it would mix two releases' files.
	if err := os.RemoveAll(tempUncompress); err != nil {
		return err
	}
	if err := uncompress(f, tempUncompress); err != nil {
		os.RemoveAll(tempUncompress)
		return err
	}

	tempPreCmd := ctx.String("precmd")
	tempPreCmdArg := ctx.String("precmdarg")

	fmt.Println("start preUpgrade...")

	if err := preUpgrade(tempPreCmd, tempPreCmdArg); err != nil {
		// return err
		fmt.Println("preUpgrade fail...")
	}

	fmt.Println("start upgrade...")
	upgradeErr := upgrade(ctx.String("app-dir"), tempRoot, tempUncompress)
	if upgradeErr != nil {
		fmt.Println("upgrade fail:", upgradeErr)
		os.RemoveAll(tempUncompress)
	} else if versionFile := ctx.String("version-file"); versionFile != "" {
		if err := ioutil.WriteFile(versionFile, []byte(p.Version), 0644); err != nil {
			fmt.Println("write version file fail...", err)
		}
	}

	tempPostCmd := ctx.String("postcmd")
	tempPostCmdArg := ctx.String("postcmdarg")

	// Still run the post command when the swap failed: the pre command has already stopped
	// the app, and upgrade() leaves the old release in place, so this is what brings it back
	// up. The failure itself is reported through the exit code, so whatever scheduled this
	// run doesn't record a silent no-op as a successful update.
	fmt.Println("start postUpgrade...")
	if err := postUpgrade(tempPostCmd, tempPostCmdArg); err != nil && upgradeErr == nil {
		return err
	}
	return upgradeErr
}

// lomod build versions start with their build time (see the Makefile's LATEST_RELEASE), in a
// fixed-width format that sorts by time as a string.
var buildTimePrefix = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}\.\d{2}-\d{2}-\d{2}`)

// isNewerBuild reports whether candidate was built after current. comparable is false when
// either isn't a lomod build version, in which case newer means nothing.
func isNewerBuild(candidate, current string) (newer, comparable bool) {
	c := buildTimePrefix.FindString(candidate)
	cur := buildTimePrefix.FindString(current)
	if c == "" || cur == "" {
		return false, false
	}
	return c > cur, true
}

func downloadReleaseMeta(url, platformKey string) (*platform, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	d := json.NewDecoder(resp.Body)
	v := make(releases)
	err = d.Decode(&v)
	if err != nil {
		return nil, err
	}

	if platformKey == "" {
		platformKey = runtime.GOOS
	}
	p, ok := v[platformKey]
	if !ok {
		return nil, errors.Errorf("Unsupported platform: %s", platformKey)
	}
	return &p, nil
}

func downloadReleaseBin(url, expectedSHA, tmpdir string) (name string, err error) {
	tmpfile, err := ioutil.TempFile(tmpdir, "")
	if err != nil {
		return "", err
	}
	defer func() {
		tmpfile.Close()
		// tmpdir is usually the persistent backup dir: don't pile up a rejected or
		// half-finished download there on every scheduled run.
		if err != nil {
			os.Remove(tmpfile.Name())
		}
	}()
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	sha := sha256.New()
	mw := io.MultiWriter(sha, tmpfile)
	size, err := io.Copy(mw, resp.Body)
	if err != nil {
		return "", err
	}
	if size == 0 {
		return "", common.ErrEmptyAsset
	}

	// sha256Temp := fmt.Sprintf("%x", sha.Sum(nil))
	// fmt.Println("the file's SHA256=", sha256Temp)

	if !strings.EqualFold(fmt.Sprintf("%x", sha.Sum(nil)), expectedSHA) {
		return "", common.ErrAssetDiffHash
	}

	return tmpfile.Name(), nil
}

// uncompress extracts a release archive into dst. Windows releases are zips, macOS ones are
// .tar.gz (the download is saved under a temp name, so the format is sniffed, not read off an
// extension).
func uncompress(src, dst string) error {
	gz, err := isGzip(src)
	if err != nil {
		return err
	}
	if gz {
		return untarGz(src, dst)
	}
	return unzip(src, dst)
}

func isGzip(src string) (bool, error) {
	f, err := os.Open(src)
	if err != nil {
		return false, err
	}
	defer f.Close()

	magic := make([]byte, 2)
	if _, err := io.ReadFull(f, magic); err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return false, nil
		}
		return false, err
	}
	return magic[0] == 0x1f && magic[1] == 0x8b, nil
}

func untarGz(src, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	root := filepath.Clean(dst)
	inRoot := func(p string) bool {
		return strings.HasPrefix(p, root+string(os.PathSeparator))
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return err
	}

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		fpath := filepath.Join(root, hdr.Name)
		if fpath == root {
			// the archive's own "./" entry (`tar -C dist .`)
			continue
		}
		if !inRoot(fpath) {
			return fmt.Errorf("%s: illegal file path", fpath)
		}
		mode := hdr.FileInfo().Mode().Perm()

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(fpath, mode|0700); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
				return err
			}
			outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			if err := outFile.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink:
			// e.g. exiftool -> exiftool-pkg/bin/exiftool. Refuse a link pointing outside dst:
			// a later entry written through it would land outside dst too.
			target := hdr.Linkname
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(fpath), target)
			}
			if !inRoot(filepath.Clean(target)) {
				return fmt.Errorf("%s: illegal symlink target %s", fpath, hdr.Linkname)
			}
			if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
				return err
			}
			if err := os.Symlink(hdr.Linkname, fpath); err != nil {
				return err
			}
		}
	}
}

func unzip(src, dst string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	// Iterate through the files in the archive,
	// printing some of their contents.
	for _, f := range r.File {
		// Windows PowerShell 5.1's Compress-Archive, which built the Windows release zips,
		// writes entry names with backslashes -- including directory entries ("lib\auto\"),
		// which archive/zip then doesn't recognize as directories. Taken as-is, such an entry
		// became an empty *file* named "auto", and every later entry under it failed.
		name := strings.ReplaceAll(f.Name, `\`, "/")
		fpath := filepath.Join(dst, filepath.FromSlash(name))

		// Check for ZipSlip. More Info: http://bit.ly/2MsjAWE
		if !strings.HasPrefix(fpath, filepath.Clean(dst)+string(os.PathSeparator)) {
			return fmt.Errorf("%s: illegal file path", fpath)
		}

		if strings.HasSuffix(name, "/") || f.FileInfo().IsDir() {
			// Make Folder
			if err := os.MkdirAll(fpath, 0755); err != nil {
				return err
			}
			continue
		}

		// Make File
		// Not f.Mode(): that's the file's own mode, and a directory created 0644 (no
		// execute bit) can't have anything created inside it outside Windows.
		if err = os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		_, err = io.Copy(outFile, rc)
		if err != nil {
			return err
		}
		if err := rc.Close(); err != nil {
			return err
		}
		if err := outFile.Close(); err != nil {
			return err
		}
	}
	return nil
}

func preUpgrade(preCmd string, preCmdArg string) error {
	if preCmd == "" {
		return nil
	}
	cmd := exec.Command(preCmd, preCmdArg)

	//log.Printf("Running command and waiting for it to finish...")
	err := cmd.Run()
	return err
}

func upgrade(appDir, bakDir, downloadDir string) error {
	now := time.Now()
	bak := filepath.Join(bakDir, fmt.Sprintf("lomod-bak-%d%02d%02d_%02d%02d%02d", now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second()))
	if err := renameWithRetry(appDir, bak); err != nil {
		return errors.Wrapf(err, "move %s aside (is something still running from it, or using it as its working directory?)", appDir)
	}

	// move new one to specified app-dir
	if err := renameWithRetry(downloadDir, appDir); err != nil {
		// Don't leave app-dir missing: put the old release back so the post command can
		// still start it.
		if rbErr := renameWithRetry(bak, appDir); rbErr != nil {
			return errors.Wrapf(err, "move new release into %s (restoring the old one from %s also failed: %v)", appDir, bak, rbErr)
		}
		return errors.Wrapf(err, "move new release into %s", appDir)
	}
	return nil
}

func renameWithRetry(src, dst string) error {
	var err error
	for i := 0; i < renameAttempts; i++ {
		if i > 0 {
			time.Sleep(renameRetryDelay)
		}
		if err = os.Rename(src, dst); err == nil || os.IsNotExist(err) {
			return err
		}
	}
	return err
}

func postUpgrade(postCmd string, postCmdArg string) error {
	if postCmd == "" {
		return nil
	}
	cmd := exec.Command(postCmd, postCmdArg)

	//log.Printf("Running command and waiting for it to finish...")
	err := cmd.Start()
	return err
}
