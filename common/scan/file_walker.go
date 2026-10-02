package scan

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/exif"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"github.com/leslie-wang/times"
	"github.com/pkg/errors"
)

const (
	dirFlag       = 1 << 0
	mediaFlag     = 1 << 1
	exifFlag      = 1 << 2
	fakeFlag      = 1 << 3
	zeroSizeFlag  = 1 << 4
	truncatedFlag = 1 << 5
	imported      = 1 << 6
	importedLink  = 1 << 7
	importedMove  = 1 << 8

	paddingDefault = "   "
	startDefault   = "|"
	startEnd       = "`"
	prefix         = "--"
)

// File structure representing files and folders with their accumulated sizes
type File struct {
	Name         string
	Parent       *File `json:"-"`
	Flag         int
	SHA1         *string `json:",omitempty"`
	Children     []*File `json:",omitempty"`
	Err          *error  `json:",omitempty"`
	CreateTime   time.Time
	EarliestTime *time.Time `json:",omitempty"`
	LatestTime   *time.Time `json:",omitempty"`
}

// HasExifTag returns flag about if the file has exif tag or not
func (f *File) HasExifTag() bool {
	return f.Flag&exifFlag != 0
}

// HasMediaFiles returns flag about if the directory has media or not
func (f *File) HasMediaFiles() bool {
	return f.Flag&mediaFlag != 0
}

// IsDir returns flag about if the file is directory or not. It uses file type to determine
func (f *File) IsDir() bool {
	return f.Flag&dirFlag != 0
}

// IsFakeFile returns flag about if the file is faked or not, mainly used for UI
func (f *File) IsFakeFile() bool {
	return f.Flag&fakeFlag != 0
}

// IsImported returns flag about if the file was imported or not
func (f *File) IsImported() *bool {
	move := true
	if f.Flag&importedMove != 0 {
		return &move
	} else if f.Flag&importedLink != 0 {
		move = false
		return &move
	}
	return nil
}

// SetImported sets import flag
func (f *File) SetImported(move bool) {
	if move {
		f.Flag &= ^(importedLink)
		f.Flag |= importedMove
	} else {
		f.Flag &= ^(importedMove)
		f.Flag |= importedLink
	}
}

// Path builds a file system location for given file
func (f *File) Path() string {
	if f.Parent == nil {
		return f.Name
	}
	return filepath.Join(f.Parent.Path(), f.Name)
}

// RootPath returns the root path of the file
func (f *File) RootPath() string {
	if f.Parent == nil {
		return f.Name
	}
	return f.Parent.RootPath()
}

// ChildrenName returns all children name
func (f *File) ChildrenName() []string {
	names := []string{}
	for _, c := range f.Children {
		names = append(names, c.Name)
	}
	return names
}

// GetChildren returns children for one given path
func (f *File) GetChildren(path string, allChildren bool) *File {
	path = filepath.Clean(path)
	if path == "" {
		return nil
	}
	return f.findChildren(path, allChildren)
}

func (f *File) findChildren(path string, allChildren bool) *File {
	fPath := f.Path()
	if path != fPath {
		if !f.IsDir() {
			return nil
		}
		for _, c := range f.Children {
			child := c.findChildren(path, allChildren)
			if child != nil {
				return child
			}
		}
		return nil
	} else if allChildren {
		return f
	}
	ret := &File{
		Name:         f.Name,
		Flag:         f.Flag,
		CreateTime:   f.CreateTime,
		EarliestTime: f.EarliestTime,
		LatestTime:   f.LatestTime,
	}
	for _, c := range f.Children {
		ret.Children = append(ret.Children, &File{
			Name:         c.Name,
			Flag:         c.Flag,
			CreateTime:   c.CreateTime,
			EarliestTime: c.EarliestTime,
			LatestTime:   c.LatestTime,
		})
	}
	return ret
}

// PrintTree prints all children recursively
func (f *File) PrintTree(w io.Writer) error {
	fmt.Fprintf(w, "%s\n", f.Name)
	return f.printSubTree(w, "")
}

func (f *File) printSubTree(w io.Writer, padding string) (err error) {
	for i, file := range f.Children {
		start := startDefault
		if i == len(f.Children)-1 {
			start = startEnd
		}
		if padding == "" {
			_, err = fmt.Fprintf(w, "%s%s %s\n", start, prefix, file.Name)
		} else {
			_, err = fmt.Fprintf(w, "%s%s%s%s %s\n", startDefault, padding, start, prefix, file.Name)
		}
		if err != nil {
			return
		}
		if !file.IsDir() {
			continue
		}
		err = file.printSubTree(w, paddingDefault+padding)
		if err != nil {
			return
		}
	}
	return nil
}

