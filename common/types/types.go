package types

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"github.com/pkg/errors"
)

const (
	// DeviceType is type of device.
	DeviceType = "DeviceType"
	// DeviceTypeChromecast is chromecast device.
	DeviceTypeChromecast = "chromecast"
	// DeviceUUID is uuid of chromecast device.
	DeviceUUID = "DeviceUUID"
	// DeviceSubType is subtype of the  device.
	// dns txt entry: md=Chromecast
	DeviceSubType = "DeviceSubType"
	// DeviceName is name of device.
	DeviceName = "DeviceName"
	// DeviceIP is ip of device
	DeviceIP = "DeviceIP"
	// DevicePort is port of device
	DevicePort = "DevicePort"

	// MetadataKeyComment is the metadata key for comment
	MetadataKeyComment = "Comment"
)

// LastSavedAsset has information of last saved asset info
type LastSavedAsset struct {
	FinalSHA string
	CurrSHA  string
	CurrSize int64
}

// Valid checks if the content is valid or not
func (lsa *LastSavedAsset) Valid() bool {
	return len(lsa.FinalSHA) == 40 &&
		((lsa.CurrSize == 0 && len(lsa.CurrSHA) == 0) || (lsa.CurrSize != 0 && len(lsa.CurrSHA) == 40))
}

// LomoTime is one wrapper of time.Time to provide customized time format
type LomoTime struct {
	time.Time
}

// ParseDBTime creates LomoTime struct by parsing DB format
func ParseDBTime(t string) (LomoTime, error) {
	dt := LomoTime{}
	for _, f := range common.TimeFormatDBs {
		d, err := time.Parse(f, t)
		if err == nil {
			dt.Time = d.UTC()
			return dt, nil
		}
	}
	return dt, errors.Errorf("unable to parse time %s", t)
}

// MarshalJSON is implementation of Marshaler interface
func (t LomoTime) MarshalJSON() ([]byte, error) {
	return []byte(`"` + t.UTC().Format(common.TimeFormatLomod) + `"`), nil
}

// UnmarshalJSON is implementation of Unmarshaler interface
func (t *LomoTime) UnmarshalJSON(data []byte) (err error) {
	if len(data) == 0 || string(data) == `""` {
		return nil
	}
	t.Time, err = time.Parse(common.TimeFormatLomod, strings.Trim(string(data), `"`))
	return
}

// String is to format time to string
func (t LomoTime) String() string {
	return t.UTC().Format(common.TimeFormatLomod)
}

// Asset is response structure for each asset
type Asset struct {
	Name      string
	Hash      string
	Device    string
	Status    int
	Longitude float64
	Latitude  float64
	Date      LomoTime
	Metadatas *[]Metadata `json:",omitempty"`
}

// AssetName is structure to store both asset name and hash
type AssetName struct {
	Name string
	Hash string
}

// AssetHash is structure to contain hash only. Used by react-pig
// refer https://github.com/nickmcmillan/react-pig/blob/master/example/src/imageData.json
type AssetHash struct {
	ID          string  `json:"id"`
	Type        string  `json:"type"`
	Hash        string  `json:"image_hash"`
	URL         string  `json:"url"`
	AspectRatio float32 `json:"aspectRatio"`
}

// AssetHashGroup is structure to group by date
type AssetHashGroup struct {
	CreateDate string      `json:"date"`
	Location   string      `json:"location"`
	Assets     []AssetHash `json:"items"`
}

// AssetsByDay is structure to returns assets by date
type AssetsByDay struct {
	ID            string      `json:"id"`
	Date          string      `json:"date"`
	Location      string      `json:"location"`
	Incomplete    bool        `json:"incomplete"`
	NumberOfItems int         `json:"numberOfItems"`
	Assets        []AssetHash `json:"items"`
}

// AssetStatus is asset status such as hidden, public, favorite
type AssetStatus int

const (
	AssetStatusScanLink AssetStatus = iota
	AssetStatusHidden
	AssetStatusPublic
	AssetStatusFavorite
)

// SetAssetStatus set asset status
func SetAssetStatus(n int, status AssetStatus) int {
	n |= 1 << status
	return n
}

