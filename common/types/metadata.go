package types

import (
	"strings"

	"bitbucket.org/lomoware/lomo-backend/common"
	"github.com/pkg/errors"
)

// InvalidID means invalia metdata category or device ID
const InvalidID = -1

const (
	// MetadataCategoryGeo is geo metadata
	MetadataCategoryGeo = "geo"
	// MetadataCategoryScene is scene metadata
	MetadataCategoryScene = "scene"
	// MetadataCategoryFace is face metadata
	MetadataCategoryFace = "face"
	// MetadataCategoryFace is face recognition metadata
	MetadataCategoryRecFace = "recface"
	// MetadataCategoryText is text metadata
	MetadataCategoryText = "text"
	// MetadataCategoryHuman is human metadata
	MetadataCategoryHuman = "human"
	// MetadataCategorySimilarity is similarity metadata
	MetadataCategorySimilarity = "similarity"
	// MetadataCategoryTag is tag metadata provided by user
	MetadataCategoryTag = "tag"
	// MetadataCategoryEncrypt is encrypt metadata provided by user
	MetadataCategoryEncrypt = "encrypt"

	// MetadataLangEn is default metadata language
	MetadataLangEn = "en_US"
)

// this is the ID stored in DB
const (
	metadataCategoryIDGeo = iota
	metadataCategoryIDScene
	metadataCategoryIDFace
	metadataCategoryIDText
	metadataCategoryIDHuman
	metadataCategoryIDSimilarity
	metadataCategoryIDTag
	metadataCategoryIDEncrypt
	metadataCategoryIDRecFace
)

var (
	// AllMetadataCategories is map of all metadata category
	AllMetadataCategories = map[string]int{
		MetadataCategoryGeo:        metadataCategoryIDGeo,
		MetadataCategoryScene:      metadataCategoryIDScene,
		MetadataCategoryFace:       metadataCategoryIDFace,
		MetadataCategoryRecFace:    metadataCategoryIDRecFace,
		MetadataCategoryText:       metadataCategoryIDText,
		MetadataCategoryHuman:      metadataCategoryIDHuman,
		MetadataCategorySimilarity: metadataCategoryIDSimilarity,
		MetadataCategoryTag:        metadataCategoryIDTag,
		MetadataCategoryEncrypt:    metadataCategoryIDEncrypt,
	}
	// AllMetadataCategoryList is list of all metadata category
	AllMetadataCategoryList = []string{
		MetadataCategoryGeo,
		MetadataCategoryScene,
		MetadataCategoryFace,
		MetadataCategoryRecFace,
		MetadataCategoryText,
		MetadataCategoryHuman,
		MetadataCategorySimilarity,
		MetadataCategoryTag,
		MetadataCategoryEncrypt,
	}
	// AllmetadataCategoryIDs is all metadata category IDs
	AllmetadataCategoryIDs = []int{
		metadataCategoryIDGeo, metadataCategoryIDScene, metadataCategoryIDFace, metadataCategoryIDText,
		metadataCategoryIDHuman, metadataCategoryIDSimilarity, metadataCategoryIDTag, metadataCategoryIDEncrypt,
		metadataCategoryIDRecFace,
	}
)

// MetadataCategory is the category of the metadata key
type MetadataCategory string

// NewMetadataCategory create metadata category
func NewMetadataCategory(c string) (MetadataCategory, error) {
	switch c {
	case MetadataCategoryGeo:
		return MetadataCategoryGeo, nil
	case MetadataCategoryScene:
		return MetadataCategoryScene, nil
	case MetadataCategoryFace:
		return MetadataCategoryFace, nil
	case MetadataCategoryRecFace:
		return MetadataCategoryRecFace, nil
	case MetadataCategoryText:
		return MetadataCategoryText, nil
	case MetadataCategoryHuman:
		return MetadataCategoryHuman, nil
	case MetadataCategorySimilarity:
		return MetadataCategorySimilarity, nil
	case MetadataCategoryTag:
		return MetadataCategoryTag, nil
	case MetadataCategoryEncrypt:
		return MetadataCategoryEncrypt, nil
	default:
		return "", common.ErrNotImplementedFormat
	}
}

