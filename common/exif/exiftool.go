package exif

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/cmd"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	// UnknownGPS means the gps location is not recognized
	UnknownGPS = float64(888)
	// Orientation is tag name for orientation
	Orientation = "Orientation"
	// Rotation is tag name for Rotation
	Rotation = "Rotation"
)

// tagJSON is EXIF json file format.
type tagJSON struct {
	// file info related tags
	FileModifyDate      string `json:"FileModifyDate"`
	FileInodeChangeDate string `json:"FileInodeChangeDate"`
	FileAccessDate      string `json:"FileAccessDate"`

	// common tags
	CreateDate string `json:"CreateDate"`
	ModifyDate string `json:"ModifyDate"`

	// make and model
	Make  string `json:"Make"`
	Model string `json:"Model"`

	ImageWidth  int `json:"ImageWidth"`
	ImageHeight int `json:"ImageHeight"`

	// image tags
	DateTimeOriginal string `json:"DateTimeOriginal"`
	GPSDateTime      string `json:"GPSDateTime"`

	// video tags
	TrackCreateDate   string `json:"TrackCreateDate"`
	TrackModifyDate   string `json:"TrackModifyDate"`
	MediaCreateDate   string `json:"MediaCreateDate"`
	MediaModifyDate   string `json:"MediaModifyDate"`
	ContentCreateDate string `json:"ContentCreateDate"`

	Comment string `json:"Comment"`

	// gps tags
	GPSLatitude  interface{} `json:"GPSLatitude,omitempty"`
	GPSLongitude interface{} `json:"GPSLongitude,omitempty"`
	GPSAltitude  interface{} `json:"GPSAltitude,omitempty"`

	// misc tags
	Rotation    *int `json:"Rotation,omitempty"`
	Orientation *int `json:"Orientation,omitempty"`

	// lomorage
	ComLomoOrigSHA string `json:"ComLomorageOriginhash"`
	LomoOrigSHA    string `json:"LomorageOriginhash"`
}

// Tags is extracted tags by exiftool.
type Tags struct {
	filename   string
	extension  string
	createTime time.Time
	tags       []tagJSON
	RawTags    RawTags
}

// NewTags use exiftool to extract all necessary Tags.
func NewTags(filename, exiftool, ext string) (*Tags, error) {
	tmpdir, err := ioutil.TempDir("", "exif")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpdir)

	// need add %c, otherwise, exiftool will fail
	jsonfile := filepath.Join(tmpdir, "output")
	c := exec.Command(exiftool, "-n", "-c", "'%.10f'", "-j", "-w", jsonfile+"%c.json", filename)
	stdoutStderr, err := cmd.CombinedOutputLowPriority(c)
	if err != nil {
		return nil, errors.Wrapf(err, "exiftool analysis %s got %s", filename, string(stdoutStderr))
	}

	content, err := ioutil.ReadFile(jsonfile + ".json")
	if err != nil {
		return nil, err
	}

	ts := &Tags{filename: filename, extension: ext, RawTags: RawTags{}}
	if err := json.Unmarshal(content, &ts.tags); err != nil {
		return nil, errors.Wrap(err, string(content))
	}
	return ts, json.Unmarshal(content, &ts.RawTags)
}

// Filename returns filename.
func (ts *Tags) Filename() string {
	return ts.filename
}

// SetExtension set extension in case no extension in filename.
func (ts *Tags) SetExtension(e string) {
	ts.extension = e
}

func (ts *Tags) getTimeByEXIFTool() (t time.Time, err error) {
	name := ts.filename
	if filepath.Ext(name) == "" {
		if ts.extension == "" {
			return time.Time{}, errors.New("no correct extension")
		}
		name += "." + ts.extension
	}
	if ext.IsImageFile(name) {
		for _, tag := range ts.tags {
			t, err = time.Parse(common.TimeFormatEXIF, tag.DateTimeOriginal)
			if err == nil && !t.IsZero() {
				return
			}
			t, err = time.Parse(common.TimeFormatEXIF, tag.CreateDate)
			if err == nil && !t.IsZero() {
				return
			}
			t, err = time.Parse(common.TimeFormatEXIF, tag.GPSDateTime)
			if err == nil && !t.IsZero() {
				return
			}
		}
		// skip marshal error
		rawContent, _ := json.Marshal(ts.RawTags)
		return t, errors.Errorf("image %s (%s) doesn't have valid time tag: %s", ts.filename, ts.extension, rawContent)
	}
	for _, tag := range ts.tags {
		t, err = time.Parse(common.TimeFormatEXIF, tag.CreateDate)
		if err == nil && !t.IsZero() {
			return
		}
		t, err = time.Parse(common.TimeFormatEXIF2, tag.ContentCreateDate)
		if err == nil && !t.IsZero() {
			return
		}
		t, err = time.Parse(common.TimeFormatEXIF, tag.MediaCreateDate)
		if err == nil && !t.IsZero() {
			return
		}
		t, err = time.Parse(common.TimeFormatEXIF, tag.TrackCreateDate)
		if err == nil && !t.IsZero() {
			return
		}
	}
	// skip marshal error
	rawContent, _ := json.Marshal(ts.RawTags)
	return t, errors.Errorf("video %s (%s) doesn't have valid time tag: %s", ts.filename, ts.extension, rawContent)
}