// UnsetAssetStatus set asset status
func UnsetAssetStatus(n int, status AssetStatus) int {
	mask := ^(1 << status)
	n &= mask
	return n
}

// IsAssetStatus check if asset's status is set
func IsAssetStatus(n int, status AssetStatus) bool {
	return n&(1<<status) != 0
}

// AssetsEditHidden is to allow client to hide or un-hide asset
type AssetsEditHidden struct {
	AssetIDs []string `json:"image_hashes"`
	Hidden   bool     `json:"hidden"`
}

// AssetsEditFavorite is to allow client to set asset favorite
type AssetsEditFavorite struct {
	AssetIDs []string `json:"image_hashes"`
	Favorite bool     `json:"favorite"`
}

// AssetsEditPublic is to allow client to set asset public status
type AssetsEditPublic struct {
	AssetIDs []string `json:"image_hashes"`
	Public   bool     `json:"val_public"`
}

// AssetsDownload is to allow client to download list of assets
type AssetsDownload struct {
	AssetIDs []string `json:"image_hashes"`
}

// Day is list of all assets at the date, and also total hash
type Day struct {
	Day    int
	Hash   string
	Assets []Asset
}

// Month is list of all assets at the month, and also total hash for each day in this month
type Month struct {
	Month int
	Hash  string
	Days  []Day
}

// Year is list of all assets at one year, and also total hash for each month in these Months
type Year struct {
	Year   int
	Hash   string
	Months []Month
}

// Years is list of years and months in the system
type Years struct {
	Hash  string
	Years []Year
}

// MkAlbumDateID create date id with its format <year>-<month>-<day>
func MkAlbumDateID(y, m, d int) string {
	return fmt.Sprintf("%d-%02d-%02d", y, m, d)
}

// ParseAlbumDateID parse album date ID with format <year>-<month>-<day>
func ParseAlbumDateID(id string) (int, int, int, error) {
	d, err := time.Parse("2006-01-02", id)
	if err != nil {
		return 0, 0, 0, err
	}

	return d.Year(), int(d.Month()), d.Day(), nil
}

// AssetIDType specifies the type of Asset ID in the request
type AssetIDType int

const (
	// Index means asset ID is based on index
	Index AssetIDType = iota
	// Hash means asset ID is based on hash
	Hash
)

// DeleteAssetItems is the structure for delete batch request
type DeleteAssetItems struct {
	List []DeleteAssetItem
}

// DeleteAssetItem is the structure for delete batch request
type DeleteAssetItem struct {
	ID     string
	Type   AssetIDType
	Force  bool
	Result bool
	Reason string
}

// Dimension is the size of image or view
type Dimension struct {
	Width  uint
	Height uint
}

// NewDimensions parse dimension string and return structure
func NewDimensions(dimensions string) ([]Dimension, error) {
	if dimensions == "0" {
		return nil, nil
	}
	dims := []Dimension{}
	parts := strings.Split(dimensions, ";")
	for _, p := range parts {
		var (
			dim Dimension
			err error
		)
		parts2 := strings.Split(p, "x")
		if len(parts2) != 2 {
			return nil, fmt.Errorf("invalid dimension: %s", p)
		}
		d, err := strconv.Atoi(parts2[0])
		if err != nil {
			return nil, errors.Wrapf(err, "while processing dimension: %s", p)
		}
		if d < 0 {
			dim.Width = 0
		} else {
			dim.Width = uint(d)
		}
		h, err := strconv.Atoi(parts2[1])
		if err != nil {
			return nil, errors.Wrapf(err, "while processing dimension: %s", p)
		}
		if h < 0 {
			dim.Height = 0
		} else {
			dim.Height = uint(h)
		}
		dims = append(dims, dim)
	}
	return dims, nil
}

// AssetLabel is label to classify asset.
type AssetLabel struct {
	ID          int
	Label       string
	LabelCN     string
	AssetsCount int
}

// AssetLabelConfidence is label confidence
type AssetLabelConfidence struct {
	ID         int
	Confidence float32
}

// AssetNameConfidence is structure to store both asset name and hash and its label confidence
type AssetNameConfidence struct {
	Name       string
	Hash       string
	Confidence float32
}
