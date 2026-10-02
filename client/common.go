package client

import (
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/pkg/errors"
)

func makeStatusError(body io.ReadCloser) error {
	defer body.Close()
	content, err := ioutil.ReadAll(body)
	if err != nil {
		return err
	}
	return errors.New(strings.TrimSpace(string(content)))
}

func getHeaders(token string) http.Header {
	if token == "" {
		return nil
	}
	return http.Header{"Authorization": []string{"token=" + token}}
}

func request(method, url string, headers http.Header, buf io.Reader) (io.ReadCloser, error) {
	resp, err := requestReply(method, url, headers, buf)
	if err != nil {
		return nil, err
	}

	switch resp.StatusCode {
	case http.StatusOK:
		return resp.Body, nil
	default:
		defer resp.Body.Close()
		return nil, makeStatusError(resp.Body)
	}
}

func requestReply(method, url string, headers http.Header, buf io.Reader) (*http.Response, error) {
	cli := &http.Client{}
	req, err := http.NewRequest(method, "http://"+url, buf)
	if err != nil {
		return nil, err
	}
	req.Header = headers

	return cli.Do(req)
}

func checkAndReturnReply(resp *http.Response) error {
	if resp.StatusCode == http.StatusOK {
		return nil
	} else if resp.StatusCode == http.StatusBadRequest ||
		resp.StatusCode == http.StatusUnauthorized ||
		resp.StatusCode == http.StatusNotFound ||
		resp.StatusCode >= http.StatusInternalServerError {
		return makeStatusError(resp.Body)
	}
	return errors.Errorf("unsupported error code: %d", resp.StatusCode)
}
