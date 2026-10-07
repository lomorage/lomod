package handler

import (
	"bytes"
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/avutil"
	"bitbucket.org/lomoware/lomo-backend/common/cmd"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/exif"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/migrator"
	"bitbucket.org/lomoware/lomo-backend/common/preview"
	"bitbucket.org/lomoware/lomo-backend/common/testutil"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/migrations/sqls/lomod"
	"github.com/pkg/errors"
	. "gopkg.in/check.v1"
)

type assetInfo struct {
	file        string
	hash        string
	time        string
	exif        string
	previewHash []string
}

var gpsAssets = []assetInfo{
	{"../cmd/lomod/test/img/5_2003_11_23.jpg",
		"4ebf54db04f335ff66bfc1fd982be62bf23fc967",
		"2003-11-23T18:07:37Z",
		`
[
    {
      "APP14Flags0": 0,
      "APP14Flags1": 0,
      "About": "uuid:5f5d2d26-eaa2-11d9-a6e9-a8189497d9c2",
      "Aperture": 4.5,
      "BitsPerSample": 8,
      "CFAPattern": "2 2 1 2 0 1",
      "CircleOfConfusion": 0.0200279788706131,
      "ColorComponents": 3,
      "ColorSpace": 65535,
      "ColorTransform": 1,
      "CompressedBitsPerPixel": 4,
      "Compression": 6,
      "Contrast": 1,
      "CopyrightFlag": 0,
      "CreateDate": "2003:11:23 18:07:37",
      "CustomRendered": 0,
      "DCTEncodeVersion": 100,
      "DateTimeOriginal": "2003:11:23 18:07:37",
      "DigitalZoomRatio": 1,
      "Directory": "/tmp/usbdisk1/alice/.lomodTemp",
      "DisplayedUnitsX": 1,
      "DisplayedUnitsY": 1,
      "DocumentID": "adobe:docid:photoshop:48361733-eaa2-11d9-a6e9-a8189497d9c2",
      "EncodingProcess": 0,
      "ExifByteOrder": "II",
      "ExifImageHeight": 375,
      "ExifImageWidth": 500,
      "ExifToolVersion": 11.88,
      "ExifVersion": "0220",
      "ExposureCompensation": 0,
      "ExposureMode": 0,
      "ExposureProgram": 3,
      "ExposureTime": 0.008,
      "FNumber": 4.5,
      "FOV": 54.4322690915859,
      "FileAccessDate": "",
      "FileInodeChangeDate": "",
      "FileModifyDate": "",
      "FileName": "4ebf54db04f335ff66bfc1fd982be62bf23fc967",
      "FilePermissions": 644,
      "FileSize": 80603,
      "FileSource": 3,
      "FileType": "JPEG",
      "FileTypeExtension": "JPG",
      "Flash": 0,
      "FlashpixVersion": "0100",
      "FocalLength": 23.33,
      "FocalLength35efl": 34.9999999999999,
      "FocalLengthIn35mmFormat": 35,
      "GPSDateStamp": "2003:11:23",
      "GPSDateTime": "2003:11:23 18:07:37Z",
      "GPSLatitude": 39.9155555555556,
      "GPSLatitudeRef": "N",
      "GPSLongitude": 116.390833333333,
      "GPSLongitudeRef": "E",
      "GPSPosition": "39.9155555555556 116.390833333333",
      "GPSTimeStamp": "18:07:37",
      "GPSVersionID": "2 2 0 0",
      "GainControl": 1,
      "GlobalAltitude": 30,
      "GlobalAngle": 30,
      "HasRealMergedData": 1,
      "HyperfocalDistance": 6.03920593636947,
      "IPTCDigest": "00000000000000000000000000000000",
      "ImageHeight": 375,
      "ImageSize": "500 375",
      "ImageWidth": 500,
      "JFIFVersion": "1 2",
      "LightSource": 0,
      "MIMEType": "image/jpeg",
      "Make": "NIKON CORPORATION",
      "MaxApertureValue": 2.82842712474619,
      "Megapixels": 0.1875,
      "MeteringMode": 3,
      "Model": "NIKON D2H",
      "ModifyDate": "2005:07:02 10:38:28",
      "NumSlices": 1,
      "Orientation": 1,
      "PhotoshopFormat": 0,
      "PhotoshopQuality": 4,
      "PhotoshopThumbnail": "(Binary data 4782 bytes, use -b option to extract)",
      "PrintPosition": "0 0",
      "PrintScale": 1,
      "PrintStyle": 0,
      "ProgressiveScans": 1,
      "ReaderName": "Adobe Photoshop 7.0",
      "RelatedSoundFile": "            ",
      "ResolutionUnit": 2,
      "Saturation": 0,
      "ScaleFactor35efl": 1.5002143163309,
      "SceneCaptureType": 0,
      "SceneType": 1,
      "SensingMethod": 2,
      "Sharpness": 0,
      "ShutterSpeed": 0.008,
      "SlicesGroupName": "gg_gps",
      "Software": "Opanda PowerExif",
      "SourceFile": "/tmp/usbdisk1/alice/.lomodTemp/4ebf54db04f335ff66bfc1fd982be62bf23fc967",
      "SubSecCreateDate": "2003:11:23 18:07:37.63",
      "SubSecDateTimeOriginal": "2003:11:23 18:07:37.63",
      "SubSecModifyDate": "2005:07:02 10:38:28.63",
      "SubSecTime": 63,
      "SubSecTimeDigitized": 63,
      "SubSecTimeOriginal": 63,
      "SubjectDistanceRange": 0,
      "ThumbnailImage": "(Binary data 4034 bytes, use -b option to extract)",
      "ThumbnailLength": 4034,
      "ThumbnailOffset": 1118,
      "URL_List": [],
      "UserComment": "taken at basilica of chinese",
      "WhiteBalance": 0,
      "WriterName": "Adobe Photoshop",
      "XMPToolkit": "XMP toolkit 2.8.2-33, framework 1.5",
      "XResolution": 256,
      "YCbCrSubSampling": "1 1",
      "YResolution": 256
    }
]`,
		[]string{"f23465291620bc3a97612e7e9db24151844ffc17",
			"8234cbfeaf434f488c28592d03a44d4a4ac9d721"},
	},
	{"../cmd/lomod/test/img/14_2017_09_13.heic",
		"2a8210982e4cfbeb56d283f43fea9c118a53a839",
		"2017-09-13T06:27:17Z",
		`
[
    {
      "AccelerationVector": "0.1813890762 -0.8817955112 -0.4300854701",
      "Aperture": 1.8,
      "ApertureValue": 1.79999993941969,
      "AverageFrameRate": 0,
      "BitDepthChroma": 8,
      "BitDepthLuma": 8,
      "BlueMatrixColumn": "0.1571 0.06657 0.78407",
      "BlueTRC": "(Binary data 32 bytes, use -b option to extract)",
      "BrightnessValue": 2.848839335,
      "CMMFlags": 0,
      "ChromaFormat": 1,
      "ChromaticAdaptation": "1.04788 0.02292 -0.0502 0.02959 0.99048 -0.01706 -0.00923 0.01508 0.75168",
      "CircleOfConfusion": "0.00428159213961349",
      "ColorSpace": 65535,
      "ColorSpaceData": "RGB ",
      "CompatibleBrands": [
        "mif1",
        "heic"
      ],
      "ComponentsConfiguration": "1 2 3 0",
      "ConnectionSpaceIlluminant": "0.9642 1 0.82491",
      "ConstantFrameRate": 0,
      "ConstraintIndicatorFlags": "176 0 0 0 0 0",
      "ContentIdentifier": "A6E2D396-5D75-45E1-B4B9-43D96584236B",
      "CreateDate": "2017:09:13 06:27:17",
      "DateTimeOriginal": "2017:09:13 06:27:17",
      "DeviceAttributes": "0 0",
      "DeviceManufacturer": "APPL",
      "DeviceModel": "",
      "Directory": "/tmp/usbdisk1/alice/.lomodTemp",
      "ExifByteOrder": "MM",
      "ExifImageHeight": 3024,
      "ExifImageWidth": 4032,
      "ExifToolVersion": 11.88,
      "ExifVersion": "0221",
      "ExposureCompensation": 0,
      "ExposureMode": 0,
      "ExposureProgram": 2,
      "ExposureTime": 0.05,
      "FNumber": 1.8,
      "FOV": 65.4705078447874,
      "FileAccessDate": "",
      "FileInodeChangeDate": "",
      "FileModifyDate": "",
      "FileName": "2a8210982e4cfbeb56d283f43fea9c118a53a839",
      "FilePermissions": 644,
      "FileSize": 1051433,
      "FileType": "HEIC",
      "FileTypeExtension": "HEIC",
      "Flash": 24,
      "FlashpixVersion": "0100",
      "FocalLength": 3.99,
      "FocalLength35efl": 28,
      "FocalLengthIn35mmFormat": 28,
      "GPSAltitude": 778.0337079,
      "GPSAltitudeRef": 0,
      "GPSDateStamp": "2017:09:13",
      "GPSDateTime": "2017:09:13 09:27:15.97Z",
      "GPSDestBearing": 324.7488372,
      "GPSDestBearingRef": "M",
      "GPSHPositioningError": 30,
      "GPSImgDirection": 324.7488372,
      "GPSImgDirectionRef": "M",
      "GPSLatitude": -23.5398944444444,
      "GPSLatitudeRef": "S",
      "GPSLongitude": -46.65655,
      "GPSLongitudeRef": "W",
      "GPSPosition": "-23.5398944444444 -46.65655",
      "GPSSpeed": 0.44,
      "GPSSpeedRef": "K",
      "GPSTimeStamp": "09:27:15.97",
      "GenProfileCompatibilityFlags": 1879048192,
      "GeneralLevelIDC": 90,
      "GeneralProfileIDC": 3,
      "GeneralProfileSpace": 0,
      "GeneralTierFlag": 0,
      "GreenMatrixColumn": "0.29198 0.69225 0.04189",
      "GreenTRC": "(Binary data 32 bytes, use -b option to extract)",
      "HEVCConfigurationVersion": 1,
      "HandlerType": "pict",
      "HyperfocalDistance": 2.06570353074275,
      "ISO": 40,
      "ImageHeight": 3024,
      "ImagePixelDepth": "8 8 8",
      "ImageSize": "4032 3024",
      "ImageSpatialExtent": "4032 3024",
      "ImageWidth": 4032,
      "LensInfo": "3.99 6.6 1.8 2.8",
      "LensMake": "Apple",
      "LensModel": "iPhone 7 Plus back dual camera 3.99mm f/1.8",
      "LightValue": 7.33985000288462,
      "MIMEType": "image/heic",
      "MajorBrand": "heic",
      "Make": "Apple",
      "MediaDataOffset": 3996,
      "MediaDataSize": 1047437,
      "MediaWhitePoint": "0.95045 1 1.08905",
      "Megapixels": 12.192768,
      "MeteringMode": 5,
      "MinSpatialSegmentationIDC": 0,
      "MinorVersion": "0.0.0",
      "Model": "iPhone 7 Plus",
      "ModifyDate": "2017:09:13 06:27:17",
      "NumTemporalLayers": 1,
      "Orientation": 6,
      "ParallelismType": 0,
      "PrimaryItemReference": 49,
      "PrimaryPlatform": "APPL",
      "ProfileCMMType": "appl",
      "ProfileClass": "mntr",
      "ProfileConnectionSpace": "XYZ ",
      "ProfileCopyright": "Copyright Apple Inc., 2017",
      "ProfileCreator": "appl",
      "ProfileDateTime": "2017:07:07 13:22:32",
      "ProfileDescription": "Display P3",
      "ProfileFileSignature": "acsp",
      "ProfileID": "202 26 149 130 37 127 16 77 56 153 19 213 209 234 21 130",
      "ProfileVersion": 1024,
      "RedMatrixColumn": "0.51512 0.2412 -0.00105",
      "RedTRC": "(Binary data 32 bytes, use -b option to extract)",
      "RenderingIntent": 0,
      "ResolutionUnit": 2,
      "Rotation": 270,
      "RunTimeEpoch": 0,
      "RunTimeFlags": 1,
      "RunTimeScale": 1000000000,
      "RunTimeSincePowerUp": 13118.806867208,
      "RunTimeValue": 13118806867208,
      "ScaleFactor35efl": 7.01754385964912,
      "SceneCaptureType": 0,
      "SceneType": 1,
      "SensingMethod": 2,
      "ShutterSpeed": 0.05,
      "ShutterSpeedValue": 0.0499979995786938,
      "Software": 11,
      "SourceFile": "/tmp/usbdisk1/alice/.lomodTemp/2a8210982e4cfbeb56d283f43fea9c118a53a839",
      "SubSecCreateDate": "2017:09:13 06:27:17.266",
      "SubSecDateTimeOriginal": "2017:09:13 06:27:17.266",
      "SubSecTimeDigitized": 266,
      "SubSecTimeOriginal": 266,
      "SubjectArea": "2015 1511 2217 1330",
      "TemporalIDNested": 0,
      "WhiteBalance": 0,
      "XResolution": 72,
      "YCbCrPositioning": 1,
      "YResolution": 72
    }
]`,
		[]string{"c81a034dafa25c5c9e0cb3638b324caa7aedd7a8",
			"0dedadba34002b3209a7678e29302034fb761915"},
	},
}

