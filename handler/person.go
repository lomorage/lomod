package handler

import (
	"github.com/gorilla/mux"
	"net/http"

	"bitbucket.org/lomoware/lomo-backend/common"
)

type Person struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	FaceCount    int    `json:"face_count"`
	FaceURL      string `json:"face_url"`
	FacePhotoURL string `json:"face_photo_url"`
}

func (h *Handler) listPerson(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	common.WriteBody(w, []Person{
		{ID: 1, Name: "alice", FaceCount: 10, FaceURL: "/persons/face/1", FacePhotoURL: "/persons/face_photo/1"},
		{ID: 2, Name: "bob", FaceCount: 20, FaceURL: "/persons/face/2", FacePhotoURL: "/persons/face_photo/2"},
	})
}

func (h *Handler) getPersonFace(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	http.ServeFile(w, r, "test/img/1_face.jpg")
}

func (h *Handler) getPersonFacePhoto(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	http.ServeFile(w, r, "test/img/1_2003_01_17.jpg")
}

type locationLeaf struct {
	Name string `json:"name"`
}

type locationNode struct {
	Name string `json:"name"`
	Children []locationLeaf `json:"children"`
	IsExpanded bool `json:"isExpanded"`
}

type locationTree struct {
	Name string `json:"name"`
	Children []locationNode `json:"children"`
	IsExpanded bool `json:"isExpanded"`
}

func (h *Handler) locationsunburst(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	reply := locationTree{
		Name: "USA",
		IsExpanded: true,
		Children: []locationNode{
			{
			Name:       "Newyork",
			IsExpanded: true,
				Children: []locationLeaf {
				{Name: "longbeach"},
				},
		},
			{
				Name:       "Sanjose",
				IsExpanded: true,
				Children: []locationLeaf {
					{Name: "zoo"},
				},
			},
		},
	}
	common.WriteBody(w, &reply)
}

type series struct {
	Label string `json:"label"`
}

type wc struct {
	Captions []series `json:"captions"`
	Locations []series `json:"locations"`
	People []series `json:"people"`
}

func (h *Handler) wordcloud(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)
	reply := wc {
		People: []series{{Label: "alice"}, {Label: "bob"}, {Label: "tony"}},
		Locations: []series{{Label: "beijing"}, {Label: "shanghai"}},
		Captions: []series{
			{Label: "flowers"}, {Label: "beach"}, {Label: "beach"}, {Label: "beach"}, {Label: "beach"}, {Label: "beach"},
			{Label: "flowers"}, {Label: "beach"}, {Label: "beach"}, {Label: "beach"}, {Label: "beach"}, {Label: "beach"},
			{Label: "flowers"}, {Label: "beach"}, {Label: "beach"}, {Label: "beach"}, {Label: "beach"}, {Label: "beach"},
			{Label: "flowers"}, {Label: "beach"}, {Label: "beach"}, {Label: "beach"}, {Label: "beach"}, {Label: "beach"},
		},
	}
	common.WriteBody(w, &reply)

}

type pmc struct {
	Month string `json:"month"`
	Count int `json:"count"`
}

func (h *Handler) photomonthcounts(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	reply := []pmc {
		{Month: "Jan", Count: 12},
		{Month: "Feb", Count: 200},
		{Month: "Mac", Count: 30},
	}
	common.WriteBody(w, &reply)
}

type ltl struct {
	Start int `json:"start"`
	End  int `json:"end"`
	Color string `json:"color"`
	Loc  string `json:"loc"`
	Data []int `json:"data"`
}

func (h *Handler) locationtimeline(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	reply := []ltl {
		{Start: 1318781876, End: 1318881876, Color: "#cd3b54", Loc: "beijing", Data: []int{24}},
		{Start: 1318881876, End: 1318981876, Color: "#59b953", Loc: "sjc", Data: []int{120}},
		{Start: 1318981876, End: 1319081876, Color: "#ba4fb9", Loc: "SFO", Data: []int{80}},
	}
	common.WriteBody(w, &reply)
}

type leaf struct {
	ID string `json:"id"`
}

