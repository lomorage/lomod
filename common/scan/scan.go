package scan

import (
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	notifyScanMediaDir = iota
	notifyScanHiddenDir
	notifyScanSymlinkDir
	notifyScanOtherDir
	notifyScanImageFile
	notifyScanVideoFile
	notifyScanHiddenFile
	notifyScanSymlinkFile
	notifyScanZeroSizeFile
	notifyScanMalformFile
	notifyScanNoCreateTimeFile
	notifyScanOtherFile
	notifyScanIgnoreVideoFile
	notifyScanDuplicateFile
)

type notifyStat struct {
	typ  int
	path string
	file *File
}

// Config is configuration for one scan
type Config struct {
	RootFolderName string
	NoExifAnalysis bool
	IgnoreVideo    bool
	IgnoreFolders  map[string]struct{}
	HashExist      func(string) bool // true means exist, false means not exist
	Logger         io.Writer
	// Exiftool is the exiftool binary to run; defaults to "exiftool" on PATH.
	// lomod passes the one it probed, which on Windows/macOS ships next to
	// lomod itself rather than on PATH.
	Exiftool string
}

// Runner is instance to do scan task
type Runner struct {
	mutex          sync.Mutex
	logger         *logrus.Logger
	ParallelCount  int
	CreateTimeUnit time.Duration
	Stats          Stats
	Begin          time.Time
	End            time.Time
	Err            error
}

// Stats is report structure for each scan
type Stats struct {
	RootDir              string
	Is1stPass            bool
	totalMediaDirs       int
	inprogressMediaDirs  int
	totalHiddenDirs      int
	totalSymlinkDirs     int
	totalOtherDirs       int
	totalImageFiles      int
	totalVideoFiles      int
	inprogressImageFiles int
	inprogressVideoFiles int
	totalHiddenFiles     int
	totalSymlinkFiles    int
	totalIgnoreVideos    int
	totalOtherFiles      int
	totalDuplicateFiles  int
	ZeroSizeFiles        []string
	MalformFiles         []string
	NoCreateTimeFiles    []string
}

// TotalMediaDirs returns total number of media directories
func (s Stats) TotalMediaDirs() int {
	return s.totalMediaDirs
}

// InProgressMediaDirs returns number of in progress scanned media directories
func (s Stats) InProgressMediaDirs() int {
	return s.inprogressMediaDirs
}

// TotalDirs returns total number of directories
func (s Stats) TotalDirs() int {
	return s.totalMediaDirs + s.totalHiddenDirs + s.totalSymlinkDirs + s.totalOtherDirs
}

// TotalMediaFiles returns total number of media files
func (s Stats) TotalMediaFiles() int {
	return s.totalVideoFiles + s.totalImageFiles
}

// TotalImageFiles returns total number of image files
func (s Stats) TotalImageFiles() int {
	return s.totalImageFiles
}

// TotalVideoFiles returns total number of video files
func (s Stats) TotalVideoFiles() int {
	return s.totalVideoFiles
}

// TotalDuplicateFiles returns total number of duplicate files
func (s Stats) TotalDuplicateFiles() int {
	return s.totalDuplicateFiles
}

// TotalIgnoreVideoFiles returns total number of ignored video files
func (s Stats) TotalIgnoreVideoFiles() int {
	return s.totalIgnoreVideos
}

// InProgressImageFiles returns number of in progress scanned image files
func (s Stats) InProgressImageFiles() int {
	return s.inprogressImageFiles
}

// InProgressVideoFiles returns number of in progress scanned video files
func (s Stats) InProgressVideoFiles() int {
	return s.inprogressVideoFiles
}

// TotalFiles returns total number of media files
func (s Stats) TotalFiles() int {
	return s.totalImageFiles + s.totalVideoFiles + s.totalHiddenFiles + s.totalSymlinkFiles + s.totalOtherFiles
}

// NewRunner starts new instance of runner
func NewRunner(logger *logrus.Logger) *Runner {
	return &Runner{
		logger:         logger,
		mutex:          sync.Mutex{},
		Stats:          Stats{},
		CreateTimeUnit: time.Second,
	}
}

// ResetStats resets runner stats
func (r *Runner) ResetStats() {
	r.End = time.Time{}
	r.Err = nil
	r.Stats.RootDir = ""
	r.Stats.Is1stPass = true
	r.Stats.totalMediaDirs = 0
	r.Stats.totalHiddenDirs = 0
	r.Stats.totalSymlinkDirs = 0
	r.Stats.totalOtherDirs = 0
	r.Stats.totalImageFiles = 0
	r.Stats.totalVideoFiles = 0
	r.Stats.totalHiddenFiles = 0
	r.Stats.totalSymlinkFiles = 0
	r.Stats.totalOtherFiles = 0
	r.Stats.totalIgnoreVideos = 0
	r.Stats.totalDuplicateFiles = 0
	r.Stats.inprogressMediaDirs = 0
	r.Stats.inprogressImageFiles = 0
	r.Stats.inprogressVideoFiles = 0
	r.Stats.ZeroSizeFiles = []string{}
	r.Stats.MalformFiles = []string{}
	r.Stats.NoCreateTimeFiles = []string{}
}