var gpsCategory = types.Years{Hash: "9e9cf5055cee4aaa04b7b6c5e270c9424a368a4e", Years: []types.Year{
	{Year: 2003, Hash: "1a7f7c7adf7e1fbcc44d83fc89b95d7815d57bf1", Months: []types.Month{
		{Month: 11, Hash: "fc61d9e5151f626737395e8fe56645f9c22dd8fb", Days: []types.Day{
			{Day: 23, Hash: "ee8e87bb216aa46de86501f4a4c5a27d00aff155", Assets: []types.Asset{
				{Name: "1.jpg", Hash: "4ebf54db04f335ff66bfc1fd982be62bf23fc967",
					Date: types.LomoTime{time.Date(2003, 11, 23, 18, 7, 37, 0, time.UTC)}},
			}},
		}},
	}},
	{Year: 2017, Hash: "6cbc0a6630b5de7c844dd148352d197bae14643b", Months: []types.Month{
		{Month: 9, Hash: "9d716ac24587512316705483aa1f846a0345c248", Days: []types.Day{
			{Day: 13, Hash: "9b882943326363e0633bc3b03f9da5dc9086186c", Assets: []types.Asset{
				{Name: "2.heic", Hash: "2a8210982e4cfbeb56d283f43fea9c118a53a839",
					Date: types.LomoTime{time.Date(2017, 9, 13, 6, 27, 17, 0, time.UTC)}},
			}},
		}},
	}},
}}

var livePhotoHashes = []string{"adf6b68f7e71912a4e2666533d4c8619f1b9ddb1", "7c672f891ac7eae12f8cbd188bf55524dac4242d"}

const assetInfoFile = "../api/test/lomod/testdata/assets_info.json"

type assetCategory struct {
	file       string
	yIdx       int
	mIdx       int
	dIdx       int
	aIdx       int
	ext        string
	aliceToken bool
}

var assetsShort = []assetCategory{
	{"../cmd/lomod/test/img/5_2003_11_23.jpg", 0, 1, 1, 0, "jpg", true},
	{"../cmd/lomod/test/img/4_2003_11_01.jpg", 0, 1, 0, 1, "jpg", false},
	{"../cmd/lomod/test/img/3_2003_11_01.jpg", 0, 1, 0, 0, "jpg", false},
	{"../cmd/lomod/test/img/1_2003_01_17.jpg", 0, 0, 0, 0, "jpg", true},
	{"../cmd/lomod/test/img/6_2004_01_21.jpg", 1, 0, 0, 0, "jpeg", true},
	{"../cmd/lomod/test/video/10_2013_08_08.mp4", 2, 1, 0, 0, "mp4", true},
	{"../cmd/lomod/test/img/9_2013_07_28.png", 2, 0, 0, 0, "png", true},
	{"../cmd/lomod/test/liph/2_2003_01_17.zip", 2, 2, 0, 2, "zip", true},
	{"../cmd/lomod/test/img/14_2017_09_13.heic", 2, 2, 0, 0, "heif", false},
	{"../cmd/lomod/test/liph/15_2017_09_13.zip", 2, 2, 0, 1, "zip", false},
}

func loadCategory() (types.Years, error) {
	fname := "../common/asset/assets_short.json"
	years := types.Years{}
	contents, err := ioutil.ReadFile(fname)
	if err != nil {
		return years, err
	}

	return years, json.Unmarshal(contents, &years)
}

func (ts *mainSuite) validateBasicCategory(c *C, category types.Years) {
	var (
		year, emptyYear1, emptyYear2    types.Year
		years, emptyYears1, emptyYears2 types.Years
	)
	if len(category.Years) != 0 && category.Years[0].Year == 2003 {
		ts.requestGet(c, fmt.Sprintf("/category/2003?token=%s", ts.token), &year, &category.Years[0])
		emptyYear2 = types.Year{Year: 2003, Months: []types.Month{}}
		ts.requestGet(c, fmt.Sprintf("/category/2003?username=bob&token=%s", ts.tokenBob), &emptyYear1, &emptyYear2)
	}

	if len(category.Years) > 1 && category.Years[1].Year == 2004 {
		ts.requestGet(c, fmt.Sprintf("/category/2004?token=%s", ts.token), &year, &category.Years[1])
		emptyYear2.Year = 2004
		ts.requestGet(c, fmt.Sprintf("/category/2004?username=bob&token=%s", ts.tokenBob), &emptyYear1, &emptyYear2)
	}

	if len(category.Years) > 2 && category.Years[2].Year == 2013 {
		ts.requestGet(c, fmt.Sprintf("/category/2013?token=%s", ts.token), &year, &category.Years[2])
		emptyYear2.Year = 2013
		ts.requestGet(c, fmt.Sprintf("/category/2013?username=bob&token=%s", ts.tokenBob), &emptyYear1, &emptyYear2)
	}

	expectedYears := types.Years{Hash: category.Hash}
	for _, year := range category.Years {
		y := types.Year{Hash: year.Hash, Year: year.Year}
		for _, month := range year.Months {
			y.Months = append(y.Months, types.Month{Hash: month.Hash, Month: month.Month, Days: []types.Day{}})
		}
		expectedYears.Years = append(expectedYears.Years, y)

		ts.requestGet(c, fmt.Sprintf("/category/%d?token=%s", year.Year, ts.token), &types.Year{}, &year)
	}
	if expectedYears.Years == nil {
		expectedYears.Years = []types.Year{}
	}
	ts.requestGet(c, fmt.Sprintf("/category?token=%s", ts.token), &years, &expectedYears)

	emptyYears2 = types.Years{Years: []types.Year{}}
	ts.requestGet(c, fmt.Sprintf("/category?username=bob&token=%s", ts.tokenBob), &emptyYears1, &emptyYears2)
}

func (ts *mainSuite) createLivephotos(c *C) {
	url := fmt.Sprintf("/asset?token=%s&ext=zip&createtime=2013-11-23T12:00:00Z&sha1=%s", ts.token,
		livePhotoHashes[0])
	ts.createAsset(c, "../cmd/lomod/test/liph/2_2003_01_17.zip", url, "1.zip", livePhotoHashes[0])

	url = fmt.Sprintf("/asset?token=%s&ext=zip&createtime=2013-11-23T12:00:00Z&sha1=%s", ts.token,
		livePhotoHashes[1])
	ts.createAsset(c, "../cmd/lomod/test/liph/15_2017_09_13.zip", url, "2.zip", livePhotoHashes[1])
}