// NewMetadataCategoryByID create metadata category by id
func NewMetadataCategoryByID(c int) (MetadataCategory, error) {
	switch c {
	case metadataCategoryIDGeo:
		return MetadataCategoryGeo, nil
	case metadataCategoryIDScene:
		return MetadataCategoryScene, nil
	case metadataCategoryIDFace:
		return MetadataCategoryFace, nil
	case metadataCategoryIDRecFace:
		return MetadataCategoryRecFace, nil
	case metadataCategoryIDText:
		return MetadataCategoryText, nil
	case metadataCategoryIDHuman:
		return MetadataCategoryHuman, nil
	case metadataCategoryIDSimilarity:
		return MetadataCategorySimilarity, nil
	case metadataCategoryIDTag:
		return MetadataCategoryTag, nil
	case metadataCategoryIDEncrypt:
		return MetadataCategoryEncrypt, nil
	default:
		return "", common.ErrNotImplementedFormat
	}
}

// ID return the category ID
func (mc MetadataCategory) ID() int {
	switch mc {
	case MetadataCategoryGeo:
		return metadataCategoryIDGeo
	case MetadataCategoryScene:
		return metadataCategoryIDScene
	case MetadataCategoryFace:
		return metadataCategoryIDFace
	case MetadataCategoryRecFace:
		return metadataCategoryIDRecFace
	case MetadataCategoryText:
		return metadataCategoryIDText
	case MetadataCategoryHuman:
		return metadataCategoryIDHuman
	case MetadataCategorySimilarity:
		return metadataCategoryIDSimilarity
	case MetadataCategoryTag:
		return metadataCategoryIDTag
	case MetadataCategoryEncrypt:
		return metadataCategoryIDEncrypt
	}
	return InvalidID
}

// String return the category as string
func (mc MetadataCategory) String() string {
	return string(mc)
}

// ParseMetadataName parses the metadata name based on the convention
func ParseMetadataName(name string) (SourceDevice, MetadataCategory, string, string, error) {
	parts := strings.Split(name, ".")
	if len(parts) != 4 {
		return "", "", "", "", errors.Errorf("invalid metadata name: %s", name)
	}
	device, err := NewSourceDevice(parts[0])
	if err != nil {
		return "", "", "", "", errors.Errorf("invalid source device: %s", name)
	}

	category, err := NewMetadataCategory(parts[1])
	if err != nil {
		return "", "", "", "", errors.Errorf("invalid metadata category: %s", name)

	}
	return device, category, parts[2], parts[3], nil
}

// GeoLevel is level for geolocation
type GeoLevel int

// refer https://docs.mapbox.com/api/search/geocoding/#data-types
const (
	MetadataGeoLevelCountry GeoLevel = iota + 1
	MetadataGeoLevelState            // corresponding to mapboxregion
	MetadataGeoLevelDistrict
	MetadataGeoLevelCity // corresponding to mapbox place
	MetadataGeoLevelLocality
	MetadataGeoLevelNeighborhood
	MetadataGeoLevelStreet
	MetadataGeoLevelSubStreet
	MetadataGeoLevelPOI
	MetadataGeoLevelUnknown = 100
)

// MetadataGeoNames are list of names for geo
var (
	MetadataGeoCityIos      = "ios." + MetadataCategoryGeo + ".city"
	MetadataGeoCityAndroid  = "android." + MetadataCategoryGeo + ".city"
	MetadataGeoCountry      = "country"
	MetadataGeoState        = "state"
	MetadataGeoDistrict     = "district"
	MetadataGeoCity         = "city"
	MetadataGeoLocality     = "locality"
	MetadataGeoNeighborhood = "neighborhood"
	MetadataGeoStreet       = "street"
	MetadataGeoSubStreet    = "substreet"
	MetadataGeoPOI          = "place"
	MetadataGeoMail         = "mail"
	MetadataGeoZipcode      = "zipcode"
)