type link struct {
	Source string `json:"source"`
	Target string `json:"target"`
}
type graph struct {
	Nodes []leaf `json:"nodes"`
	Links []link `json:"links"`
}

func (h *Handler) socialgraph(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	reply := graph {
		Nodes: []leaf{
			{ID: "Node0"}, {ID: "Node1"}, {ID: "Node2"}, {ID: "Node3"},{ID: "Node4"}, {ID: "Node5"}, {ID: "Node6"},
		},
		Links: []link {
			{Source: "Node1", Target: "Node0"},
			{Source: "Node2", Target: "Node1"},
			{Source: "Node3", Target: "Node0"},
			{Source: "Node3", Target: "Node1"},
			{Source: "Node3", Target: "Node2"},
			{Source: "Node3", Target: "Node3"},
			{Source: "Node4", Target: "Node3"},
			{Source: "Node4", Target: "Node4"},
			{Source: "Node5", Target: "Node4"},
			{Source: "Node5", Target: "Node5"},
			{Source: "Node6", Target: "Node4"},
			{Source: "Node6", Target: "Node6"},
			{Source: "Node6", Target: "Node6"},
		},
	}

	common.WriteBody(w, &reply)
}

type faceSize struct {
	X int `json:"x"`
	Y int `json:"y"`
	Size int `json:"size"`
}

type face struct {
	PersonName string `json:"person_name"`
	Color string `json:"color"`
	FaceURL string `json:"face_url"`
	Value faceSize `json:"value"`
}
func (h *Handler) clusterfaces(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)
	reply := []face {
		{PersonName: "alice", Color: "#cd3b54", FaceURL: "/clusterfaces/1.jpg", Value: faceSize{X: 0, Y: 0, Size: 50}},
		{PersonName: "bob", Color: "#cd3b54", FaceURL: "/clusterfaces/3.jpg", Value: faceSize{X: 50, Y: 50, Size: 50}},
	}
	common.WriteBody(w, &reply)
}

func (h *Handler) clusterfacesFile(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	hash := mux.Vars(r)["assetID"]
	switch hash {
	case "1.jpg":
		http.ServeFile(w, r, "test/img/1_2003_01_17.jpg")
	case "2.jpg":
		http.ServeFile(w, r, "test/img/3_2003_11_01.jpg")
	case "3.jpg":
		http.ServeFile(w, r, "test/img/4_2003_11_01.jpg")
	case "4.jpg":
		http.ServeFile(w, r, "test/img/5_2003_11_23.jpg")
	}
}

type inferredFace struct {
	PersonName string `json:"person_name"`
	ImageHash string `json:"image_hash""`
	Image string `json:"image"`
	PersonLabelProbability int `json:"person_label_probability"`
}

func (h *Handler) listInferredFace(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	reply := []inferredFace{
		{PersonName: "Alice", PersonLabelProbability: 80, ImageHash: "1.jpg", Image: "1.jpg/2.jpg/3.jpg"},
		{PersonName: "Alice", PersonLabelProbability: 70, ImageHash: "2.jpg", Image: "1.jpg/2.jpg/3.jpg"},
		{PersonName: "Bob", PersonLabelProbability: 80, ImageHash: "3.jpg", Image: "1.jpg/2.jpg/3.jpg"},
		{PersonName: "Bob", PersonLabelProbability: 60, ImageHash: "4.jpg", Image: "1.jpg/2.jpg/3.jpg"},
	}
	common.WriteBody(w, &reply)

}

type labeledFace struct {
	PersonName string `json:"person_name"`
	ImageHash string `json:"image_hash""`
	Image string `json:"image"`
}

func (h *Handler) listLabeledFace(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)
	reply := []labeledFace{
		{PersonName: "Alice", ImageHash: "1.jpg", Image: "1.jpg/3.jpg"},
		{PersonName: "Alice", ImageHash: "2.jpg", Image: "1.jpg/2.jpg"},
		{PersonName: "Bob", ImageHash: "3.jpg", Image: "2.jpg/1.jpg"},
		{PersonName: "Bob", ImageHash: "4.jpg", Image: "1.jpg/2.jpg/3.jpg"},
	}
	common.WriteBody(w, &reply)
}