// jpg live photo uses the one without comment; heif live photo uses the one having comment on the fly.
func (ts *mainSuite) createAssets(c *C, category *types.Years, token string) {
	// upload one by one, list, compare, delete and compare
	for _, a := range assetsShort {
		t := token
		if a.aliceToken {
			t = ts.token
		}
		h := category.Years[a.yIdx].Months[a.mIdx].Days[a.dIdx].Assets[a.aIdx].Hash
		url := fmt.Sprintf("/asset?token=%s&ext=%s&createtime=%s&sha1=%s", t, a.ext,
			category.Years[a.yIdx].Months[a.mIdx].Days[a.dIdx].Assets[a.aIdx].Date, h)
		ts.createAsset(c, a.file, url, category.Years[a.yIdx].Months[a.mIdx].Days[a.dIdx].Assets[a.aIdx].Name, h)
	}
}

// create all assets
func (ts *mainSuite) createAllAssets(c *C) int {
	category, err := loadCategory()
	c.Assert(err, IsNil)

	ts.createAssets(c, &category, ts.token)
	// add 3 new files not in category
	sha := "0c03dcd19f804a559bd0ae32db2998dad4a4936a"
	u := fmt.Sprintf("/asset?token=%s&ext=webp&createtime=2003-11-23T12:00:00Z&sha1=%s", ts.token, sha)
	ts.createAsset(c, "../cmd/lomod/test/img/11_2014_01_21.webp", u, "11.webp", sha)

	sha = "893326e969385849d88176538650f181cd350e52"
	u = fmt.Sprintf("/asset?token=%s&ext=dng&createtime=2003-11-23T12:00:00Z&sha1=%s", ts.token, sha)
	ts.createAsset(c, "../cmd/lomod/test/img/8_2008_12_14.dng", u, "12.dng", sha)

	sha = "27bdb514e0501360dc81eb1db1165cb8c23c6acd"
	u = fmt.Sprintf("/asset?token=%s&ext=heic&createtime=2003-11-23T12:00:00Z&sha1=%s", ts.token, sha)
	ts.createAsset(c, "../cmd/lomod/test/img/12_2014_01_21.heic", u, "13.heic", sha)

	return 13
}

func (ts *mainSuite) validateAssetsDownload(c *C, category types.Years) {
	for _, year := range category.Years {
		for _, month := range year.Months {
			for _, day := range month.Days {
				for _, asset := range day.Assets {
					e := filepath.Ext(asset.Name)
					if e == ".zip" {
						fmt.Printf("skip zip file %+v\n", asset)
						continue
					}
					u := "/asset/" + asset.Hash + `?token=` + ts.token
					if ext.IsVideoFile(e) {
						u += "&" + common.QueryKeyOrig + "=1"
					}
					ts.assetGet(c, u, asset.Hash)
				}
			}
		}
	}
}

func (ts *mainSuite) validateAssets(c *C, start int) {
	ts.validateAsset(c, fmt.Sprintf("%s/alice/Photos/master/2003/11/23/20031123_%d.jpg", photodir, start), 80603)
	ts.validateAsset(c, fmt.Sprintf("%s/alice/Photos/master/2003/11/01/20031101_%d.jpg", photodir, start+1), 70513)
	ts.validateAsset(c, fmt.Sprintf("%s/alice/Photos/master/2003/11/01/20031101_%d.jpg", photodir, start+2), 247759)
	ts.validateAsset(c, fmt.Sprintf("%s/alice/Photos/master/2003/01/17/20030117_%d.jpg", photodir, start+3), 231635)
	ts.validateAsset(c, fmt.Sprintf("%s/alice/Photos/master/2004/01/21/20040121_%d.jpg", photodir, start+4), 1125776)
	ts.validateAsset(c, fmt.Sprintf("%s/alice/Photos/master/2013/08/08/20130808_%d.mp4", photodir, start+5), 7360)
	ts.validateAsset(c, fmt.Sprintf("%s/alice/Photos/master/2013/07/28/20130728_%d.png", photodir, start+6), 44969)
	ts.validateAsset(c, fmt.Sprintf("%s/alice/Photos/master/2013/11/23/20131123_%d_image.jpg", photodir, start+7), 231635)
	ts.validateAsset(c, fmt.Sprintf("%s/alice/Photos/master/2013/11/23/20131123_%d.zip", photodir, start+7), 238254)
	ts.validateAsset(c, fmt.Sprintf("%s/alice/Photos/master/2013/11/23/20131123_%d.heic", photodir, start+8), 1051433)
	ts.validateAsset(c, fmt.Sprintf("%s/alice/Photos/master/2013/11/23/20131123_%d_image.heic", photodir, start+9), 1051433)
	ts.validateAsset(c, fmt.Sprintf("%s/alice/Photos/master/2013/11/23/20131123_%d.zip", photodir, start+9), 1055380)

	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/", photodir), 3, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2003", photodir), 2, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2004", photodir), 1, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2013", photodir), 3, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2003/11", photodir), 2, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2003/11/23", photodir), 0, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2003/11/23", photodir), 1, false)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2003/11/01", photodir), 0, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2003/11/01", photodir), 2, false)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2003/01", photodir), 1, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2003/01/17", photodir), 0, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2003/01/17", photodir), 1, false)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2004", photodir), 1, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2004/01", photodir), 1, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2004/01/21", photodir), 0, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2004/01/21", photodir), 1, false)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2013", photodir), 3, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2013/08", photodir), 1, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2013/08/08", photodir), 0, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2013/08/08", photodir), 1, false)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2013/07", photodir), 1, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2013/07/28", photodir), 0, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2013/07/28", photodir), 1, false)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2013/11", photodir), 1, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2013/11/23", photodir), 0, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2013/11/23", photodir), 5, false)
}

func (ts *mainSuite) waitPreviewComplete(c *C, hasBob bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c.Assert(testutil.WaitWithFileCounts(ctx, photodir+"/alice/Photos/preview/2003/11/23", 2), IsNil)
	if !hasBob {
		c.Assert(testutil.WaitWithFileCounts(ctx, photodir+"/alice/Photos/preview/2003/11/01", 4), IsNil)
	}
	c.Assert(testutil.WaitWithFileCounts(ctx, photodir+"/alice/Photos/preview/2003/01/17", 2), IsNil)
	c.Assert(testutil.WaitWithFileCounts(ctx, photodir+"/alice/Photos/preview/2004/01/21", 2), IsNil)
	c.Assert(testutil.WaitWithFileCounts(ctx, photodir+"/alice/Photos/preview/2013/08/08", 3), IsNil)
	c.Assert(testutil.WaitWithFileCounts(ctx, photodir+"/alice/Photos/preview/2013/07/28", 2), IsNil)
	if hasBob {
		c.Assert(testutil.WaitWithFileCounts(ctx, photodir+"/alice/Photos/preview/2013/11/23", 2), IsNil)
	} else {
		c.Assert(testutil.WaitWithFileCounts(ctx, photodir+"/alice/Photos/preview/2013/11/23", 6), IsNil)
	}
	c.Assert(ts.h.previewRunner.PendingJobCount(), Equals, 0)
}

func (ts *mainSuite) validateAsset(c *C, p string, size int) {
	stat, err := os.Stat(p)
	if size == -1 {
		c.Assert(err, NotNil, Commentf("p: %s", p))
		c.Assert(os.IsNotExist(err), Equals, true)
		return
	}
	c.Assert(err, IsNil)
	c.Assert(stat.Size(), Equals, int64(size), Commentf("p: %s", p))
}

func (ts *mainSuite) validateDir(c *C, p string, numFiles int, dir bool) {
	fis, err := ioutil.ReadDir(p)
	c.Assert(err, IsNil)
	count := 0
	allFiles := []string{}
	countedFiles := []string{}
	for _, fi := range fis {
		allFiles = append(allFiles, fi.Name())
		if dir {
			if fi.IsDir() {
				count++
				countedFiles = append(countedFiles, fi.Name())
			}
		} else {
			if !fi.IsDir() {
				count++
				countedFiles = append(countedFiles, fi.Name())
			}
		}
	}
	c.Assert(count, Equals, numFiles, Commentf("p: %s, dir mode: %v, %v(%v)", p, dir, countedFiles, allFiles))
}

func (ts *mainSuite) importAsset(c *C, p, url string) *types.Asset {
	var a io.ReadCloser
	if filepath.IsAbs(p) {
		var err error
		a, err = os.Open(p)
		c.Assert(err, IsNil)
	} else {
		wd, err := os.Getwd()
		c.Assert(err, IsNil)
		a, err = os.Open(filepath.Join(wd, p))
		c.Assert(err, IsNil)
	}
	defer a.Close()

	req, err := http.NewRequest("POST", url, a)
	req.Header.Set("Content-Type", "application/octet-stream")
	c.Assert(err, IsNil)

	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	validateStatusCode(c, url, rr, http.StatusOK)

	resp := rr.Result()
	defer resp.Body.Close()

	ai := types.Asset{}
	c.Assert(json.NewDecoder(resp.Body).Decode(&ai), IsNil)
	return &ai
}

func (ts *mainSuite) importAssetWithResult(c *C, p, url, method, expectBody string, status int,
	headers map[string]string) []byte {
	var a io.ReadCloser
	if path.IsAbs(p) {
		var err error
		a, err = os.Open(p)
		c.Assert(err, IsNil)
	} else {
		wd, err := os.Getwd()
		c.Assert(err, IsNil)
		a, err = os.Open(path.Join(wd, p))
		c.Assert(err, IsNil)
	}
	defer a.Close()

	req, err := http.NewRequest(method, url, a)
	req.Header.Set("Content-Type", "application/octet-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	c.Assert(err, IsNil)

	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	validateStatusCode(c, url, rr, status)

	resp := rr.Result()
	defer resp.Body.Close()

	buf, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, IsNil)
	if expectBody != "" {
		c.Assert(string(buf), Equals, expectBody)
	}
	return buf
}

func (ts *mainSuite) createAsset(c *C, p, url, expectedName, expectedHash string) {
	ai := ts.importAsset(c, p, url)
	if expectedName != "" {
		c.Assert(ai.Name, Equals, expectedName)
	}
	c.Assert(ai.Hash, Equals, expectedHash)
}

