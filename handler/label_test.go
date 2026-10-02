package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"bitbucket.org/lomoware/lomo-backend/common/types"
	. "gopkg.in/check.v1"
)

func (ts *mainSuite) createLabel(c *C, l *types.AssetLabel) {
	contents, err := json.Marshal(l)
	c.Assert(err, IsNil)
	buf := bytes.NewBuffer(contents)
	var res int
	ts.requestWithMethodBody(c, "/assets/label?token="+ts.token, http.MethodPost, http.StatusOK, buf, &res, nil, nil)
	c.Assert(res, Not(Equals), 0)
	l.ID = res
}

func (ts *mainSuite) listLabels(c *C) []*types.AssetLabel {
	as := []*types.AssetLabel{}
	ts.requestGet(c, "/assets/label?token="+ts.token, &as, nil)
	return as
}

func (ts *mainSuite) listAssetsInLabel(c *C, labelID int, token string) []types.AssetNameConfidence {
	as := []types.AssetNameConfidence{}
	ts.requestGet(c, fmt.Sprintf("/assets/label/%d?token=%s", labelID, token), &as, nil)
	return as
}

func (ts *mainSuite) listLabelsForAsset(c *C, assetID int, token string) []types.AssetLabelConfidence {
	as := []types.AssetLabelConfidence{}
	ts.requestGet(c, fmt.Sprintf("/asset/label/%d?token=%s", assetID, token), &as, nil)
	return as
}

func (ts *mainSuite) addAssetsInLabel(c *C, labelID int, aids []string, token string) {
	content, err := json.Marshal(aids)
	c.Assert(err, IsNil)
	ts.requestWithMethodBody(c, fmt.Sprintf("/assets/label/%d?token=%s", labelID, token), http.MethodPost, http.StatusOK,
		bytes.NewBuffer(content), nil, nil, nil)
}

func (ts *mainSuite) removeAssetsFromLabel(c *C, labelID int, aids []string, token string) {
	content, err := json.Marshal(aids)
	c.Assert(err, IsNil)
	ts.requestWithMethodBody(c, fmt.Sprintf("/assets/label/%d?token=%s", labelID, token), http.MethodDelete, http.StatusOK,
		bytes.NewBuffer(content), nil, nil, nil)
}

func (ts *mainSuite) TestLabelBasic(c *C) {
	ts.insertAssetsWithGPS(c)

	l := &types.AssetLabel{Label: "hi", LabelCN: "测试"}
	labels := ts.listLabels(c)
	found := false
	for _, label := range labels {
		if label.Label == l.Label && label.LabelCN == l.LabelCN {
			found = true
			break
		}
	}
	c.Assert(found, Equals, false)

	// 1. add label
	l.ID = len(labels) + 1
	ts.createLabel(c, l)

	labels = ts.listLabels(c)

	found = false
	for _, label := range labels {
		c.Assert(label.AssetsCount, Equals, 0)
		if label.Label == l.Label && label.LabelCN == l.LabelCN {
			found = true
			break
		}
	}
	c.Assert(found, Equals, true, Commentf("%v", labels))

	// 2. add assets in label
	assets := ts.listAssetsInLabel(c, l.ID, ts.token)
	c.Assert(len(assets), Equals, 0)

	ts.addAssetsInLabel(c, l.ID, []string{"1.jpg", gpsAssets[1].hash}, ts.token)
	assets = ts.listAssetsInLabel(c, l.ID, ts.token)

	c.Assert(assets, DeepEquals, []types.AssetNameConfidence{
		{Name: "2.heic", Hash: gpsAssets[1].hash, Confidence: 100.0},
		{Name: "1.jpg", Hash: gpsAssets[0].hash, Confidence: 100.0},
	})

	// 3. list labels belonging to the asset
	for i := 1; i <= 2; i++ {
		labels := ts.listLabelsForAsset(c, i, ts.token)
		c.Assert(labels, DeepEquals, []types.AssetLabelConfidence{
			{ID: l.ID, Confidence: 100.0},
		})
	}

	// 4. list current labels, and count should change
	labels = ts.listLabels(c)
	found = false
	for _, label := range labels {
		if label.Label == l.Label && label.LabelCN == l.LabelCN {
			found = true
			c.Assert(label.AssetsCount, Equals, 2)
			break
		}
	}
	c.Assert(found, Equals, true)

	// 5. remove assets from label
	ts.removeAssetsFromLabel(c, l.ID, []string{gpsAssets[1].hash}, ts.token)
	assets = ts.listAssetsInLabel(c, l.ID, ts.token)
	c.Assert(assets, DeepEquals, []types.AssetNameConfidence{
		{Name: "1.jpg", Hash: gpsAssets[0].hash, Confidence: 100.0},
	})

	// 6. delete asset should remove it from associated labels too.
	ts.requestDeleteByID(c, fmt.Sprintf("/asset/%d?token=%s", 1, ts.token))
	assets = ts.listAssetsInLabel(c, l.ID, ts.token)
	c.Assert(len(assets), Equals, 0)

	// 7. list labels again, and asset count should be zero now
	labels = ts.listLabels(c)

	found = false
	for _, label := range labels {
		c.Assert(label.AssetsCount, Equals, 0)
	}
	c.Assert(found, Equals, false)

	// 8. add asset 2 label again, then try another delete API, and see if it works
	ts.addAssetsInLabel(c, l.ID, []string{gpsAssets[1].hash}, ts.token)
	assets = ts.listAssetsInLabel(c, l.ID, ts.token)
	c.Assert(assets, DeepEquals, []types.AssetNameConfidence{
		{Name: "2.heic", Hash: gpsAssets[1].hash, Confidence: 100.0},
	})

	labelAssets := ts.listLabelsForAsset(c, 2, ts.token)
	c.Assert(labelAssets, DeepEquals, []types.AssetLabelConfidence{
		{ID: l.ID, Confidence: 100.0},
	})

	labels = ts.listLabels(c)
	found = false
	for _, label := range labels {
		if label.Label == l.Label && label.LabelCN == l.LabelCN {
			found = true
			c.Assert(label.AssetsCount, Equals, 1)
			break
		}
	}
	c.Assert(found, Equals, true)

	ts.requestDelete(c, ts.token, &types.DeleteAssetItems{
		List: []types.DeleteAssetItem{{ID: "2"}},
	})
	assets = ts.listAssetsInLabel(c, l.ID, ts.token)
	c.Assert(len(assets), Equals, 0)

	// 7. list labels again, and asset count should be zero now
	labels = ts.listLabels(c)

	found = false
	for _, label := range labels {
		c.Assert(label.AssetsCount, Equals, 0)
	}
	c.Assert(found, Equals, false)
}