// GetWidth return width
func (ts *Tags) GetWidth() int {
	for _, tag := range ts.tags {
		if tag.ImageWidth != 0 {
			return tag.ImageWidth
		}
	}
	return 0
}

// GetHeight return height
func (ts *Tags) GetHeight() int {
	for _, tag := range ts.tags {
		if tag.ImageHeight != 0 {
			return tag.ImageHeight
		}
	}
	return 0
}

// GetComment read media file's comment by exiftool.
func (ts *Tags) GetComment() string {
	for _, tag := range ts.tags {
		if tag.Comment != "" {
			return tag.Comment
		}
	}
	return ""
}

// GetLomoOriginSHA return asset's origin sha marked by lomorage.
func (ts *Tags) GetLomoOriginSHA() string {
	for _, tag := range ts.tags {
		if tag.LomoOrigSHA != "" {
			return tag.LomoOrigSHA
		} else if tag.ComLomoOrigSHA != "" {
			return tag.ComLomoOrigSHA
		}
	}
	return ""
}

func (ts *Tags) parseGPS(v interface{}) float64 {
	f, ok := v.(float64)
	if ok {
		return f
	}
	fp, ok := v.(*float64)
	if ok {
		return *fp
	}
	return UnknownGPS
}

// GetLatitude returns asset's latitude.
func (ts *Tags) GetLatitude() float64 {
	for _, tag := range ts.tags {
		if tag.GPSLatitude != nil {
			return ts.parseGPS(tag.GPSLatitude)
		}
	}
	return UnknownGPS
}

// GetLongitude returns asset's longitude.
func (ts *Tags) GetLongitude() float64 {
	for _, tag := range ts.tags {
		if tag.GPSLongitude != nil {
			return ts.parseGPS(tag.GPSLongitude)
		}
	}
	return UnknownGPS
}

// SetCreateTime set create time.
func (ts *Tags) SetCreateTime(t time.Time) {
	ts.createTime = t
}

// GetCreateTime return asset's create time.
func (ts *Tags) GetCreateTime() (time.Time, error) {
	if !ts.createTime.IsZero() {
		return ts.createTime, nil
	}

	t, err := ts.getTimeByEXIFTool()
	if err != nil {
		return t, err
	}
	t = t.UTC().Truncate(time.Second)
	ts.createTime = t
	return t, nil
}

// GetMake returns camera make
func (ts *Tags) GetMake() string {
	for _, tag := range ts.tags {
		if tag.Make != "" {
			return tag.Make
		}
	}
	return ""
}

// GetModel returns camera model
func (ts *Tags) GetModel() string {
	for _, tag := range ts.tags {
		if tag.Model != "" {
			return tag.Model
		}
	}
	return ""
}

// GetOrientation return asset's orientation
/*
Orientation string meaning in exiftool
1 = Horizontal (normal)
2 = Mirror horizontal
3 = Rotate 180
4 = Mirror vertical
5 = Mirror horizontal and rotate 270 CW
6 = Rotate 90 CW
7 = Mirror horizontal and rotate 90 CW
8 = Rotate 270 CW
*/
// -1 means no orientation
// true means returned value is orientation, otherwise return rotation.
func (ts *Tags) GetOrientation() (string, int) {
	// probe Orietation, if not exist, try rotation
	for _, tag := range ts.tags {
		if tag.Orientation != nil {
			return Orientation, *tag.Orientation
		} else if tag.Rotation != nil {
			return Rotation, *tag.Rotation
		}
	}
	return "", 0
}

// SetOrientation set orientation by using name value.
func SetOrientation(filename, exiftool, tagName string, orientation int) error {
	switch tagName {
	case Orientation:
	case Rotation:
	default:
		return nil
	}
	if orientation == 0 {
		logrus.Warnf("invalid orientation %s: %s,%d", filename, tagName, orientation)
		return nil
	} else if orientation == 1 {
		logrus.Warnf("unchanged orientation %s: %s,%d", filename, tagName, orientation)
		return nil
	}
	logrus.Debugf("Set orientation %s: %s,%d", filename, tagName, orientation)
	return common.RetryIfKnownError(fmt.Sprintf("while set orientation for %s", filename), func() error {
		return cmd.Exec(exiftool, "-overwrite_original", fmt.Sprintf("-%s=%d", tagName, orientation), "-n", filename)
	})
}

// RemoveGPS remove all GPS metadata from given file.
func RemoveGPS(filename, exiftool string) error {
	logrus.Debugf("Remove all GPS data from %s", filename)
	return common.RetryIfKnownError(fmt.Sprintf("while removing GPS for %s", filename), func() error {
		return cmd.Exec(exiftool, "-overwrite_original", "-gps*=", filename)
	})
}