func (ts *mainSuite) assetGet(c *C, url string, expectedHash string) {
	req, err := http.NewRequest("GET", url, nil)
	c.Assert(err, IsNil)

	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	validateStatusCode(c, url, rr, http.StatusOK)

	resp := rr.Result()
	defer resp.Body.Close()

	hash := sha1.New()
	_, err = io.Copy(hash, resp.Body)
	c.Assert(err, IsNil)
	c.Assert(fmt.Sprintf("%x", hash.Sum(nil)), Equals, expectedHash)
}

/*
 moved to api/test/lomod TestAssetBasic except below case
 TODO: is below case needed? or should we add asset validation capability and auto regenerate
func (ts *mainSuite) TestAssetPreviewJIT(c *C) {
	// preview should read from cached one. This test will manually replace pre-created preview with empty file, and make sure it was download
	c.Assert(os.Remove(cachedFilename), IsNil)
	f, err := os.Create(cachedFilename)
	c.Assert(err, IsNil)
	_, err = f.WriteString("hello")
	c.Assert(err, IsNil)
	c.Assert(f.Close(), IsNil)
	ts.assetGet(c, fmt.Sprintf("/preview/1?token=%s&width=75&height=75", ts.token), "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d")
}
*/

func (ts *mainSuite) TestAssetMemdb(c *C) {
	tmpdir, err := ioutil.TempDir("", "")
	c.Assert(err, IsNil)
	defer os.RemoveAll(tmpdir)

	testDB, err := os.Create(path.Join(tmpdir, "assets.db"))
	c.Assert(err, IsNil)

	wd, err := os.Getwd()
	c.Assert(err, IsNil)
	origDB, err := os.Open(path.Join(wd, "../cmd/lomod/test/assets.db"))
	c.Assert(err, IsNil)
	size, err := io.Copy(testDB, origDB)
	c.Assert(err, IsNil)
	c.Assert(int(size), Equals, 1349632)
	c.Assert(testDB.Close(), IsNil)
	c.Assert(origDB.Close(), IsNil)

	c.Assert(migrator.StartLomod(testDB.Name(), "", lomod.SchemaStatements, common.DefaultFolderPermission), IsNil)
	ts.resetDB(c, testDB.Name(), true)

	for _, token := range []string{"1234567", "1234567"} {
		ts.compareMemDBVsRealDB(c, fmt.Sprintf("/category?token=%s", token), &types.Years{}, &types.Years{})
		for y := 2007; y < 2020; y++ {
			ts.compareMemDBVsRealDB(c, fmt.Sprintf("/category/%d?token=%s", y, token), &types.Year{}, &types.Year{})
			for m := 1; m <= 12; m++ {
				ts.compareMemDBVsRealDB(c, fmt.Sprintf("/category/%d/%d?token=%s", y, m, token),
					&types.Month{}, &types.Month{})
				for d := 1; d <= 31; d++ {
					days := daysInMonth(y, time.Month(m))
					if d > days {
						break
					}
					ts.compareMemDBVsRealDB(c, fmt.Sprintf("/category/%d/%d/%d?token=%s", y, m, d, token),
						&types.Day{}, &types.Day{})
				}
			}
		}
	}
}

func (ts *mainSuite) compareMemDBVsRealDB(c *C, url string, memdb, realdb interface{}) {
	req, err := http.NewRequest("GET", url, nil)
	c.Assert(err, IsNil)

	ts.h.conf.UseMemdb = true
	rrMemdb := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rrMemdb, req)
	validateStatusCode(c, url, rrMemdb, http.StatusOK)

	resp := rrMemdb.Result()
	defer resp.Body.Close()

	json.NewDecoder(resp.Body).Decode(memdb)

	ts.h.conf.UseMemdb = false
	rrOrigdb := httptest.NewRecorder()
	req, err = http.NewRequest("GET", url, nil)
	c.Assert(err, IsNil)
	ts.h.CreateRouter().ServeHTTP(rrOrigdb, req)
	validateStatusCode(c, url, rrOrigdb, http.StatusOK)

	respOrig := rrOrigdb.Result()
	defer respOrig.Body.Close()

	json.NewDecoder(respOrig.Body).Decode(realdb)

	c.Assert(memdb, DeepEquals, realdb, Commentf("input url: %s", url))
}

func (ts *mainSuite) TestAssetUploadSHA1(c *C) {
	// wrong SHA should return failure
	url := fmt.Sprintf("/asset?token=%s&ext=jpg&createtime=2003-11-23T12:00:00Z&sha1=631ab5ab5befe28f88ad5c2af28e5def4b477a67", ts.token)
	var a io.ReadCloser
	p := "../cmd/lomod/test/img/5_2003_11_23.jpg"
	wd, err := os.Getwd()
	c.Assert(err, IsNil)
	a, err = os.Open(path.Join(wd, p))
	c.Assert(err, IsNil)
	defer a.Close()

	req, err := http.NewRequest("POST", url, a)
	req.Header.Set("Content-Type", "application/octet-stream")
	c.Assert(err, IsNil)

	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	validateStatusCode(c, url, rr, http.StatusBadRequest)

	resp := rr.Result()
	defer resp.Body.Close()

	buf := &bytes.Buffer{}
	_, err = io.Copy(buf, resp.Body)
	c.Assert(err, IsNil)
	reply := buf.String()
	fmt.Println(reply)
	c.Assert(strings.Contains(reply, common.ErrAssetDiffHash.Error()), Equals, true)

	// correct SHA should return ok
	url = fmt.Sprintf(
		"/asset?token=%s&ext=jpg&createtime=2003-11-23T12:00:00Z&sha1=4ebf54db04f335ff66bfc1fd982be62bf23fc967",
		ts.token)
	ts.createAsset(c, p, url, "1.jpg", "4ebf54db04f335ff66bfc1fd982be62bf23fc967")

	// sleep 1 second until libvips finish thumbnail creation
	time.Sleep(time.Second)
}

type resumedAsset struct {
	extension    string
	sha          string
	path         string
	p1Size       int
	p1SHA        string
	p1Error      string
	p1Path       string
	p2Path       string
	expectStatus int
}

func (ts *mainSuite) testAssetResumeWithInfo(c *C, ra resumedAsset) {
	// test 1: basic flow
	//  1. use HEAD to query, which should return 404
	//  2. upload partial content
	//  3. use HEAD to get partial info and 206
	//  4. use PATCH to upload left contents. upload should be success
	//  5. use HEAD again, which should return 200
	ts.requestWithMethod(c, fmt.Sprintf("/asset/%s?token=%s", ra.sha, ts.token), http.MethodHead,
		http.StatusNotFound, nil, nil, nil)

	ts.importAssetWithResult(c, ra.p1Path,
		fmt.Sprintf("/asset/%s?token=%s&ext=%s&createtime=2003-11-23T12:00:00Z", ra.sha, ts.token,
			ra.extension), http.MethodPost, ra.p1Error, ra.expectStatus, nil)

	ts.validateAsset(c, fmt.Sprintf("%s/alice/.lomodTemp/%s", photodir, ra.sha), ra.p1Size)

	headers := map[string]string{"If-Match": fmt.Sprintf("size=%d, sha1=%s", ra.p1Size, ra.p1SHA)}
	ts.requestWithMethod(c, fmt.Sprintf("/asset/%s?token=%s", ra.sha, ts.token), http.MethodHead,
		http.StatusPartialContent, nil, nil, headers)

	reply := ts.importAssetWithResult(c, ra.p2Path,
		fmt.Sprintf("/asset/%s?token=%s&ext=%s&createtime=2003-11-23T12:00:00Z", ra.sha, ts.token, ra.extension), http.MethodPatch,
		"", http.StatusOK, headers)

	a := types.Asset{}
	c.Assert(json.NewDecoder(bytes.NewBuffer(reply)).Decode(&a), IsNil)

	ts.requestWithMethod(c, fmt.Sprintf("/asset/%s?token=%s", ra.sha, ts.token), http.MethodHead, http.StatusOK, nil, nil, nil)
	ts.validateDir(c, fmt.Sprintf("%s/alice/.lomodTemp", photodir), 0, true)

	time.Sleep(5 * time.Second)
	ts.requestDelete(c, ts.token, &types.DeleteAssetItems{
		List: []types.DeleteAssetItem{{ID: a.Name}},
	})

	// test 2: upload partial content, and then upload again, which should overwrite the first one
	ts.requestWithMethod(c, fmt.Sprintf("/asset/%s?token=%s", ra.sha, ts.token), http.MethodHead, http.StatusNotFound, nil, nil, nil)
	ts.importAssetWithResult(c, ra.p1Path,
		fmt.Sprintf("/asset/%s?token=%s&ext=%s&createtime=2003-11-23T12:00:00Z", ra.sha, ts.token,
			ra.extension), "POST", ra.p1Error, ra.expectStatus, nil)

	ts.importAsset(c, ra.path, fmt.Sprintf("/asset/%s?token=%s&ext=%s&createtime=2003-11-23T12:00:00Z", ra.sha, ts.token, ra.extension))
	ts.validateDir(c, fmt.Sprintf("%s/alice/.lomodTemp", photodir), 0, true)
	ts.requestWithMethod(c, fmt.Sprintf("/asset/%s?token=%s", ra.sha, ts.token), http.MethodHead, http.StatusOK, nil, nil, nil)
	time.Sleep(5 * time.Second)
	ts.requestDelete(c, ts.token, &types.DeleteAssetItems{
		List: []types.DeleteAssetItem{{ID: "1", Type: types.Hash}},
	})

}

