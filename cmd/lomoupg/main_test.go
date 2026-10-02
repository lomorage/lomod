package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

const (
	oldVersion = "2026-01-01.00-00-00.0.aaaaaaa"
	newVersion = "2026-02-02.00-00-00.0.bbbbbbb"
)

// env is one fake install: an app dir holding the "old" release, a persistent backup dir,
// pre/post hooks that leave marker files behind, and an HTTP server publishing a "new"
// release the same way lomorage.com/release.json + GitHub Releases do.
type env struct {
	t          *testing.T
	appDir     string
	backupDir  string
	preMarker  string
	postMarker string
	preCmd     string
	postCmd    string
	server     *httptest.Server
	sha        string // what the manifest claims; tests may corrupt it
}

func newEnv(t *testing.T) *env {
	t.Helper()
	root := t.TempDir()
	e := &env{
		t:          t,
		appDir:     filepath.Join(root, "lomod"),
		backupDir:  filepath.Join(root, "lomod-update-backup"),
		preMarker:  filepath.Join(root, "pre.ran"),
		postMarker: filepath.Join(root, "post.ran"),
	}
	for _, d := range []string{e.appDir, e.backupDir, filepath.Join(root, "hooks")} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(e.appDir, "release.txt"), "old")
	writeFile(t, filepath.Join(e.appDir, "only-in-old.txt"), "old")
	e.preCmd = writeHook(t, filepath.Join(root, "hooks"), "pre", e.preMarker)
	e.postCmd = writeHook(t, filepath.Join(root, "hooks"), "post", e.postMarker)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range map[string]string{"release.txt": "new", "sub/nested.txt": "new"} {
		// Regular-file modes and no directory entries, like the zips real tools produce.
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		h.SetMode(0644)
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	zipBytes := buf.Bytes()
	e.sha = fmt.Sprintf("%x", sha256.Sum256(zipBytes))

	mux := http.NewServeMux()
	mux.HandleFunc("/release.json", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(releases{
			// runtime.GOOS is LomoAgent's key; the CLI must never pick it up.
			runtime.GOOS:  {Version: "agent", URL: e.server.URL + "/missing.zip", SHA256: "00"},
			"windows-cli": {Version: newVersion, URL: e.server.URL + "/release.zip", SHA256: e.sha},
		})
	})
	mux.HandleFunc("/release.zip", func(w http.ResponseWriter, r *http.Request) {
		w.Write(zipBytes)
	})
	e.server = httptest.NewServer(mux)
	t.Cleanup(e.server.Close)
	return e
}

func (e *env) run(currVersion string, extraArgs ...string) error {
	args := []string{"lomoupg",
		"-a", e.appDir,
		"-b", e.backupDir,
		"-c", currVersion,
		"-u", e.server.URL + "/release.json",
		"-k", "windows-cli",
		"--precmd", e.preCmd,
		"--postcmd", e.postCmd,
		"--postcmdarg", e.backupDir,
	}
	return newApp().Run(append(args, extraArgs...))
}

// assertSwapped checks the run installed the manifest's release.
func (e *env) assertSwapped() {
	e.t.Helper()
	if got := readFile(e.t, filepath.Join(e.appDir, "release.txt")); got != "new" {
		e.t.Errorf("app dir release.txt = %q, want the manifest's release", got)
	}
	if b := e.backups(); len(b) != 1 {
		e.t.Errorf("backups = %v, want exactly one", b)
	}
	waitFor(e.t, e.postMarker)
}

func (e *env) backups() []string {
	m, err := filepath.Glob(filepath.Join(e.backupDir, "lomod-bak-*"))
	if err != nil {
		e.t.Fatal(err)
	}
	return m
}

// assertOldReleaseIntact checks a failed or skipped run left the install exactly as it was.
func (e *env) assertOldReleaseIntact() {
	e.t.Helper()
	if got := readFile(e.t, filepath.Join(e.appDir, "release.txt")); got != "old" {
		e.t.Errorf("app dir release.txt = %q, want the old release untouched", got)
	}
	if !exists(filepath.Join(e.appDir, "only-in-old.txt")) {
		e.t.Error("old release's files are gone from app dir")
	}
	if b := e.backups(); len(b) != 0 {
		e.t.Errorf("unexpected backups left behind: %v", b)
	}
	// Neither the unzipped release nor the downloaded zip may pile up in the persistent
	// backup dir on every failed scheduled run.
	left, err := os.ReadDir(e.backupDir)
	if err != nil {
		e.t.Fatal(err)
	}
	for _, f := range left {
		e.t.Errorf("%s left behind in backup dir", f.Name())
	}
}