// Update goes through subfiles and subfolders and detect if it has unknown time media or not
func (f *File) Update() (bool, *time.Time, *time.Time) {
	if len(f.Children) == 0 {
		return ext.IsMediaFile(f.Name), &f.CreateTime, &f.CreateTime
	}

	has := false
	subFolders := []*File{}
	for _, child := range f.Children {
		h, etime, ltime := child.Update()
		if !h {
			continue
		}
		has = true
		subFolders = append(subFolders, child)
		if f.EarliestTime != nil && f.EarliestTime.After(*etime) {
			f.EarliestTime = etime
		}
		if f.LatestTime.Before(*ltime) {
			f.LatestTime = ltime
		}
	}
	f.Children = subFolders
	return has, f.EarliestTime, f.LatestTime
}

// ReadDir function can return list of files for given folder path
type ReadDir func(dirname string) ([]os.FileInfo, error)

func ignoringReadDir(ignoredFolders map[string]struct{}, originalReadDir ReadDir) ReadDir {
	return func(path string) ([]os.FileInfo, error) {
		if ignoredFolders != nil {
			if _, ignored := ignoredFolders[path]; ignored {
				return []os.FileInfo{}, nil
			}
		}
		return originalReadDir(path)
	}
}

// WalkFolder will go through a given folder and subfolders and produces file structure
// with aggregated file sizes
func (r *Runner) WalkFolder(
	conf Config,
	readDir ReadDir,
	progress chan<- notifyStat,
) *File {
	var wg sync.WaitGroup

	// full speed scan for the 1st pass
	count := 2 * runtime.NumCPU()
	if !r.Stats.Is1stPass && r.ParallelCount != 0 {
		count = r.ParallelCount
	}
	c := make(chan bool, count)
	root := r.walkSubFolderConcurrently(conf.RootFolderName, nil,
		ignoringReadDir(conf.IgnoreFolders, readDir),
		c, &wg, progress, conf)
	wg.Wait()
	close(progress)

	// no need return root if it is first pass
	if r.Stats.Is1stPass {
		return nil
	}

	if root == nil {
		return &File{Children: []*File{}, Name: conf.RootFolderName}
	}

	root.Update()
	return root
}

// same as photoprims implementation: use modtime instead of change time as create time
func (r *Runner) getCreateTime(filename string) (t time.Time, err error) {
	ts, err := times.Stat(filename)
	if err != nil {
		r.logger.Warn(err)
		t = time.Now()
	} else if ts.HasBirthTime() {
		t = ts.BirthTime()
	} else {
		t = ts.ModTime()
	}
	return t.UTC().Truncate(r.CreateTimeUnit), err
}

func (r *Runner) walkSubFolderConcurrently(
	path string,
	parent *File,
	readDir ReadDir,
	c chan bool,
	wg *sync.WaitGroup,
	progress chan<- notifyStat,
	conf Config,
) *File {
	entries, err := readDir(path)
	if err != nil {
		r.logger.Warn(err)
		return nil
	}

	if len(entries) == 0 {
		return nil
	}

	result := &File{Children: []*File{}, Flag: dirFlag, Parent: parent}

	if !r.Stats.Is1stPass {
		_, name := filepath.Split(path)
		if parent != nil {
			result.Name = name
		} else {
			// Root dir
			// TODO unit test this Join
			result.Name = path
		}
		result.CreateTime, err = r.getCreateTime(path)
		if err != nil {
			r.logger.Warnf("Error get create time %s: %v", path, err)
		} else {
			result.EarliestTime = &result.CreateTime
			result.LatestTime = &result.CreateTime
		}
	}

	for _, entry := range entries {
		if entry.IsDir() {
			if common.IsHiddenFile(filepath.Join(path, entry.Name())) {
				progress <- notifyStat{typ: notifyScanHiddenDir}
				continue
			}
			if entry.Mode()&os.ModeSymlink != 0 {
				progress <- notifyStat{typ: notifyScanSymlinkDir}
				continue
			}
			subFolderPath := filepath.Join(path, entry.Name())

			if r.ParallelCount == 1 {
				subFolder := r.walkSubFolderConcurrently(subFolderPath, result, readDir,
					c, wg, progress, conf)
				if subFolder != nil && !r.Stats.Is1stPass {
					r.mutex.Lock()
					result.Children = append(result.Children, subFolder)
					r.mutex.Unlock()
				}
			} else {
				wg.Add(1)
				go func(subFolderPath string, result *File) {
					c <- true
					subFolder := r.walkSubFolderConcurrently(subFolderPath, result, readDir,
						c, wg, progress, conf)
					if subFolder != nil && !r.Stats.Is1stPass {
						r.mutex.Lock()
						result.Children = append(result.Children, subFolder)
						r.mutex.Unlock()
					}
					<-c
					wg.Done()
				}(subFolderPath, result)
			}
			continue
		}
		filename := filepath.Join(path, entry.Name())
		isMediaFile, err := r.walkFile(result, filename, entry, conf, progress)
		if err != nil {
			r.logger.Warnf("Error get walk %s: %v", filename, err)
			continue
		}
		if isMediaFile {
			result.Flag |= mediaFlag
		}
	}

	// process media directory
	if !result.HasMediaFiles() {
		progress <- notifyStat{typ: notifyScanOtherDir}
	} else {
		progress <- notifyStat{typ: notifyScanMediaDir}
	}

	return result
}