func (ts *mainSuite) TestAssetResume(c *C) {
	ras := []resumedAsset{
		{
			path: "../cmd/lomod/test/img/5_2003_11_23.jpg", sha: "4ebf54db04f335ff66bfc1fd982be62bf23fc967", extension: "jpg",
			p1Path: "../cmd/lomod/test/img/5_2003_11_23.jpg.part1", p1Size: 1000, p1SHA: "631ab5ab5befe28f88ad5c2af28e5def4b477a67",
			p1Error:      "{\"id\": \"24\", \"text\": \"[save asset 4ebf54db04f335ff66bfc1fd982be62bf23fc967]: Uploaded asset has different hash\"}\n",
			p2Path:       "../cmd/lomod/test/img/5_2003_11_23.jpg.part2",
			expectStatus: http.StatusBadRequest,
		},
		{
			path: "../cmd/lomod/test/liph/2_2003_01_17.zip", sha: "adf6b68f7e71912a4e2666533d4c8619f1b9ddb1", extension: "zip",
			p1Path: "../cmd/lomod/test/liph/2_2003_01_17.zip.part1", p1Size: 1000, p1SHA: "e950e850fcef58b7b339866b7ca01a976ad88bc7",
			p1Error:      "{\"id\": \"0\", \"text\": \"[save asset adf6b68f7e71912a4e2666533d4c8619f1b9ddb1]: zip: not a valid zip file\"}\n",
			p2Path:       "../cmd/lomod/test/liph/2_2003_01_17.zip.part2",
			expectStatus: http.StatusInternalServerError,
		},
		{
			path: "../cmd/lomod/test/liph/15_2017_09_13.zip", sha: "7c672f891ac7eae12f8cbd188bf55524dac4242d", extension: "zip",
			p1Path: "../cmd/lomod/test/liph/15_2017_09_13.zip.part1", p1Size: 1000, p1SHA: "37b912bb490fd872daf53f9da27bcb813b0f0219",
			p1Error:      "{\"id\": \"0\", \"text\": \"[save asset 7c672f891ac7eae12f8cbd188bf55524dac4242d]: zip: not a valid zip file\"}\n",
			p2Path:       "../cmd/lomod/test/liph/15_2017_09_13.zip.part2",
			expectStatus: http.StatusInternalServerError,
		},
	}
	for _, ra := range ras {
		ts.testAssetResumeWithInfo(c, ra)
	}
}

func (ts *mainSuite) TestAssetSlowMotion(c *C) {
	contentSHA := "b12fb8a5bb04738efe66f186a45f0388d8fabff1"
	fileSHA := "a6902dc1b38136afe949a78d7d458d760125c864"
	url := fmt.Sprintf("/asset/%s?token=%s&ext=mov&createtime=2003-11-22T00:01:00Z&filesha=%s", contentSHA, ts.token, fileSHA)

	ai := ts.importAsset(c, "../cmd/lomod/test/video/slow_motion.mov", url)
	c.Assert(ai.Name, Equals, "1.mov")
	c.Assert(ai.Hash, Equals, contentSHA)

	ts.validateAsset(c, fmt.Sprintf("%s/alice/Photos/master/2003/11/22/20031122_%d.mov", photodir, 1), 237228)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2003/11/22", photodir), 0, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2003/11/22", photodir), 1, false)

	// for preview generation
	c.Assert(testutil.WaitWithFileCounts(context.Background(), photodir+"/alice/Photos/preview/2003/11/22", 3), IsNil)
}

type info struct {
	f   string
	sha string
	w   uint
	h   uint
	s   int
	r   string
	sw  uint
	sh  uint
}

func (ts *mainSuite) TestVideoPreviewBasic(c *C) {
	assets := []info{
		{
			f:   "../cmd/lomod/test/video/rotate_0.mov",
			sha: "6d7d57813d3bbf94c92c5aa17ced276b99bf85ca",
			w:   1920,
			h:   1080,
			r:   "",
			s:   687941,
			sw:  320,
			sh:  180,
		},
		{
			f:   "../cmd/lomod/test/video/rotate_90.mov",
			sha: "b3a1a4840d85fef1b108c0689518c35ff1f3a951",
			w:   1920,
			h:   1080,
			r:   "90",
			s:   721439,
			sw:  180,
			sh:  320,
		},
		{
			f:   "../cmd/lomod/test/video/rotate_180.mov",
			sha: "37300a14028b7b9571c8ed094e525fa1f925e0ab",
			w:   1920,
			h:   1080,
			r:   "180",
			s:   724483,
			sw:  320,
			sh:  180,
		},
		{
			f:   "../cmd/lomod/test/video/rotate_270.mov",
			sha: "02fafad174eedb48f7ec0f4dc8f233966b36ddd8",
			w:   1920,
			h:   1080,
			r:   "270",
			s:   722001,
			sw:  180,
			sh:  320,
		},
		{
			f:   "../cmd/lomod/test/video/rotate_notag.mov",
			sha: "3d2bfe57ef239612a16c5e2737a27bd6907131d5",
			w:   1080,
			h:   1920,
			r:   "",
			s:   260327,
			sw:  180,
			sh:  320,
		},
	}
	engine := avutil.DefaultEngine(ts.h.transcodeApp, ts.h.ffprobe)
	for i, a := range assets {
		info, err := engine.ProbeVideoInfo(a.f)
		c.Assert(err, IsNil)
		c.Assert(info.Streams[0].Width, Equals, a.w)
		c.Assert(info.Streams[0].Height, Equals, a.h)
		c.Assert(info.Streams[0].Tags.Rotate, Equals, a.r)
		url := fmt.Sprintf("/asset/%s?token=%s&ext=mov&createtime=2003-11-22T00:01:00Z", a.sha, ts.token)
		ai := ts.importAsset(c, a.f, url)

		idx := strconv.Itoa(i + 1)
		c.Assert(ai.Name, Equals, idx+".mov")
		c.Assert(ai.Hash, Equals, a.sha)
		ts.validateAsset(c, fmt.Sprintf("%s/alice/Photos/master/2003/11/22/20031122_"+idx+".mov", photodir), a.s)
	}
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2003/11/22", photodir), 0, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2003/11/22", photodir), 5, false)

	// for preview generation
	time.Sleep(10 * time.Second)

	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/preview/2003/11/22", photodir), 0, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/preview/2003/11/22", photodir), 15, false)

	// probe new preview video dimension
	for i, a := range assets {
		idx := strconv.Itoa(i + 1)
		info, err := engine.ProbeVideoInfo(photodir + "/alice/Photos/preview/2003/11/22/20031122_" + idx + "_320_0.mp4")
		c.Assert(err, IsNil)
		c.Assert(info.Streams[0].Width, Equals, a.sw)
		c.Assert(info.Streams[0].Height, Equals, a.sh)
		c.Assert(info.Streams[0].Tags.Rotate, Equals, "")
	}

	// if video preview files are not exist, need download original one automatically
	c.Assert(os.RemoveAll(photodir+`/alice/Photos/preview/2003/11/22`), IsNil)

	for i, a := range assets {
		idx := strconv.Itoa(i + 1)
		ts.assetGet(c, "/asset/"+idx+`?token=`+ts.token, a.sha)
	}
}

func (ts *mainSuite) TestVideoPreviewLowerResolution(c *C) {
	assets := []info{
		{
			f:   "../cmd/lomod/test/video/13_2014_01_21.3gp",
			sha: "491154fee1b01c2e41fb93dc41843f5e77c1732b",
			w:   352,
			h:   288,
			r:   "",
			s:   1252077,
			sw:  480,
			sh:  392,
		},
	}
	// use higher resolution
	ts.h.previewRunner.VideoDims = []types.Dimension{{Width: 480}}
	engine := avutil.DefaultEngine(ts.h.transcodeApp, ts.h.ffprobe)

	for i, a := range assets {
		info, err := engine.ProbeVideoInfo(a.f)
		c.Assert(err, IsNil)
		c.Assert(info.Streams[0].Width, Equals, a.w)
		c.Assert(info.Streams[0].Height, Equals, a.h)
		c.Assert(info.Streams[0].Tags.Rotate, Equals, a.r)
		url := fmt.Sprintf("/asset/%s?token=%s&ext=3gp&createtime=2003-11-22T00:01:00Z", a.sha, ts.token)
		ai := ts.importAsset(c, a.f, url)

		idx := strconv.Itoa(i + 1)
		c.Assert(ai.Name, Equals, idx+".3gp")
		c.Assert(ai.Hash, Equals, a.sha)
		ts.validateAsset(c, fmt.Sprintf("%s/alice/Photos/master/2003/11/22/20031122_"+idx+".3gp", photodir), a.s)
	}
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2003/11/22", photodir), 0, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2003/11/22", photodir), 1, false)

	// for preview generation
	ctx, _ := context.WithTimeout(context.Background(), 20*time.Second)
	c.Assert(testutil.WaitWithFileCounts(ctx, photodir+"/alice/Photos/preview/2003/11/22", 3), IsNil)

	// probe new preview video dimension
	for i, a := range assets {
		idx := strconv.Itoa(i + 1)
		info, err := engine.ProbeVideoInfo(photodir + "/alice/Photos/preview/2003/11/22/20031122_" + idx + "_480_0.mp4")
		c.Assert(err, IsNil)
		c.Assert(info.Streams[0].Width, Equals, a.sw)
		c.Assert(info.Streams[0].Height, Equals, a.sh)
		c.Assert(info.Streams[0].Tags.Rotate, Equals, "")
	}
}

