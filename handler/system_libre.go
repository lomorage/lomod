package handler

import (
	"bitbucket.org/lomoware/lomo-backend/common"
	"net/http"
)

func (h *Handler) getStats(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)
	var stats struct {
		NumPhoto int `json:"num_photos"`
		NumPeople int `json:"num_people"`
		NumFace int `json:"num_faces"`
		NumAlbumAuto int `json:"num_albumauto"`
		NumAlbumDate int `json:"num_albumdate"`
	}
	stats.NumPhoto = 1
	stats.NumPeople = 1
	stats.NumFace = 1
	stats.NumAlbumAuto = 1
	stats.NumAlbumDate = 1

	common.WriteBody(w, stats)
}

type searchExamples struct {
	Results []string `json:"results"`
}
func (h *Handler) getSearchExample(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	common.WriteBody(w, searchExamples{Results: []string{"example1", "example2"}})
}