// Start given directory to find out which assets doesn't have image
func (r *Runner) Start(conf Config, reportFileChan chan<- *File) (*File, error) {
	var err error
	conf.RootFolderName, err = filepath.Abs(conf.RootFolderName)
	if err != nil {
		return nil, err
	}

	// reset progress stats
	r.ResetStats()
	r.Stats.RootDir = filepath.Clean(conf.RootFolderName) + string(filepath.Separator)
	r.Begin = time.Now()
	defer func() {
		r.End = time.Now()
	}()

	info := fmt.Sprintf("1st PASS START: root folder (%s), ignore video (%v), ignore EXIF analysis (%v)",
		conf.RootFolderName, conf.IgnoreVideo, conf.NoExifAnalysis)
	if conf.Logger != nil {
		conf.Logger.Write([]byte(info + "\n"))
	}
	logrus.Info(info)

	// the 1st pass to get total numbers
	progress := make(chan notifyStat)
	go r.reportStats(conf.Logger, reportFileChan, progress, true)
	r.WalkFolder(conf, ioutil.ReadDir, progress)

	if r.Stats.TotalDirs() == 0 && r.Stats.TotalMediaFiles() != 0 {
		// all media files are in one root directory
		r.Stats.totalMediaDirs = 1
	}

	info = fmt.Sprintf("1st PASS FINISH: scanned %d directories, found %d/%d image/video files, %d malform media files, %d other files",
		r.Stats.TotalDirs(),
		r.Stats.totalImageFiles,
		r.Stats.totalVideoFiles,
		len(r.Stats.MalformFiles),
		r.Stats.totalOtherFiles)
	if conf.Logger != nil {
		conf.Logger.Write([]byte(info + "\n"))
	}
	logrus.Info(info)

	// the 2nd pass to do real scan
	r.Stats.Is1stPass = false
	progress = make(chan notifyStat)
	go r.reportStats(conf.Logger, reportFileChan, progress, false)
	rootFolder := r.WalkFolder(conf, ioutil.ReadDir, progress)
	rootFolder.Name = conf.RootFolderName

	SortDesc(rootFolder)

	return rootFolder, nil
}

func (r *Runner) reportStats(logger io.Writer, reportFileChan chan<- *File, progress <-chan notifyStat, firstPass bool) {

	for {
		select {
		case c, ok := <-progress:
			if !ok {
				return
			}
			switch c.typ {
			case notifyScanMediaDir:
				if firstPass {
					r.Stats.totalMediaDirs++
				} else {
					r.Stats.inprogressMediaDirs++
				}
			case notifyScanHiddenDir:
				if firstPass {
					r.Stats.totalHiddenDirs++
				}
			case notifyScanSymlinkDir:
				if firstPass {
					r.Stats.totalSymlinkDirs++
				}
			case notifyScanOtherDir:
				if firstPass {
					r.Stats.totalOtherDirs++
				}
			case notifyScanImageFile:
				if firstPass {
					r.Stats.totalImageFiles++
				} else {
					r.Stats.inprogressImageFiles++
				}
			case notifyScanVideoFile:
				if firstPass {
					r.Stats.totalVideoFiles++
				} else {
					r.Stats.inprogressVideoFiles++
				}
			case notifyScanHiddenFile:
				if firstPass {
					r.Stats.totalHiddenFiles++
				}
			case notifyScanSymlinkFile:
				if firstPass {
					r.Stats.totalSymlinkFiles++
				}
			case notifyScanZeroSizeFile:
				if firstPass {
					if logger != nil {
						logger.Write([]byte("found zero size file " + c.path + "\n"))
					}

					r.Stats.ZeroSizeFiles = append(r.Stats.ZeroSizeFiles, strings.TrimPrefix(c.path, r.Stats.RootDir))
				}
			case notifyScanMalformFile:
				// both 1st and 2nd pass may have malform files
				if logger != nil {
					logger.Write([]byte("found malformed file " + c.path + "\n"))
				}

				r.Stats.MalformFiles = append(r.Stats.MalformFiles, strings.TrimPrefix(c.path, r.Stats.RootDir))
			case notifyScanNoCreateTimeFile:
				if logger != nil {
					logger.Write([]byte(c.path + " miss create time\n"))
				}

				r.Stats.NoCreateTimeFiles = append(r.Stats.NoCreateTimeFiles, strings.TrimPrefix(c.path, r.Stats.RootDir))
			case notifyScanOtherFile:
				if firstPass {
					r.Stats.totalOtherFiles++
				}
			case notifyScanIgnoreVideoFile:
				r.Stats.totalIgnoreVideos++
				if logger != nil {
					logger.Write([]byte("skip video file: " + c.path + "\n"))
				}
			case notifyScanDuplicateFile:
				r.Stats.totalDuplicateFiles++
				if logger != nil {
					logger.Write([]byte(c.path + " was imported, skip\n"))
				}
			}
			if firstPass {
				continue
			}
			if c.file != nil && reportFileChan != nil {
				reportFileChan <- c.file
			}
		}
	}
}