func (r *Runner) walkFile(parent *File, filename string, entry os.FileInfo,
	conf Config, progress chan<- notifyStat) (bool, error) {
	if common.IsHiddenFile(filename) {
		progress <- notifyStat{typ: notifyScanHiddenFile}
		return false, nil
	}
	if entry.Mode()&os.ModeSymlink != 0 {
		progress <- notifyStat{typ: notifyScanSymlinkFile}
		return false, nil
	}
	if !ext.IsMediaFile(entry.Name()) {
		progress <- notifyStat{typ: notifyScanOtherFile}
		return false, nil
	}
	if entry.Size() == 0 {
		r.logger.Warnf("%s has zero size", filename)
		progress <- notifyStat{typ: notifyScanZeroSizeFile, path: filename}
		return false, nil
	}
	if err := ext.IsMediaFileMatch(filename); err != nil {
		r.logger.Warnf("%s is probably malformed, type match got: %s", filename, err)
		if r.Stats.Is1stPass {
			progress <- notifyStat{typ: notifyScanMalformFile, path: filename}
		}
		return false, nil
	}

	// skip if file is livephone image file which has suffix _image
	if ext.IsLivePhotoImage(filename) {
		r.logger.Infof("Skip potential live photo image file %s", filename)
		progress <- notifyStat{typ: notifyScanOtherFile}
		return false, nil
	}

	file := &File{
		Name:     entry.Name(),
		Flag:     mediaFlag,
		Parent:   parent,
		Children: []*File{},
	}

	if r.Stats.Is1stPass {
		r.notifyScanMediaFile(file, progress)
		return true, nil
	}

	if ext.IsVideoFile(filepath.Ext(filename)) && conf.IgnoreVideo {
		progress <- notifyStat{typ: notifyScanIgnoreVideoFile, path: filename}
		return false, nil
	}

	// validate if SHA is exist or not
	sha1, err := common.GetFileSHA(filename)
	if err != nil {
		r.logger.Warnf("Error get SHA1 for %s: %v", filename, err)
		progress <- notifyStat{typ: notifyScanMalformFile, path: filename}
		return false, nil
	}
	file.SHA1 = &sha1

	if conf.HashExist != nil && conf.HashExist(sha1) {
		r.logger.Warnf("Skip existing %s", filename)
		progress <- notifyStat{typ: notifyScanDuplicateFile, path: filename}
		return false, nil
	}

	var t time.Time
	if !conf.NoExifAnalysis {
		exiftool := conf.Exiftool
		if exiftool == "" {
			exiftool = "exiftool"
		}
		tags, err := exif.NewTags(filename, exiftool, "")
		if err != nil {
			// skip the asset which has time
			r.logger.Warnf("Error analysis EXIF tags for %s", filename)
		} else {
			t, err = tags.GetCreateTime()
			if err != nil {
				r.logger.Warnf("Error analysis EXIF create time for %s", filename)
			} else {
				t = t.Truncate(r.CreateTimeUnit)
				file.Flag |= exifFlag
			}
		}
	}
	if !file.HasExifTag() {
		var err error
		t, err = r.getCreateTime(filename)
		if err != nil {
			progress <- notifyStat{typ: notifyScanNoCreateTimeFile, path: filename}
			return false, errors.Wrapf(err, "get create time")
		}
	}

	file.CreateTime = t
	r.mutex.Lock()
	parent.Children = append(parent.Children, file)
	r.mutex.Unlock()

	r.notifyScanMediaFile(file, progress)
	return true, nil
}

func (r *Runner) notifyScanMediaFile(file *File, progress chan<- notifyStat) {
	if ext.IsVideoFile(filepath.Ext(file.Name)) {
		progress <- notifyStat{typ: notifyScanVideoFile, file: file}
	} else {
		progress <- notifyStat{typ: notifyScanImageFile, file: file}
	}
}
