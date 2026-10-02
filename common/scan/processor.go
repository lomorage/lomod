package scan

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"sort"
	"strings"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/types"
)

type byNameDesc []*File

func (f byNameDesc) Len() int           { return len(f) }
func (f byNameDesc) Swap(i, j int)      { f[i], f[j] = f[j], f[i] }
func (f byNameDesc) Less(i, j int) bool { return strings.Compare(f[i].Name, f[j].Name) == -1 }

// SortDesc sorts folder content by size from largest to smallest
func SortDesc(folder *File) {
	sort.Sort(byNameDesc(folder.Children))
	for _, file := range folder.Children {
		SortDesc(file)
	}
}

// ProcessFolder removes empty files and sorts folder content based on name
func ProcessFolder(folder *File) {
	files := []*File{}

	for _, f := range folder.Children {
		if len(f.Children) == 0 {
			files = append(files, f)
		}
		ProcessFolder(f)
		files = append(files, f)
	}

	folder.Children = files

	SortDesc(folder)
}

// FilterFolder removes files having EXIF tag
func FilterFolder(folder *File) {
	files := []*File{}

	for _, f := range folder.Children {
		if len(f.Children) > 0 {
			FilterFolder(f)
			files = append(files, f)
		} else if !f.HasExifTag() {
			f.Children = []*File{
				{Parent: f, Flag: fakeFlag, Name: "New Year  : "},
				{Parent: f, Flag: fakeFlag, Name: "New Month : "},
				{Parent: f, Flag: fakeFlag, Name: "New Day   : "},
				{Parent: f, Flag: fakeFlag, Name: "Apply"},
			}
			files = append(files, f)
		}
	}

	folder.Children = files
}

// ParseFile is to generate new root folder
func ParseFile(f string) (*File, error) {
	rootFolder := &File{}
	content, err := ioutil.ReadFile(f)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(content, rootFolder); err != nil {
		return nil, err
	}

	recreateParent(rootFolder, nil)
	return rootFolder, nil
}

// RecreateParent rebuilds a File tree's Parent links after a JSON round-trip.
// Parent is `json:"-"`, so any tree decoded straight from JSON -- e.g. a
// subtree a client POSTs back to import after browsing a scan result --
// has every node's Parent nil until this runs, which makes Path() silently
// return just that node's own Name instead of its real filesystem location.
func RecreateParent(root *File) {
	recreateParent(root, nil)
}

func recreateParent(folder, parent *File) {
	folder.Parent = parent
	for _, f := range folder.Children {
		if len(f.Children) == 0 {
			f.Parent = folder
			continue
		}
		recreateParent(f, folder)
	}
}

// CreatePreview creates preview files for scan file
func CreatePreview(rootFolder string, f *File, r types.PreviewRunner) {
	if !f.IsDir() {
		createPreview(rootFolder, f, r)
		return
	}
	for _, c := range f.Children {
		CreatePreview(rootFolder, c, r)
	}
}

func createPreview(rootFolder string, f *File, r types.PreviewRunner) {
	previewPath := common.NormalDatedDirName(rootFolder, f.CreateTime.Year(),
		int(f.CreateTime.Month()), f.CreateTime.Day())
	r.Generate(f.Path(), previewPath, *f.SHA1, filepath.Ext(f.Name), false, false)
	r.Generate(f.Path(), previewPath, *f.SHA1, filepath.Ext(f.Name), false, true)
}