func (ts *mainSuite) TestNoVideoPreview(c *C) {
	assets := []info{
		{
			f:   "../cmd/lomod/test/video/13_2014_01_21.3gp",
			sha: "491154fee1b01c2e41fb93dc41843f5e77c1732b",
			w:   352,
			h:   288,
			r:   "",
			s:   1252077,
			sw:  480,
			sh:  392,
		},
	}
	// use higher resolution
	ts.h.previewRunner.VideoDims = nil
	engine := avutil.DefaultEngine(ts.h.transcodeApp, ts.h.ffprobe)

	for i, a := range assets {
		info, err := engine.ProbeVideoInfo(a.f)
		c.Assert(err, IsNil)
		c.Assert(info.Streams[0].Width, Equals, a.w)
		c.Assert(info.Streams[0].Height, Equals, a.h)
		c.Assert(info.Streams[0].Tags.Rotate, Equals, a.r)
		url := fmt.Sprintf("/asset/%s?token=%s&ext=3gp&createtime=2003-11-22T00:01:00Z", a.sha, ts.token)
		ai := ts.importAsset(c, a.f, url)

		idx := strconv.Itoa(i + 1)
		c.Assert(ai.Name, Equals, idx+".3gp")
		c.Assert(ai.Hash, Equals, a.sha)
		ts.validateAsset(c, fmt.Sprintf("%s/alice/Photos/master/2003/11/22/20031122_"+idx+".3gp", photodir), a.s)
	}
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2003/11/22", photodir), 0, true)
	ts.validateDir(c, fmt.Sprintf("%s/alice/Photos/master/2003/11/22", photodir), 1, false)

	// should have none preview generation
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	c.Assert(testutil.WaitWithFileCounts(ctx, photodir+"/alice/Photos/preview/2003/11/22", 1), NotNil)

	c.Assert(testutil.ValidateFilesInDir(photodir+"/alice/Photos/preview/", 1, true), IsNil)

	// download original video should be good
	u := "/asset/" + assets[0].sha + `?token=` + ts.token + "&" + common.QueryKeyOrig + "=1"
	ts.assetGet(c, u, assets[0].sha)

	// download preview video should be original video
}

func (ts *mainSuite) TestLomoOrigSHA(c *C) {
	// only support mov file type, so mp4 should fail
	u := fmt.Sprintf("/asset?token=%s&ext=mp4&createtime=2013-08-08T08:08:08Z&%s=%s&sha1=%s", ts.token, common.QueryKeyFileHash, "69a65215eea4def69e1cfd3fbf4295ebd2dfa714", "b12fb8a5bb04738efe66f186a45f0388d8fabff1")
	ts.importAssetWithResult(c, "../cmd/lomod/test/video/lomo_orig_sha.mov", u, "POST",
		`{"id": "24", "text": "[save asset b12fb8a5bb04738efe66f186a45f0388d8fabff1]: Uploaded asset has different hash"}
`, http.StatusBadRequest, nil)

	// wrong original or file sha should not succes
	u = fmt.Sprintf("/asset?token=%s&ext=mov&createtime=2013-08-08T08:08:08Z&%s=%s&sha1=%s", ts.token, common.QueryKeyFileHash, "b12fb8a5bb04738efe66f186a45f0388d8fabff1", "b12fb8a5bb04738efe66f186a45f0388d8fabff1")
	ts.importAssetWithResult(c, "../cmd/lomod/test/video/lomo_orig_sha.mov", u, "POST",
		`{"id": "24", "text": "[save asset b12fb8a5bb04738efe66f186a45f0388d8fabff1]: Uploaded asset has different hash"}
`, http.StatusBadRequest, nil)

	u = fmt.Sprintf("/asset?token=%s&ext=mov&createtime=2013-08-08T08:08:08Z&%s=%s&sha1=%s", ts.token, common.QueryKeyFileHash, "69a65215eea4def69e1cfd3fbf4295ebd2dfa714", "69a65215eea4def69e1cfd3fbf4295ebd2dfa714")
	ts.importAssetWithResult(c, "../cmd/lomod/test/video/lomo_orig_sha.mov", u, "POST",
		`{"id": "0", "text": "[save asset 69a65215eea4def69e1cfd3fbf4295ebd2dfa714]: ValidateLomorageOriginSHA read SHA: b12fb8a5bb04738efe66f186a45f0388d8fabff1, but expect 69a65215eea4def69e1cfd3fbf4295ebd2dfa714"}
`, http.StatusInternalServerError, nil)

	u = fmt.Sprintf("/asset?token=%s&ext=mov&createtime=2013-08-08T08:08:08Z&%s=%s&sha1=%s", ts.token, common.QueryKeyFileHash, "69a65215eea4def69e1cfd3fbf4295ebd2dfa714", "b12fb8a5bb04738efe66f186a45f0388d8fabff1")
	ts.createAsset(c, "../cmd/lomod/test/video/lomo_orig_sha.mov", u, "", "b12fb8a5bb04738efe66f186a45f0388d8fabff1")

	// without filesha should be success too
	u = fmt.Sprintf("/asset?token=%s&ext=mov&createtime=2021-01-01T08:08:08Z&sha1=%s", ts.token, "69a65215eea4def69e1cfd3fbf4295ebd2dfa714")
	ts.createAsset(c, "../cmd/lomod/test/video/lomo_orig_sha.mov", u, "", "69a65215eea4def69e1cfd3fbf4295ebd2dfa714")

	// wait until preview generation is done
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c.Assert(testutil.WaitWithFileCounts(ctx, photodir+"/alice/Photos/preview/2021/01/01", 3), IsNil)
}

func (ts *mainSuite) TestAssetConcurrentUpload(c *C) {
	// concurrent 50 upload and check the result
	count := 50
	assets := map[string]string{}
	tmpdir, err := ioutil.TempDir("", "")
	c.Assert(err, IsNil)
	defer os.RemoveAll(tmpdir)

	for i := 0; i < count; i++ {
		filename := filepath.Join(tmpdir, common.RandomString(8)+".mp4")
		c.Assert(cmd.Exec("cp", "../cmd/lomod/test/video/10_2013_08_08.mp4", filename), IsNil)
		c.Assert(cmd.Exec("exiftool", "-Title="+filename, filename), IsNil)

		f, err := os.Open(filename)
		c.Assert(err, IsNil)

		h := sha1.New()
		_, err = io.Copy(h, f)
		c.Assert(err, IsNil)
		c.Assert(f.Close(), IsNil)

		assets[filename] = fmt.Sprintf("%x", h.Sum(nil))
	}

	srv := httptest.NewServer(ts.h.CreateRouter())
	defer srv.Close()
	done := make(chan struct{}, count)
	for filename, sha := range assets {
		go func(filename, sha string) {
			f, err := os.Open(filename)
			c.Assert(err, IsNil)
			defer f.Close()

			res, err := http.Post(srv.URL+"/asset/"+sha+"?ext=mp4&createtime=2013-11-23T12:00:00Z&token="+ts.token, "application/octet-stream", f)
			c.Assert(err, IsNil)
			ai := types.Asset{}
			c.Assert(json.NewDecoder(res.Body).Decode(&ai), IsNil)
			c.Assert(ai.Hash, Equals, sha)
			res.Body.Close()

			// download and verify
			res, err = http.Get(srv.URL + "/asset/" + ai.Name + "?orig=1&token=" + ts.token)
			c.Assert(err, IsNil)

			h := sha1.New()
			_, err = io.Copy(h, res.Body)
			c.Assert(err, IsNil)
			res.Body.Close()

			c.Assert(fmt.Sprintf("%x", h.Sum(nil)), Equals, sha, Commentf("%s - %s", sha, filename))

			done <- struct{}{}
		}(filename, sha)
	}
	for i := 0; i < count; i++ {
		<-done
	}

	//
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	c.Assert(testutil.WaitWithFileCounts(ctx, photodir+"/alice/Photos/preview/2013/11/23", count*3), IsNil)
}

func (ts *mainSuite) insertAssetsWithGPS(c *C) {
	for idx, f := range gpsAssets {
		e := strings.TrimPrefix(filepath.Ext(f.file), ".")
		u := fmt.Sprintf("/asset?token=%s&ext=%s&createtime=%s&sha1=%s", ts.token, e, f.time, f.hash)
		ts.createAsset(c, f.file, u, strconv.Itoa(idx+1)+"."+e, f.hash)
	}
}

func (ts *mainSuite) waitPreviewAssetsWithGPS(ctx context.Context, c *C) {
	c.Assert(testutil.WaitWithFileCounts(ctx, photodir+"/alice/Photos/preview", 2*len(gpsAssets)), IsNil)
}

func (ts *mainSuite) TestPreviewGPS(c *C) {
	ts.h.conf.UseJpg = true
	defer func() {
		ts.h.conf.UseJpg = false
	}()

	ts.insertAssetsWithGPS(c)

	// verify master image GPS tag before test preview
	dir1 := photodir + "/alice/Photos/master/2003/11/23"
	exifTags, err := exif.NewTags(filepath.Join(dir1, "20031123_1.jpg"), ts.h.exiftool, "jpg")
	c.Assert(err, IsNil)
	c.Assert(exifTags.GetLatitude(), Equals, 39.9155555555556)
	c.Assert(exifTags.GetLongitude(), Equals, 116.390833333333)

	dir2 := photodir + "/alice/Photos/master/2017/09/13"
	exifTags, err = exif.NewTags(filepath.Join(dir2, "20170913_2.heic"), ts.h.exiftool, "heic")
	c.Assert(err, IsNil)
	c.Assert(exifTags.GetLatitude(), Equals, -23.5398944444444)
	c.Assert(exifTags.GetLongitude(), Equals, -46.65655)

	dir := photodir + "/alice/Photos/preview"
	c.Assert(testutil.WaitWithFileCounts(context.Background(), dir, 4), IsNil)

	dir1 = photodir + "/alice/Photos/preview/2003/11/23"
	dir2 = photodir + "/alice/Photos/preview/2017/09/13"
	for _, f := range []string{
		filepath.Join(dir1, "20031123_1_480_320.jpg"),
		filepath.Join(dir1, "20031123_1_75_75.jpg"),
		filepath.Join(dir2, "20170913_2_480_320.jpg"),
		filepath.Join(dir2, "20170913_2_75_75.jpg"),
	} {
		exifTags, err = exif.NewTags(f, ts.h.exiftool, "jpg")
		c.Assert(err, IsNil)
		c.Assert(exifTags.GetLatitude(), Equals, exif.UnknownGPS)
		c.Assert(exifTags.GetLongitude(), Equals, exif.UnknownGPS)
	}
}

