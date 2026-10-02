package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"bitbucket.org/lomoware/lomo-backend/common/types"

	. "gopkg.in/check.v1"
)

func (ts *mainSuite) listAlbumScenes(c *C) libreAlbums {
	url := fmt.Sprintf("/api/albums/thing/list/?token=%s", ts.token)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	c.Assert(err, IsNil)

	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	validateStatusCode(c, url, rr, http.StatusOK)

	res := rr.Result()
	defer res.Body.Close()

	reply := libreAlbums{}
	c.Assert(json.NewDecoder(res.Body).Decode(&reply), IsNil)
	return reply
}

func (ts *mainSuite) listAlbumScene(c *C, id int) libreAlbumDetails {
	url := fmt.Sprintf("/api/albums/thing/%d/?token=%s", id, ts.token)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	c.Assert(err, IsNil)

	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	validateStatusCode(c, url, rr, http.StatusOK)

	res := rr.Result()
	defer res.Body.Close()

	reply := libreAlbumDetails{}
	c.Assert(json.NewDecoder(res.Body).Decode(&reply), IsNil)
	return reply
}

func (ts *mainSuite) TestMetadataSceneBasic(c *C) {
	ts.initFakeAssets(c)

	ts.h.conf.UseMemdb = true
	c.Assert(ts.h.openMemDB(), IsNil)

	metas := []types.Metadata{
		{
			Category:     types.MetadataCategoryScene,
			SourceDevice: types.SourceDeviceIos,
			AssetID:      1,
			Name:         "ios." + types.MetadataSceneLabel,
			Value:        "dingo, nappy, napkin",
			Model:        "tensorflow",
			Version:      1,
		},
		{
			Category:     types.MetadataCategoryScene,
			SourceDevice: types.SourceDeviceIos,
			AssetID:      1,
			Name:         "ios." + types.MetadataSceneProbability,
			Value:        "0.99",
			Model:        "tensorflow",
			Version:      1,
		},
	}
	ts.testInsertMetadatas(c, metas, true, true)

	things := ts.listAlbumScenes(c)
	c.Assert(things, DeepEquals, libreAlbums{
		Count: 1,
		Results: []libreAlbumBrief{
			{
				ID:         121,
				Title:      "dog",
				PhotoCount: 1,
				CoverPhotos: []libreAlbumCover{
					{ImageHash: "1.jpg"},
				},
			},
		},
	})

	ts.testInsertMetadatas(c, []types.Metadata{
		{
			Category:     types.MetadataCategoryScene,
			SourceDevice: types.SourceDeviceIos,
			AssetID:      5,
			Name:         "ios." + types.MetadataSceneLabel,
			Value:        "breakwater, groin, groyne, mole, bulwark, seawall, jetty",
			Model:        "tensorflow",
			Version:      1,
		},
		{
			Category:     types.MetadataCategoryScene,
			SourceDevice: types.SourceDeviceIos,
			AssetID:      5,
			Name:         "ios." + types.MetadataSceneProbability,
			Value:        "0.99",
			Model:        "tensorflow",
			Version:      1,
		},
	}, true, true)

	things = ts.listAlbumScenes(c)
	c.Assert(things, DeepEquals, libreAlbums{
		Count: 2,
		Results: []libreAlbumBrief{
			{
				ID:         121,
				Title:      "dog",
				PhotoCount: 1,
				CoverPhotos: []libreAlbumCover{
					{ImageHash: "1.jpg"},
				},
			},
			{
				ID:         384,
				Title:      "water",
				PhotoCount: 1,
				CoverPhotos: []libreAlbumCover{
					{ImageHash: "5.jpg"},
				},
			},
		},
	})

	album := ts.listAlbumScene(c, 121)
	album.Results.CreateTime = ""
	for i, photos := range album.Results.GroupedPhotos {
		album.Results.GroupedPhotos[i].CreateTime = ""
		for j := range photos.Items {
			album.Results.GroupedPhotos[i].Items[j].CreateTime = ""
			album.Results.GroupedPhotos[i].Items[j].BirthTime = ""
		}
	}
	c.Assert(album, DeepEquals, libreAlbumDetails{Results: libreAlbumDetail{
		ID:       "121",
		Title:    "dog",
		Owner:    libreAlbumOwner{ID: 1, Username: "alice"},
		SharedTo: []int{},
		GroupedPhotos: []libreAlbumPhotoGroup{
			{
				Items: []libreAlbumPhoto{
					{
						ID:       "1.jpg",
						URL:      "1.webp",
						Type:     "image",
						Owner:    libreAlbumOwner{ID: 1, Username: "alice"},
						SharedTo: []int{},
					},
				},
			},
		},
	},
	})
}

func (ts *mainSuite) TestMetadataSceneEmptyLabel(c *C) {
	// empty label should not be in any album
	ts.initFakeAssets(c)

	ts.h.conf.UseMemdb = true
	c.Assert(ts.h.openMemDB(), IsNil)

	metas := []types.Metadata{
		{
			Category:     types.MetadataCategoryScene,
			SourceDevice: types.SourceDeviceIos,
			AssetID:      1,
			Name:         "ios." + types.MetadataSceneLabel,
			Value:        "diaper",
			Model:        "tensorflow",
			Version:      1,
		},
		{
			Category:     types.MetadataCategoryScene,
			SourceDevice: types.SourceDeviceIos,
			AssetID:      1,
			Name:         "ios." + types.MetadataSceneProbability,
			Value:        "0.99",
			Model:        "tensorflow",
			Version:      1,
		},
	}
	ts.testInsertMetadatas(c, metas, true, true)

	things := ts.listAlbumScenes(c)
	c.Assert(things, DeepEquals, libreAlbums{})
}