func writeHook(t *testing.T, dir, name, marker string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		p := filepath.Join(dir, name+".bat")
		writeFile(t, p, "@echo off\r\necho ran> \""+marker+"\"\r\n")
		return p
	}
	p := filepath.Join(dir, name+".sh")
	writeFile(t, p, "#!/bin/sh\necho ran > '"+marker+"'\n")
	if err := os.Chmod(p, 0755); err != nil {
		t.Fatal(err)
	}
	return p
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// waitFor polls for a hook's marker: the post command is started, not waited on.
func waitFor(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !exists(path) {
		if time.Now().After(deadline) {
			t.Fatalf("%s never appeared", path)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func fastRetries(t *testing.T) {
	attempts, delay := renameAttempts, renameRetryDelay
	renameAttempts, renameRetryDelay = 3, 50*time.Millisecond
	t.Cleanup(func() { renameAttempts, renameRetryDelay = attempts, delay })
}

func TestUpgradeSwapsInNewRelease(t *testing.T) {
	e := newEnv(t)
	// Leftover from an earlier failed run; must not leak into the new install.
	stale := filepath.Join(e.backupDir, "uncompress")
	if err := os.MkdirAll(stale, 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(stale, "stale.txt"), "stale")

	if err := e.run(oldVersion); err != nil {
		t.Fatalf("upgrade failed: %v", err)
	}

	if got := readFile(t, filepath.Join(e.appDir, "release.txt")); got != "new" {
		t.Errorf("app dir release.txt = %q, want new", got)
	}
	if got := readFile(t, filepath.Join(e.appDir, "sub", "nested.txt")); got != "new" {
		t.Errorf("app dir sub/nested.txt = %q, want new", got)
	}
	for _, leftover := range []string{"only-in-old.txt", "stale.txt"} {
		if exists(filepath.Join(e.appDir, leftover)) {
			t.Errorf("%s leaked into the new install", leftover)
		}
	}
	b := e.backups()
	if len(b) != 1 {
		t.Fatalf("backups = %v, want exactly one", b)
	}
	if got := readFile(t, filepath.Join(b[0], "only-in-old.txt")); got != "old" {
		t.Errorf("backup only-in-old.txt = %q, want old", got)
	}
	if !exists(e.preMarker) {
		t.Error("pre command did not run")
	}
	waitFor(t, e.postMarker)
}

// Release zips built by Windows PowerShell 5.1's Compress-Archive name their entries with
// backslashes and include directory entries ("lib\auto\"); every Windows release up to
// 2026-09-30 is one. Unpacking those used to turn "auto" into an empty file and then fail.
func TestUnzipHandlesBackslashEntryNames(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "release.zip")
	f, err := os.Create(src)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, e := range []struct{ name, content string }{
		{`lomod.exe`, "bin"},
		{`exiftool_files\exiftool.pl`, "pl"},
		{`exiftool_files\lib\auto\`, ""},
		{`exiftool_files\lib\autouse.pm`, "pm"},
		{`exiftool_files\lib\auto\Compress\`, ""},
		{`exiftool_files\lib\auto\Compress\Raw.dll`, "dll"},
		{`exiftool_files\empty\`, ""},
	} {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(e.content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(dir, "out")
	if err := unzip(src, dst); err != nil {
		t.Fatalf("unzip failed: %v", err)
	}
	for path, want := range map[string]string{
		"lomod.exe":                                "bin",
		"exiftool_files/exiftool.pl":               "pl",
		"exiftool_files/lib/autouse.pm":            "pm",
		"exiftool_files/lib/auto/Compress/Raw.dll": "dll",
	} {
		if got := readFile(t, filepath.Join(dst, filepath.FromSlash(path))); got != want {
			t.Errorf("%s = %q, want %q", path, got, want)
		}
	}
	for _, d := range []string{"exiftool_files/lib/auto", "exiftool_files/empty"} {
		if fi, err := os.Stat(filepath.Join(dst, filepath.FromSlash(d))); err != nil || !fi.IsDir() {
			t.Errorf("%s is not a directory (err=%v)", d, err)
		}
	}
}

func TestUpgradeSkipsWhenVersionIsCurrent(t *testing.T) {
	e := newEnv(t)
	if err := e.run(newVersion); err != nil {
		t.Fatalf("no-op run failed: %v", err)
	}
	e.assertOldReleaseIntact()
	if exists(e.preMarker) {
		t.Error("pre command ran (stopping the app) although there was nothing to update")
	}
}

func TestUpgradeRejectsWrongSHA256(t *testing.T) {
	e := newEnv(t)
	e.sha = "deadbeef"
	if err := e.run(oldVersion); err == nil {
		t.Fatal("upgrade succeeded with a SHA256 mismatch")
	}
	e.assertOldReleaseIntact()
	if exists(e.preMarker) {
		t.Error("pre command ran (stopping the app) although the download was rejected")
	}
}

// The regression this guards: a process with its working directory inside the app dir (the
// tray icon, started from a shortcut whose "Start in" was the install dir) makes Windows
// refuse the rename-swap. lomoupg used to print "upgrade fail..." and exit 0 anyway.
func TestUpgradeReportsBlockedSwap(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("only Windows refuses to rename a directory that is a process's working directory")
	}
	fastRetries(t)
	e := newEnv(t)

	holder := exec.Command("cmd.exe", "/c", "ping -n 60 127.0.0.1 >nul")
	holder.Dir = e.appDir
	if err := holder.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprint(holder.Process.Pid)).Run()
		holder.Wait()
	}()

	if err := e.run(oldVersion); err == nil {
		t.Fatal("upgrade reported success although the app dir could not be swapped")
	}
	e.assertOldReleaseIntact()
	// The pre command stopped the app, so the post command must still bring it back.
	waitFor(t, e.postMarker)
}

// Once whatever held the directory goes away mid-run (a just-killed process releasing its
// handles), the retry picks the swap up instead of failing the whole update.
func TestUpgradeRetriesUntilDirIsReleased(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("only Windows refuses to rename a directory that is a process's working directory")
	}
	e := newEnv(t)

	holder := exec.Command("cmd.exe", "/c", "ping -n 3 127.0.0.1 >nul")
	holder.Dir = e.appDir
	if err := holder.Start(); err != nil {
		t.Fatal(err)
	}
	defer holder.Wait()

	if err := e.run(oldVersion); err != nil {
		t.Fatalf("upgrade failed although the app dir was released within the retry window: %v", err)
	}
	if got := readFile(t, filepath.Join(e.appDir, "release.txt")); got != "new" {
		t.Errorf("app dir release.txt = %q, want new", got)
	}
}

func TestUpgradeRestoresOldReleaseWhenNewOneCannotMoveIn(t *testing.T) {
	fastRetries(t)
	e := newEnv(t)

	err := upgrade(e.appDir, e.backupDir, filepath.Join(e.backupDir, "does-not-exist"))
	if err == nil {
		t.Fatal("upgrade succeeded without a new release to move in")
	}
	e.assertOldReleaseIntact()
}

func TestIsNewerBuild(t *testing.T) {
	for _, c := range []struct {
		candidate, current string
		newer, comparable  bool
	}{
		{newVersion, oldVersion, true, true},
		{oldVersion, newVersion, false, true},
		// Same build time, different commit: not newer.
		{"2026-02-02.00-00-00.0.ccccccc", newVersion, false, true},
		// One second later.
		{"2026-02-02.00-00-01.0.aaaaaaa", newVersion, true, true},
		{newVersion, "unknown", false, false},
		{"1.2.3", oldVersion, false, false},
		{newVersion, "", false, false},
	} {
		newer, comparable := isNewerBuild(c.candidate, c.current)
		if newer != c.newer || comparable != c.comparable {
			t.Errorf("isNewerBuild(%q, %q) = %v, %v; want %v, %v", c.candidate, c.current, newer, comparable, c.newer, c.comparable)
		}
	}
}

// A machine on a newer build than the manifest's (say, switched from the nightly channel back
// to stable) must stay where it is, not downgrade to a release that may not read its data.
func TestOnlyNewerSkipsOlderRelease(t *testing.T) {
	e := newEnv(t)
	if err := e.run("2026-03-03.00-00-00.0.ccccccc", "--only-newer"); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	e.assertOldReleaseIntact()
	if exists(e.preMarker) {
		t.Error("pre command ran (stopping the app) although the release was older")
	}
}

func TestOnlyNewerStillInstallsNewerRelease(t *testing.T) {
	e := newEnv(t)
	if err := e.run(oldVersion, "-only-newer"); err != nil {
		t.Fatalf("upgrade failed: %v", err)
	}
	e.assertSwapped()
}

// An install whose version can't be read as a build time (a damaged version.txt) has to keep
// updating, or it would be stuck for good.
func TestOnlyNewerInstallsWhenCurrentVersionIsUnknown(t *testing.T) {
	e := newEnv(t)
	if err := e.run("unknown", "--only-newer"); err != nil {
		t.Fatalf("upgrade failed: %v", err)
	}
	e.assertSwapped()
}

// Without the flag, behavior is what LomoAgent and older update scripts rely on: any other
// version is installed, older included.
func TestWithoutOnlyNewerOlderReleaseIsInstalled(t *testing.T) {
	e := newEnv(t)
	if err := e.run("2026-03-03.00-00-00.0.ccccccc"); err != nil {
		t.Fatalf("upgrade failed: %v", err)
	}
	e.assertSwapped()
}
