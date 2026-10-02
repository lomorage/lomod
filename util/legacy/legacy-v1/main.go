package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/user"
	"path"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

// FileType indicates the type of file, either directory or plain file
// TBD: use string to enumerate all types???
type FileType int

const (
	// Dir indicates file type is director.
	Dir = iota
	// File indicates file type is plain file.
	File
)

// FileInfo is the structure for local file.
type FileInfo struct {
	Name string
	Type FileType
}

// ListResponse is the response structure for list request.
type ListResponse struct {
	Files []FileInfo
}

type fsHandler struct {
	baseDir string
}

func main() {
	app := cli.NewApp()

	app.Version = "0.0.1"
	app.Usage = "access remote file system over http"
	app.Email = "support@lomorage.com"

	u, err := user.Current()
	if err != nil {
		logrus.Fatal(err)
		os.Exit(1)
	}
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "base, b",
			Value: u.HomeDir,
		},
		cli.UintFlag{
			Name:  "port, p",
			Value: 8000,
		},
	}

	app.Action = bootService

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
		os.Exit(1)
	}
}

func bootService(ctx *cli.Context) {
	if err := http.ListenAndServe(
		fmt.Sprintf(":%d", ctx.Uint("port")),
		&fsHandler{baseDir: ctx.String("base")}); err != nil {
		logrus.Fatal(err)
		os.Exit(1)
	}
}

func isHidden(file os.FileInfo) bool {
	return file.Name()[0:1] == "."
}

func getFileType(name string) FileType {
	return File
}

func writeError(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), 500)
}

func writeErrorCode(w http.ResponseWriter, err int) {
	w.WriteHeader(err)
}

func validatePath(w http.ResponseWriter, pa string) bool {
	if _, err := os.Stat(pa); !os.IsNotExist(err) {
		if err != nil {
			writeError(w, err)
		} else {
			writeErrorCode(w, 400)
		}
		return false
	}
	return true
}

// ServeHTTP serves http request
func (h *fsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// PUT is used to create a folder
	// POST is used to create new content
	switch r.Method {
	case "GET":
		h.handleGet(w, r)
	case "PUT":
		h.handlePut(w, r)
	case "POST":
		h.handlePost(w, r)
	case "DELETE":
		h.handleDelete(w, r)
	default:
		fmt.Fprintf(w, "hello, method %s is not supported\n", r.Method)
	}
}

func (h *fsHandler) handlePut(w http.ResponseWriter, r *http.Request) {
	dir := path.Join(h.baseDir, r.URL.Path)

	if !validatePath(w, dir) {
		return
	}

	if err := os.Mkdir(dir, 0700); err != nil {
		writeError(w, err)
	}
}

func (h *fsHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	dir := path.Join(h.baseDir, r.URL.Path)

	if err := os.RemoveAll(dir); err != nil {
		writeError(w, err)
	}
}

func (h *fsHandler) handlePost(w http.ResponseWriter, r *http.Request) {
	pa := path.Join(h.baseDir, r.URL.Path)

	logrus.Infof("upload file : %s", pa)
	if !validatePath(w, pa) {
		return
	}

	f, err := os.OpenFile(pa, os.O_WRONLY|os.O_CREATE, 0700)
	if err != nil {
		writeError(w, err)
		return
	}
	defer f.Close()

	body := r.Body
	defer body.Close()

	size, err := io.Copy(f, body)
	if err != nil {
		writeError(w, err)
	}

	logrus.Infof("upload file size: %d", size)
}

func (h *fsHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	dir := path.Join(h.baseDir, r.URL.Path)

	fi, err := os.Lstat(dir)
	if err != nil {
		writeError(w, err)
		return
	}

	if fi.Mode().IsDir() {
		h.handleDirGet(w, dir)
	} else {
		h.handleFileGet(w, dir)
	}
}

func (h *fsHandler) handleFileGet(w http.ResponseWriter, filename string) {
	file, err := ioutil.ReadFile(filename)
	if err != nil {
		writeError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(file)
}

func (h *fsHandler) handleDirGet(w http.ResponseWriter, dir string) {
	fileinfos, err := ioutil.ReadDir(dir)
	if err != nil {
		writeError(w, err)
		return
	}

	len := 10
	i := 0
	files := make([]FileInfo, len)
	for _, v := range fileinfos {
		// skip hidden file
		if isHidden(v) {
			continue
		}

		file := FileInfo{}
		file.Name = v.Name()

		if v.IsDir() {
			file.Type = Dir
		} else {
			file.Type = getFileType(path.Join(dir, file.Name))
		}
		if i < len {
			files[i] = file
		} else {
			files = append(files, file)
		}
		i++
	}

	resp, err := json.Marshal(ListResponse{Files: files[:i]})
	if err != nil {
		writeError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(resp)
}