// GeoLocation is structure for geo location
type GeoLocation struct {
	ID           int
	Level        GeoLevel
	Country      string
	State        string
	District     string
	City         string
	Locality     string
	Neighborhood string
	Street       string
	SubStreet    string
	POI          string
	Mail         string
	Zipcode      string
	Longitude    float32
	Latitude     float32
}

// Set check key name and set corresponding property
func (g *GeoLocation) Set(k, v string) error {
	switch k {
	case MetadataGeoCountry:
		g.Country = v
	case MetadataGeoState:
		g.State = v
	case MetadataGeoDistrict:
		g.District = v
	case MetadataGeoCity:
		g.City = v
	case MetadataGeoLocality:
		g.Locality = v
	case MetadataGeoNeighborhood:
		g.Neighborhood = v
	case MetadataGeoStreet:
		g.Street = v
	case MetadataGeoSubStreet:
		g.SubStreet = v
	case MetadataGeoPOI:
		g.POI = v
	case MetadataGeoMail:
		g.Mail = v
	case MetadataGeoZipcode:
		g.Zipcode = v
	default:
		return errors.Errorf("unrecognized geo location name: %s", k)
	}

	return nil
}

// NameByLevel return location's name based on its level
func (g *GeoLocation) NameByLevel() string {
	value := ""
	switch g.Level {
	case MetadataGeoLevelCountry:
		value = g.Country
	case MetadataGeoLevelState:
		value = g.State
	case MetadataGeoLevelDistrict:
		value = g.District
	case MetadataGeoLevelCity:
		value = g.City
	case MetadataGeoLevelLocality:
		value = g.Locality
	case MetadataGeoLevelNeighborhood:
		value = g.Neighborhood
	case MetadataGeoLevelStreet:
		value = g.Street
	case MetadataGeoLevelSubStreet:
		value = g.SubStreet
	case MetadataGeoLevelPOI:
		value = g.POI
	}
	return value
}

// GeoMetadataToLevel returns corresponding geo level
func GeoMetadataToLevel(data string) GeoLevel {
	switch data {
	case MetadataGeoCountry:
		return MetadataGeoLevelCountry
	case MetadataGeoState:
		return MetadataGeoLevelState
	case MetadataGeoDistrict:
		return MetadataGeoLevelDistrict
	case MetadataGeoCity:
		return MetadataGeoLevelCity
	case MetadataGeoLocality:
		return MetadataGeoLevelLocality
	case MetadataGeoNeighborhood:
		return MetadataGeoLevelNeighborhood
	case MetadataGeoStreet:
		return MetadataGeoLevelStreet
	case MetadataGeoSubStreet:
		return MetadataGeoLevelSubStreet
	case MetadataGeoPOI:
		return MetadataGeoLevelPOI
	}
	return MetadataGeoLevelUnknown
}

var (
	// MetadataSceneLabel is scene classification label
	MetadataSceneLabel = "vision.classify.label"
	// MetadataSceneProbability is scene classification probability
	MetadataSceneProbability = "vision.classify.confi"
)

const (
	// SourceDeviceIos means the metadata is generated from ios
	SourceDeviceIos SourceDevice = "ios"
	// SourceDeviceAndroid means the metadata is generated from android
	SourceDeviceAndroid SourceDevice = "android"
	// SourceDeviceMac means the metadata is generated from mac OS
	SourceDeviceMac SourceDevice = "mac"
	// SourceDeviceWindows means the metadata is generated from windows os
	SourceDeviceWindows SourceDevice = "windows"
	// SourceDeviceWeb means the metadata is generated from web
	SourceDeviceWeb SourceDevice = "web"
	// SourceDeviceLinux means the metadata is generated from linux
	SourceDeviceLinux SourceDevice = "linux"
)

const (
	sourceDeviceIDIos = iota
	sourceDeviceIDAndroid
	sourceDeviceIDMac
	sourceDeviceIDWindows
	sourceDeviceIDWeb
	sourceDeviceIDLinux
)

// SourceDevice is type of device generate the metadata
type SourceDevice string