func (ts *mainSuite) TestOrientation(c *C) {
	ts.createAllAssets(c)

	ts.testOrientation(c, 1, 1, exif.Orientation)
	ts.testOrientation(c, 2, 1, exif.Orientation)
	ts.testOrientation(c, 3, 1, exif.Orientation)
	ts.testOrientation(c, 4, 1, exif.Orientation)
	ts.testOrientation(c, 5, 1, exif.Orientation)
	ts.testOrientation(c, 6, 0, "")
	ts.testOrientation(c, 7, 0, "")
	ts.testOrientation(c, 8, 1, exif.Orientation)
	ts.testOrientation(c, 9, 1, exif.Orientation)
	ts.testOrientation(c, 10, 1, exif.Orientation)
	ts.testOrientation(c, 11, 1, exif.Orientation)
	ts.testOrientation(c, 12, 1, exif.Orientation)
	ts.testOrientation(c, 13, 1, exif.Orientation)
	// TODO: is rotation handled by vips
	//ts.testOrientation(c, 13, 180, exif.Rotation)
}

func (ts *mainSuite) testOrientation(c *C, id, expectOrienValue int, expectOrienName string) {
	u := fmt.Sprintf("/asset/preview/%d?token=%s", id, ts.token)
	req, err := http.NewRequest("GET", u, nil)
	c.Assert(err, IsNil)

	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	validateStatusCode(c, u, rr, http.StatusOK)

	resp := rr.Result()
	defer resp.Body.Close()

	tmpfile, err := ioutil.TempFile("", "")
	c.Assert(err, IsNil)
	_, err = io.Copy(tmpfile, resp.Body)
	c.Assert(err, IsNil)
	c.Assert(tmpfile.Close(), IsNil)
	defer os.Remove(tmpfile.Name())

	tags, err := exif.NewTags(tmpfile.Name(), "exiftool", "")
	c.Assert(err, IsNil)
	n, v := tags.GetOrientation()
	c.Assert(n, Equals, expectOrienName)
	c.Assert(v, Equals, expectOrienValue)
}

func (ts *mainSuite) TestAssetDeleteRepeat(c *C) {
	category, err := loadCategory()
	c.Assert(err, IsNil)

	ts.createAssets(c, &category, ts.token)
	ts.validateAssets(c, 1)
	ts.waitPreviewComplete(c, false)

	// delete all assets
	for i := 1; i <= 10; i++ {
		ts.requestDelete(c, ts.token, &types.DeleteAssetItems{
			List: []types.DeleteAssetItem{{ID: strconv.Itoa(i)}},
		})
	}

	// should get empty now
	var (
		year  types.Year
		years types.Years
	)
	ts.requestGet(c, fmt.Sprintf("/category/2003?token=%s", ts.token), &year, &types.Year{Year: 2003, Months: []types.Month{}})
	ts.requestGet(c, fmt.Sprintf("/category/2004?token=%s", ts.token), &year, &types.Year{Year: 2004, Months: []types.Month{}})
	ts.requestGet(c, fmt.Sprintf("/category/2013?token=%s", ts.token), &year, &types.Year{Year: 2013, Months: []types.Month{}})

	ts.requestGet(c, fmt.Sprintf("/category?token=%s", ts.token), &years, &types.Years{Years: []types.Year{}})

	// upload again should be success
	category.Years[0].Months[1].Days[1].Assets[0].Name = "11.jpg"
	category.Years[0].Months[1].Days[0].Assets[1].Name = "12.jpg"
	category.Years[0].Months[1].Days[0].Assets[0].Name = "13.jpg"
	category.Years[0].Months[0].Days[0].Assets[0].Name = "14.jpg"
	category.Years[1].Months[0].Days[0].Assets[0].Name = "15.jpg"
	category.Years[2].Months[1].Days[0].Assets[0].Name = "16.mp4"
	category.Years[2].Months[0].Days[0].Assets[0].Name = "17.png"
	category.Years[2].Months[2].Days[0].Assets[0].Name = "19.heic"
	category.Years[2].Months[2].Days[0].Assets[1].Name = "20.zip"
	category.Years[2].Months[2].Days[0].Assets[2].Name = "18.zip"

	ts.createAssets(c, &category, ts.token)
	ts.validateAssets(c, 11)

	ts.validateBasicCategory(c, category)
	ts.waitPreviewComplete(c, false)
}

func (ts *mainSuite) TestAssetSize(c *C) {
	category, err := loadCategory()
	c.Assert(err, IsNil)

	// zero assets should return nil
	ts.h.migrate()
	result := ts.listSystemInfo(c, "/system?token="+ts.token)
	c.Assert(result.UserStatus, NotNil)
	c.Assert(result.UserStatus[ts.alice.Name].AssetSummary, IsNil)
	c.Assert(result.UserStatus[ts.bob.Name].AssetSummary, IsNil)

	ts.createAssets(c, &category, ts.tokenBob)

	ts.validateAssetSize(c)

	// reset size and run migration to validate again
	c.Assert(dbx.InQuery(ts.h.db, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "update asset set size = 0")
		return err
	}), IsNil)

	ts.h.userAssetSummary = map[int]map[string]types.AssetSummary{}
	ts.h.migrate()
	ts.validateAssetSize(c)
}

func (ts *mainSuite) validateAssetSize(c *C) {
	c.Assert(dbx.InQuery(ts.h.db, func(ctx context.Context, tx *sql.Tx) error {
		size := 0
		for i, a := range assetsShort {
			if err := tx.QueryRowContext(ctx, "select size from asset where id = ?", i+1).Scan(&size); err != nil {
				return err
			}
			stat, err := os.Stat(a.file)
			if err != nil {
				return err
			}
			if size != int(stat.Size()) {
				return errors.Errorf("import %s, expect %d, obtain %d", a.file, stat.Size(), size)
			}
		}
		return nil
	}), IsNil)

	result := ts.listSystemInfo(c, "/system?token="+ts.token)
	c.Assert(result.UserStatus, NotNil)
	as := result.UserStatus[ts.alice.Name].AssetSummary
	c.Assert(as, NotNil)
	c.Assert(as, DeepEquals, map[string]types.AssetSummary{"jpg": {Count: 3, Size: 1438014},
		"mp4": {Count: 1, Size: 7360}, "png": {Count: 1, Size: 44969}, "zip": {Count: 1, Size: 238254}})
	as = result.UserStatus[ts.bob.Name].AssetSummary
	c.Assert(as, NotNil)
	c.Assert(as, DeepEquals, map[string]types.AssetSummary{"jpg": {Count: 2, Size: 318272},
		"heic": {Count: 1, Size: 1051433}, "zip": {Count: 1, Size: 1055380}})
}

func (ts *mainSuite) TestAssetUpdateTime(c *C) {
	// should work for any assets. Use this for easy import
	ts.insertAssetsWithGPS(c)

	prefix := "20031123_1"
	dir := photodir + "/alice/Photos/master/2003/11/23"
	fi, err := os.Stat(dir + "/" + prefix + ".jpg")
	c.Assert(err, IsNil)
	masterSize := fi.Size()

	dir = photodir + "/alice/Photos/preview/"
	c.Assert(testutil.WaitWithFileCounts(context.Background(), dir, 4), IsNil)
	fis, err := ioutil.ReadDir(filepath.Join(dir, "2003", "11", "23"))
	c.Assert(err, IsNil)
	sizes := map[string]int64{}
	for _, fi := range fis {
		if !strings.HasPrefix(fi.Name(), prefix) {
			continue
		}
		sizes[strings.TrimPrefix(fi.Name(), prefix)] = fi.Size()
	}

	url := fmt.Sprintf("/asset/%s/2021/2/1?token=%s", gpsAssets[0].hash, ts.token)
	req, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, IsNil)

	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	validateStatusCode(c, url, rr, http.StatusOK)

	// master and preview should be removed
	dir = photodir + "/alice/Photos/master/2003/11/23"
	_, err = os.Stat(dir + "/" + prefix + ".jpg")
	c.Assert(err, NotNil)
	c.Assert(os.IsNotExist(err), Equals, true)

	dir = photodir + "/alice/Photos/preview/2003/11/23"
	fis, err = ioutil.ReadDir(dir)
	c.Assert(err, IsNil)
	c.Assert(len(fis), Equals, 0, Commentf("%v", fis))
	for _, fi := range fis {
		c.Assert(strings.HasPrefix(fi.Name(), prefix), Equals, false, Commentf("%s", fi.Name()))
	}

	// check new directory
	prefix = "20210201_1"
	dir = photodir + "/alice/Photos/master/2021/02/01"
	fi, err = os.Stat(dir + "/" + prefix + ".jpg")
	c.Assert(err, IsNil)
	c.Assert(fi.Size(), Equals, masterSize)

	dir = photodir + "/alice/Photos/preview/2021/02/01"
	c.Assert(testutil.WaitWithFileCounts(context.Background(), dir, 2), IsNil)
	fis, err = ioutil.ReadDir(dir)
	c.Assert(err, IsNil)
	c.Assert(len(fis), Equals, 2, Commentf("%v", fis))
	for _, fi := range fis {
		c.Assert(strings.HasPrefix(fi.Name(), prefix), Equals, true, Commentf("%s", fi.Name()))
		c.Assert(sizes[strings.TrimPrefix(fi.Name(), prefix)], Equals, fi.Size())
	}

	// still able to download file
	for idx, f := range gpsAssets {
		ts.assetGet(c, "/asset/"+strconv.Itoa(idx+1)+`?token=`+ts.token, f.hash)
	}

	// test new category
	years := types.Years{Hash: "4b0f4727527b6d1d94e1508f592e04daecfac9f0", Years: []types.Year{
		{Year: 2003, Hash: "6cbc0a6630b5de7c844dd148352d197bae14643b", Months: []types.Month{
			{Month: 11, Hash: "9d716ac24587512316705483aa1f846a0345c248", Days: []types.Day{}},
		}},
		{Year: 2021, Hash: "1a7f7c7adf7e1fbcc44d83fc89b95d7815d57bf1", Months: []types.Month{
			{Month: 2, Hash: "fc61d9e5151f626737395e8fe56645f9c22dd8fb", Days: []types.Day{}},
		}},
	}}
	ts.requestGet(c, fmt.Sprintf("/category?token=%s", ts.token), &years, &years)

	createTime, err := time.Parse(common.TimeFormatLomod, "2003-11-23T12:00:00Z")
	c.Assert(err, IsNil)
	lomoTime := types.LomoTime{Time: createTime}
	year := types.Year{
		Year: 2003, Hash: "6cbc0a6630b5de7c844dd148352d197bae14643b", Months: []types.Month{
			{Month: 11, Hash: "9d716ac24587512316705483aa1f846a0345c248", Days: []types.Day{
				{Day: 23, Hash: "9b882943326363e0633bc3b03f9da5dc9086186c", Assets: []types.Asset{
					{Name: "2.heic", Hash: gpsAssets[1].hash, Date: lomoTime},
				}},
			}},
		}}
	ts.requestGet(c, fmt.Sprintf("/category/2003?token=%s", ts.token), &year, &year)

	createTime, err = time.Parse(common.TimeFormatLomod, "2021-02-01T12:00:00Z")
	c.Assert(err, IsNil)
	lomoTime.Time = createTime
	year = types.Year{
		Year: 2021, Hash: "1a7f7c7adf7e1fbcc44d83fc89b95d7815d57bf1", Months: []types.Month{
			{Month: 2, Hash: "fc61d9e5151f626737395e8fe56645f9c22dd8fb", Days: []types.Day{
				{Day: 1, Hash: "ee8e87bb216aa46de86501f4a4c5a27d00aff155", Assets: []types.Asset{
					{Name: "1.jpg", Hash: gpsAssets[0].hash, Date: lomoTime},
				}},
			}},
		}}
	ts.requestGet(c, fmt.Sprintf("/category/2021?token=%s", ts.token), &year, &year)

	// other year should be empty
	ts.requestGet(c, fmt.Sprintf("/category/2004?token=%s", ts.token), &year, &types.Year{Year: 2004, Months: []types.Month{}})
	ts.requestGet(c, fmt.Sprintf("/category/2020?token=%s", ts.token), &year, &types.Year{Year: 2020, Months: []types.Month{}})
}

