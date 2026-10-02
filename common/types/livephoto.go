package types

import (
	"archive/zip"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var jsonErrRegex = regexp.MustCompile("unexpected end of JSON input")

// LivePhotoHash is the hash for live photo
type LivePhotoHash struct {
	ImageSHA1 string `json:"image_sha1"`
	VideoSHA1 string `json:"video_sha1"`
	TotalSHA1 string `json:"total_sha1"`
}

// GetLivePhotoFileSHA read sha by trying comment firstly, and content if no comment
func GetLivePhotoFileSHA(filename string) (*LivePhotoHash, error) {
	h, err := GetLivePhotoFileSHAByComments(filename)
	if err == nil {
		return h, nil
	}
	if !jsonErrRegex.MatchString(err.Error()) {
		return nil, err
	}
	logrus.Warnf("%s doesn't use comment to specify hash", filename)
	return GetLivePhotoFileSHAByContent(filename, func(name string, imgSHA hash.Hash) (io.Writer, error) {
		return imgSHA, nil
	})
}

// GetLivePhotoFileSHAByComments read sha1 of given live photo file from its comment
func GetLivePhotoFileSHAByComments(filename string) (*LivePhotoHash, error) {
	hash := &LivePhotoHash{}

	zr, err := zip.OpenReader(filename)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	err = json.Unmarshal([]byte(zr.Comment), hash)
	return hash, err
}

// GetLivePhotoFileSHAByContent calculates sha1 of given live photo file by uncompress the file and calculate sha based on content
func GetLivePhotoFileSHAByContent(filename string, imgHandler func(name string, imgSHA hash.Hash) (io.Writer, error)) (*LivePhotoHash, error) {
	zr, err := zip.OpenReader(filename)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	imgSHA := sha1.New()
	videoSHA := sha1.New()
	defer func() {
		// set sha to empty for GC
		imgSHA.Reset()
		imgSHA = nil
		videoSHA.Reset()
		videoSHA = nil
	}()
	for _, f := range zr.File {
		var (
			retErr error
			w      io.Writer
		)
		switch strings.ToLower(filepath.Ext(f.Name)) {
		case ".jpeg":
			fallthrough
		case ".jpg":
			fallthrough
		case ".heic":
			fallthrough
		case ".heif":
			w, err = imgHandler(f.Name, imgSHA)
			if err != nil {
				return nil, err
			}
		case ".mov":
			fallthrough
		case ".mp4":
			w = videoSHA
		default:
			continue
		}

		fr, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer fr.Close()

		if _, err := io.Copy(w, fr); err != nil {
			retErr = err
		}
		if err := fr.Close(); err != nil {
			if retErr != nil {
				retErr = errors.Wrapf(err, "has previous err: %s", retErr.Error())
			} else {
				retErr = err
			}
		}
		wc, ok := w.(io.WriteCloser)
		if ok {
			if err := wc.Close(); err != nil {
				if retErr != nil {
					retErr = errors.Wrapf(err, "has previous err: %s", retErr.Error())
				} else {
					retErr = err
				}
			}
		}
		if retErr != nil {
			return nil, retErr
		}
	}

	hash := &LivePhotoHash{}
	hash.ImageSHA1 = fmt.Sprintf("%x", imgSHA.Sum(nil))
	hash.VideoSHA1 = fmt.Sprintf("%x", videoSHA.Sum(nil))
	hash.TotalSHA1 = fmt.Sprintf("%x", sha1.Sum([]byte(hash.ImageSHA1+hash.VideoSHA1)))
	return hash, nil
}