// NewSourceDevice create metadata soure device by id
func NewSourceDevice(sd string) (SourceDevice, error) {
	switch sd {
	case string(SourceDeviceIos):
		return SourceDeviceIos, nil
	case string(SourceDeviceAndroid):
		return SourceDeviceAndroid, nil
	case string(SourceDeviceMac):
		return SourceDeviceMac, nil
	case string(SourceDeviceWindows):
		return SourceDeviceWindows, nil
	case string(SourceDeviceWeb):
		return SourceDeviceWeb, nil
	case string(SourceDeviceLinux):
		return SourceDeviceLinux, nil
	default:
		return "", common.ErrNotImplementedFormat
	}
}

// NewSourceDeviceByID create source device by id
func NewSourceDeviceByID(c int) (SourceDevice, error) {
	switch c {
	case sourceDeviceIDIos:
		return SourceDeviceIos, nil
	case sourceDeviceIDAndroid:
		return SourceDeviceAndroid, nil
	case sourceDeviceIDMac:
		return SourceDeviceMac, nil
	case sourceDeviceIDWindows:
		return SourceDeviceWindows, nil
	case sourceDeviceIDWeb:
		return SourceDeviceWeb, nil
	case sourceDeviceIDLinux:
		return SourceDeviceLinux, nil
	default:
		return "", common.ErrNotImplementedFormat
	}
}

// ID return the category ID
func (dt SourceDevice) ID() int {
	switch dt {
	case SourceDeviceIos:
		return sourceDeviceIDIos
	case SourceDeviceAndroid:
		return sourceDeviceIDAndroid
	case SourceDeviceMac:
		return sourceDeviceIDMac
	case SourceDeviceWindows:
		return sourceDeviceIDWindows
	case SourceDeviceWeb:
		return sourceDeviceIDWeb
	case SourceDeviceLinux:
		return sourceDeviceIDLinux
	}
	return InvalidID
}

// Metadata is the structure for the asset
type Metadata struct {
	Category         MetadataCategory
	SourceDevice     SourceDevice
	AssetID          int
	Name             string
	Value            string
	Model            string
	Version          int
	CreateTime       LomoTime
	LastModifiedTime LomoTime
}

// Validate check if category or source device is valid or not
func (m Metadata) Validate() bool {
	if m.Category.ID() == InvalidID {
		return false
	}
	if m.SourceDevice.ID() == InvalidID {
		return false
	}
	return true
}

// ByLevel sort GeoLocation struct by its level and name
type ByLevel []GeoLocation

func (b ByLevel) Len() int      { return len(b) }
func (b ByLevel) Swap(i, j int) { b[i], b[j] = b[j], b[i] }
func (b ByLevel) Less(i, j int) bool {
	if b[i].Level < b[j].Level {
		return true
	} else if b[i].Level > b[j].Level {
		return false
	}
	if b[i].Country < b[j].Country {
		return true
	} else if b[i].Country > b[j].Country {
		return false
	}
	if b[i].Country < b[j].Country {
		return true
	} else if b[i].Country > b[j].Country {
		return false
	}
	if b[i].State < b[j].State {
		return true
	} else if b[i].State > b[j].State {
		return false
	}
	if b[i].District < b[j].District {
		return true
	} else if b[i].District > b[j].District {
		return false
	}
	if b[i].City < b[j].City {
		return true
	} else if b[i].City > b[j].City {
		return false
	}
	if b[i].Locality < b[j].Locality {
		return true
	} else if b[i].Locality > b[j].Locality {
		return false
	}
	if b[i].Neighborhood < b[j].Neighborhood {
		return true
	} else if b[i].Neighborhood > b[j].Neighborhood {
		return false
	}
	if b[i].Street < b[j].Street {
		return true
	} else if b[i].Street > b[j].Street {
		return false
	}
	if b[i].SubStreet < b[j].SubStreet {
		return true
	} else if b[i].SubStreet > b[j].SubStreet {
		return false
	}
	return b[i].POI < b[j].POI
}

// MetadataForeignLang is foreign language of specified metadata
type MetadataForeignLang struct {
	Lang   string
	Name   string
	NameEn string
}