func (ts *mainSuite) TestLabelMetadataClassify(c *C) {
	ts.h.conf.UseMemdb = true
	c.Assert(ts.h.openMemDB(), IsNil)

	f := gpsAssets[1]
	e := strings.TrimPrefix(filepath.Ext(f.file), ".")
	u := fmt.Sprintf("/asset?token=%s&ext=%s&createtime=%s&sha1=%s", ts.token, e, f.time, f.hash)
	assetName := "1.heic"
	ts.createAsset(c, f.file, u, assetName, f.hash)

	// label ID 181 is for 'info'
	ts.testLabelMetadataClassify(c, 2, []int{181}, "web site, website, internet site, site", 0.585483, true)
	ts.testLabelMetadataClassify(c, 3, []int{181}, "website, web site, internet site, site", 0.304024, true)
	ts.testLabelMetadataClassify(c, 4, []int{181}, "web site, website, internet site, site", 0.204024, false)
	ts.testLabelMetadataClassify(c, 5, []int{181}, "hair slide", 0.24534284, false)
}

func (ts *mainSuite) testLabelMetadataClassify(c *C, expectAssetID int, expectLabelIDs []int, name string, value float32,
	highProbability bool) {
	f := gpsAssets[0]
	e := strings.TrimPrefix(filepath.Ext(f.file), ".")
	u := fmt.Sprintf("/asset?token=%s&ext=%s&createtime=%s&sha1=%s", ts.token, e, f.time, f.hash)
	assetName := strconv.Itoa(expectAssetID)+"."+e
	ts.createAsset(c, f.file, u, assetName, f.hash)

	metas := []types.Metadata{
		{
			Category:     types.MetadataCategoryScene,
			SourceDevice: types.SourceDeviceIos,
			AssetID:      expectAssetID,
			Name:         types.MetadataSceneLabel,
			Value:        name,
			Model:        "tensorflow",
			Version:      1,
		},
		{
			Category:     types.MetadataCategoryScene,
			SourceDevice: types.SourceDeviceIos,
			AssetID:      expectAssetID,
			Name:         types.MetadataSceneProbability,
			Value:        fmt.Sprintf("%f", value),
			Model:        "tensorflow",
			Version:      1,
		},
	}

	ts.testInsertMetadatas(c, metas, false, true)

	for _, id := range expectLabelIDs {
		assets := ts.listAssetsInLabel(c, id, ts.token)
		if highProbability {
			c.Assert(assets, DeepEquals, []types.AssetNameConfidence{
				{Name: assetName, Hash: f.hash, Confidence: value},
			})
		} else {
			c.Assert(len(assets), Equals, 0)
		}
	}

	ts.requestDelete(c, ts.token, &types.DeleteAssetItems{
		List: []types.DeleteAssetItem{{ID: strconv.Itoa(expectAssetID)}},
	})

}
