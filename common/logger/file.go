package logger

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"sync"

	"github.com/sirupsen/logrus"
)

var gzPool = sync.Pool{
	New: func() interface{} {
		w := gzip.NewWriter(ioutil.Discard)
		return w
	},
}

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w *gzipResponseWriter) WriteHeader(status int) {
	w.Header().Del("Content-Length")
	w.ResponseWriter.WriteHeader(status)
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

// WriteHTTP downloads the file at given directory
func WriteHTTP(files []string, w http.ResponseWriter) error {
	// use general octect-stream to avoid client decompressing the file
	w.Header().Set("Content-Type", "application/octet-stream")

	gz := gzPool.Get().(*gzip.Writer)
	defer gzPool.Put(gz)

	gz.Reset(w)
	defer gz.Close()

	grw := &gzipResponseWriter{ResponseWriter: w, Writer: gz}

	tw := tar.NewWriter(grw)
	defer tw.Close()
	for _, f := range files {
		if err := writeTar(f, tw); err != nil {
			return err
		}
	}
	return nil
}

func writeTar(f string, tw *tar.Writer) error {
	logrus.Infof("Downloaing log file: %s", f)
	if _, err := os.Stat(f); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	body, err := ioutil.ReadFile(f)
	if err != nil {
		return err
	}

	hdr := &tar.Header{
		Name: path.Base(f),
		Mode: 0600,
		Size: int64(len(body)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err = tw.Write(body)
	return err
}
