package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"path"

	"bitbucket.org/lomoware/lomo-backend/common"
)

// FileType indicates the type of file, either directory or plain file
// TBD: use string to enumerate all types???
type FileType int

const (
	// Dir indicates file type is director
	Dir = iota
	// File indicates file type is plain file
	File
)

// FileInfo is the structure for local file
type FileInfo struct {
	Name string
	Type FileType
}

// ListResponse is the response structure for list request
type ListResponse struct {
	Files []FileInfo
}

type browseHandler struct {
	baseDir string
}

// StaticFileServer serves static file with json structure
func StaticFileServer(baseDir string) http.Handler {
	return &browseHandler{baseDir: baseDir}
}

func (b *browseHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// PUT is used to create a folder
	// POST is used to create new content
	if r.Method != "GET" {
		common.WriteError(w, common.ErrBadRequest)
	}
	dir := path.Join(b.baseDir, r.URL.Path)

	fi, err := os.Lstat(dir)
	if err != nil {
		common.WriteError(w, err)
		return
	}

	if fi.Mode().IsDir() {
		b.handleDirGet(w, dir)
	} else {
		b.handleFileGet(w, r, dir)
	}
}

func (b *browseHandler) handleFileGet(w http.ResponseWriter, r *http.Request, filename string) {
	http.ServeFile(w, r, filename)
}

func (b *browseHandler) handleDirGet(w http.ResponseWriter, dir string) {
	fileinfos, err := ioutil.ReadDir(dir)
	if err != nil {
		common.WriteError(w, err)
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
		common.WriteError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(resp)
}

func isHidden(file os.FileInfo) bool {
	return file.Name()[0:1] == "."
}

func getFileType(name string) FileType {
	return File
}
