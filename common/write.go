package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

//ErrResponse is the error structure
type ErrResponse struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// WriteBody writes body in json format
func WriteBody(w http.ResponseWriter, v interface{}) {
	if v == nil {
		w.Header().Add("Content-Length", "0")
		return
	}
	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(v); err != nil {
		WriteError(w, err)
		return
	}
	w.Header().Add("Content-Length", strconv.Itoa(buf.Len()))
	w.Write(buf.Bytes())
}

// WriteError writes error data
func WriteError(w http.ResponseWriter, err error, args ...interface{}) {
	retErr := err
	if len(args) > 0 {
		retErr = errors.Wrapf(err, "%v", args)
	}
	logrus.Errorf("http reply: %s", retErr)

	// http.Error add one new line, so need add 1 for content length
	resp := ""
	e, ok := errMaps[err]
	if ok {
		resp = fmt.Sprintf("{\"id\": \"%d\", \"text\": \"%s\"}", e.id, retErr.Error())
	} else {
		resp = fmt.Sprintf("{\"id\": \"%d\", \"text\": \"%s\"}", 0, retErr.Error())
	}

	w.Header().Add("Content-Length", strconv.Itoa(len(resp)+1))
	http.Error(w, resp, errToHTTPStatus(err))
}

// MakeStatusError returns an appropriate error created from the HTTP response.
func MakeStatusError(body io.ReadCloser) error {
	defer body.Close()
	content, err := ioutil.ReadAll(body)
	if err != nil {
		return err
	}
	return errors.New(strings.TrimSpace(string(content)))
}
