package exif

import (
	"fmt"
	"path/filepath"
	. "testing"

	. "gopkg.in/check.v1"
)

type exifSuite struct {
}

type expectTags struct {
	Latitude   float64
	Longitude  float64
	OrienName  string
	OrienValue int
}

var _ = Suite(&exifSuite{})

func TestExifSuite(t *T) {
	TestingT(t)
}

func (ts *exifSuite) TestExifBasic(c *C) {
	expectResults := map[string]expectTags{
		"1_2003_01_17.jpg": {Latitude: UnknownGPS, Longitude: UnknownGPS,
			OrienName: Orientation, OrienValue: 1},
		"4_2003_11_01.jpg": {Latitude: UnknownGPS, Longitude: UnknownGPS},
		"5_2003_11_23.jpg": {Latitude: 39.9155555555556, Longitude: 116.390833333333,
			OrienName: Orientation, OrienValue: 1},
		"6_2004_01_21.jpg": {Latitude: UnknownGPS, Longitude: UnknownGPS,
			OrienName: Orientation, OrienValue: 1},
		"8_2008_12_14.dng": {Latitude: UnknownGPS, Longitude: UnknownGPS,
			OrienName: Orientation, OrienValue: 1},
		"9_2013_07_28.png":   {Latitude: UnknownGPS, Longitude: UnknownGPS},
		"11_2014_01_21.webp": {Latitude: UnknownGPS, Longitude: UnknownGPS},
		"12_2014_01_21.heic": {Latitude: UnknownGPS, Longitude: UnknownGPS,
			OrienName: Rotation, OrienValue: 180},
		"14_2017_09_13.heic": {Latitude: -23.5398944444444, Longitude: -46.65655,
			OrienName: Orientation, OrienValue: 6},
		"preview.jpg": {Latitude: UnknownGPS, Longitude: UnknownGPS,
			OrienName: Orientation, OrienValue: 1},
	}
	for f, t := range expectResults {
		fmt.Printf("------ test %s\n", f)
		result, err := NewTags(filepath.Join("../../cmd/lomod/test/img", f), "exiftool", "")
		c.Assert(err, IsNil)
		c.Assert(result.GetLatitude(), Equals, t.Latitude)
		c.Assert(result.GetLongitude(), Equals, t.Longitude)
		n, v := result.GetOrientation()
		c.Assert(n, Equals, t.OrienName)
		c.Assert(v, Equals, t.OrienValue)
	}
}