func (ts *mainSuite) TestAssetDeleteFromHost(c *C) {
	// this testcase delete file from filesystem, and request get should return 404
	ts.insertAssetsWithGPS(c)

	dir := photodir + "/alice/Photos"
	c.Assert(testutil.WaitWithFileCounts(context.Background(), dir, 6), IsNil)

	c.Assert(os.RemoveAll(dir), IsNil)

	for _, p := range []string{
		"1", "preview/1",
		"2", "preview/2",
	} {
		u := fmt.Sprintf("/asset/%s?token=%s", p, ts.token)
		req, err := http.NewRequest("GET", u, nil)
		c.Assert(err, IsNil)

		rr := httptest.NewRecorder()
		ts.h.CreateRouter().ServeHTTP(rr, req)
		validateStatusCode(c, u, rr, http.StatusNotFound)
	}
}

func (ts *mainSuite) TestAssetAspectRatioUpload(c *C) {
	notify := make(chan types.PreviewRequest)
	count := 0
	go func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			case <-notify:
				count++
			}
		}
	}(ts.h.gCtx)

	ts.restartPreviewRunnerForAspectRatio(c, notify)

	// start insert content
	totalCount := ts.createAllAssets(c)

	for i := 0; i < 60; i++ {
		if count == totalCount {
			break
		}
		time.Sleep(time.Second)
	}
	c.Assert(count, Equals, totalCount)
	// wait until all aspect ratio update is done

	ts.validateAssetAspectRatio(c, map[int]float32{
		1:  1.33333337306976,
		2:  1.56097555160522,
		3:  1.777777777777778,
		4:  0.670312523841858,
		5:  1.777777777777778,
		6:  1.777777777777778,
		7:  1.65374677002584,
		8:  0.670312523841858,
		9:  0.75,
		10: 0.75,
		11: 1.4953271150589,
		12: 1.4953271150589,
		13: 1.33333337306976,
	})
}

func (ts *mainSuite) TestAssetAspectRatioScan(c *C) {
	notify := make(chan types.PreviewRequest)
	count := 0
	go func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			case n := <-notify:
				fmt.Printf("preview generation notify: %s\n", n.MasterPath)
				count++
			}
		}
	}(ts.h.gCtx)

	ts.restartPreviewRunnerForAspectRatio(c, notify)

	// start insert content
	totalCount := 0
	category := ts.scanAndImport(c, false, true, true)
	for _, y := range category.Years {
		for _, m := range y.Months {
			for _, d := range m.Days {
				totalCount += len(d.Assets)
			}
		}
	}

	for i := 0; i < 90; i++ {
		if count == totalCount {
			break
		}
		time.Sleep(time.Second)
	}
	c.Assert(count, Equals, totalCount)
	// wait until all aspect ratio update is done

	ts.validateAssetAspectRatio(c, map[int]float32{
		1:  1.77777779102325,
		2:  1.22137403488159,
		3:  0.562390148639679,
		4:  1.77777779102325,
		5:  1.81303119659424,
		6:  1.77777779102325,
		7:  1.77777779102325,
		8:  1.81303119659424,
		9:  0.562390148639679,
		10: 0.562390148639679,
		11: 0.551562488079071,
		12: 0.562390148639679,
		13: 0.562390148639679,
		14: 0.551562488079071,
		15: 0.562390148639679,
		16: 0.562390148639679,
		17: 0.562390148639679,
		18: 1.33333337306976,
		19: 0.562390148639679,
		20: 1.4953271150589,
		21: 1.33333337306976,
		22: 0.75,
		23: 1.50234746932983,
		24: 0.670312523841858,
		25: 1.77777779102325,
		26: 1.56097555160522,
		27: 1.33333337306976,
		28: 1.77777779102325,
		29: 0.75,
		30: 1.4953271150589,
		31: 1.65374672412872,
		32: 1.33333337306976,
	})
}

func (ts *mainSuite) restartPreviewRunnerForAspectRatio(c *C, notify chan types.PreviewRequest) {
	ts.h.previewRunner.Stop()

	// set notify channel, proxy is to intercept the notify and calculate number of event
	// worker is the real notify channel
	notifyProxy := make(chan types.PreviewRequest)
	notifyWorker := make(chan types.PreviewRequest)
	count := 0
	go func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			case req := <-notifyProxy:
				notifyWorker <- req
				notify <- req
				count++
			}
		}
	}(ts.h.gCtx)
	previewImgDims := []types.Dimension{{Width: 75, Height: 75}, {Width: common.MaxPreviewWidth}}
	previewVideoDims := []types.Dimension{{Width: common.DefaultVideoPreviewWidth}}
	go ts.h.postPreviewGeneration(ts.h.gCtx, notifyWorker, common.MaxPreviewWidth, common.DefaultVideoPreviewWidth)
	ts.h.previewRunner = preview.NewRunner(ts.h.gCtx, ts.h.transcodeApp, ts.h.ffprobe, ts.h.exiftool,
		common.DefaultFolderPermission, previewImgDims, previewVideoDims, 1, &preview.NotifyConfig{
			ImageWidth: common.MaxPreviewWidth,
			VideoWidth: common.DefaultVideoPreviewWidth,
			Chan:       notifyProxy,
		})
	go ts.h.previewRunner.Start()
}

// start insert content
func (ts *mainSuite) validateAssetAspectRatio(c *C, ids map[int]float32) {
	for id, ar := range ids {
		for i := 0; i < 60; i++ {
			wait := false
			c.Assert(dbx.InQuery(ts.h.db, func(ctx context.Context, tx *sql.Tx) error {
				var obtAR float32
				err := tx.QueryRowContext(ctx, "select aspect_ratio from asset where id = ?", id).Scan(&obtAR)
				if err != nil {
					return err
				}
				if obtAR == 0 {
					wait = true
					return nil
				} else if obtAR == ar {
					return nil
				}
				return errors.Errorf("%d expect %f, got %f", id, ar, obtAR)
			}), IsNil)
			if wait {
				time.Sleep(time.Second)
				continue
			}
			break
		}
	}
}

func (ts *mainSuite) TestAssetAspectRatioMigrate(c *C) {
	ts.h.previewRunner.Stop()

	previewImgDims := []types.Dimension{{Width: 75, Height: 75}, {Width: common.MaxPreviewWidth}}
	previewVideoDims := []types.Dimension{{Width: common.DefaultVideoPreviewWidth}}
	ts.h.previewRunner = preview.NewRunner(ts.h.gCtx, ts.h.transcodeApp, ts.h.ffprobe, ts.h.exiftool,
		common.DefaultFolderPermission, previewImgDims, previewVideoDims, 1, nil)
	go ts.h.previewRunner.Start()

	category, err := loadCategory()
	c.Assert(err, IsNil)

	ts.createAssets(c, &category, ts.token)
	ts.validateAssets(c, 1)
	ts.waitPreviewComplete(c, false)

	ts.h.migrateAssetAspectRatio()

	ts.validateAssetAspectRatio(c, map[int]float32{
		1:  1.33333337306976,
		2:  1.56097555160522,
		3:  1.777777777777778,
		4:  0.670312523841858,
		5:  1.777777777777778,
		6:  1.777777777777778,
		7:  1.65374677002584,
		8:  0.670312523841858,
		9:  0.75,
		10: 0.75,
	})
}

func (ts *mainSuite) TestAssetEXIF(c *C) {
	// should work for any assets. Use this for easy import
	ts.insertAssetsWithGPS(c)

	exifs := map[int]exif.RawTags{}
	c.Assert(dbx.InQuery(ts.h.db, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "select id, exif from asset")
		if err != nil {
			return err
		}
		for rows.Next() {
			var (
				id      int
				content string
			)
			err = rows.Scan(&id, &content)
			if err != nil {
				return err
			}
			tags, err := exif.NewRawTags(content)
			c.Assert(err, IsNil)
			tags.SetDateZero()
			exifs[id] = tags
		}
		return nil
	}), IsNil)

	expectExifs := map[int]exif.RawTags{}
	for i, a := range gpsAssets {
		tags, err := exif.NewRawTags(a.exif)
		c.Assert(err, IsNil)
		tags.SetDateZero()
		expectExifs[i+1] = tags
	}
	c.Assert(exifs, DeepEquals, expectExifs)
}
