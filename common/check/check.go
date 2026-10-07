package check

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"strconv"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/exif"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/logger"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	// GeneralError means system error to access asset
	GeneralError = iota
	// LivePhotoZIPNotExist means live photo image exist, but zip does not exist
	LivePhotoZIPNotExist
	// LivePhotoImageNotExist means live photo exist, but master image does not exist
	LivePhotoImageNotExist
	// LivePhotoImageDuplicate means duplicated live photo image is found
	LivePhotoImageDuplicate
	// LivePhotoImageZeroSize means live photo image has zero size
	LivePhotoImageZeroSize
	// AssetZeroSize means asset has zero size
	AssetZeroSize
	// AssetMissInFS means asset is in db, but not in file system
	AssetMissInFS
	// AssetMissInDB means asset is in file system, but not in db
	AssetMissInDB
	// AssetDateDiff means asset in db has different date from file system
	AssetDateDiff
	// AssetExtDiff means asset in db has different extension from file system
	AssetExtDiff
	// AssetPathDiff means asset in db has different Path from file system
	AssetPathDiff
	// AssetHashDuplicate means asset HASH is duplicated in file system
	AssetHashDuplicate
	// UserMissInFS means user is in db, but not in file system
	UserMissInFS
	// UserMissInDB means user is in file system, but not in db
	UserMissInDB
	// PreviewMiss means preview is not created in advance
	PreviewMiss
	// MasterDirMiss means user's master dir is missing
	MasterDirMiss
	// PreviewDirMiss means user's preview dir is missing
	PreviewDirMiss
	// TotalCheck is number of checks
	TotalCheck
)

// AssetFileInfo prints asset file information
type AssetFileInfo struct {
	Path      string
	nameNoExt string
	extension string
	Year      int
	Month     int
	Day       int
	ID        int
	eid       int
	Hash      string
	LiphHASH  *types.LivePhotoHash
	dbMatches bool
	dbExists  bool
}

// InconsistentAsset has the information on the inconsistent asset
type InconsistentAsset struct {
	UserID      int
	Asset1Name  string
	Asset1Path  string
	Asset1Hash  string
	Asset1Error string
	Asset1Tags  *exif.Tags
	Asset2Name  string
	Asset2Path  string
	Asset2Hash  string
	Asset2Error string
}

var errMaps map[int]string

func init() {
	errMaps = map[int]string{
		GeneralError:            "GeneralError",
		LivePhotoZIPNotExist:    "LivePhotoZIPNotExist",
		LivePhotoImageNotExist:  "LivePhotoImageNotExist",
		LivePhotoImageDuplicate: "LivePhotoImageDuplicate",
		LivePhotoImageZeroSize:  "LivePhotoImageZeroSize",
		AssetZeroSize:           "AssetZeroSize",
		AssetMissInFS:           "AssetMissInFS",
		AssetMissInDB:           "AssetMissInDB",
		AssetDateDiff:           "AssetDateDiff",
		AssetExtDiff:            "AssetExtDiff",
		AssetPathDiff:           "AssetPathDiff",
		AssetHashDuplicate:      "AssetHashDuplicate",
		UserMissInFS:            "UserMissInFS",
		UserMissInDB:            "UserMissInDB",
		PreviewMiss:             "PreviewMiss",
		MasterDirMiss:           "MasterDirMiss",
		PreviewDirMiss:          "PreviewDirMiss",
	}
}

// String is to output meaning string for given check
func String(check int) string {
	switch check {
	case GeneralError:
		return "General check"
	case LivePhotoZIPNotExist:
		return "LivePhoto ZIP file check"
	case LivePhotoImageNotExist:
		return "LivePhoto Image file check"
	case LivePhotoImageDuplicate:
		return "LivePhoto Image file duplication check"
	case LivePhotoImageZeroSize:
		return "LivePhoto Image file zero size check"
	case AssetZeroSize:
		return "Asset file zero size check"
	case AssetMissInFS:
		return "Asset in file system check"
	case AssetMissInDB:
		return "Asset in database check"
	case AssetDateDiff:
		return "Asset create date in file system check"
	case AssetExtDiff:
		return "Asset extension check"
	case AssetPathDiff:
		return "Asset location check"
	case AssetHashDuplicate:
		return "Asset file hash duplication check"
	case UserMissInFS:
		return "User directory in file system check"
	case UserMissInDB:
		return "User in database check"
	case PreviewMiss:
		return "Preview files in file system check"
	case MasterDirMiss:
		return "Master directory in file system check"
	case PreviewDirMiss:
		return "Preview directory in file system check"
	}
	return "unknown check"
}

// ErrString returns the string of errors
func ErrString(k int) string {
	str, ok := errMaps[k]
	if !ok {
		return strconv.Itoa(k)
	}
	return str
}

// Runner is one consistent run
type Runner struct {
	exiftool      string
	logger        *logger.CheckLogger
	usernames     map[int]string
	Begin         time.Time
	End           time.Time
	ScanDuration  time.Duration
	CheckDuration time.Duration
	TotalScan     map[int][]int // [# of assets in FS, # of images of livephoto in FS, # of assets in FS]

	BadAssets map[int][][]InconsistentAsset // userID - inconsistent type - assets

	// Unverified holds every DB asset this run could not confirm on disk (asset ID ->
	// check type). An asset in the run's DB snapshot that is not in here was found in the
	// file system with a matching hash.
	Unverified map[int]int
}

// NewRunner creates new consistent run
func NewRunner(exiftool string, logger *logger.CheckLogger) *Runner {
	return &Runner{exiftool: exiftool, logger: logger}
}

func outputAsset(w io.Writer, name, dir string, beauty bool) error {
	tab := ""
	if beauty {
		tab = "\t"
	}
	_, err := fmt.Fprintf(w, tab+"%s\n", dir)
	if err != nil {
		return err
	}
	found := false
	if err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			if common.IsHiddenFile(p) {
				return filepath.SkipDir
			}
			return err
		}
		if !strings.HasPrefix(info.Name(), name) {
			return nil
		}
		found = true
		_, err = fmt.Fprintf(w, tab+tab+"%s %d %s\n", info.Name(), info.Size(), info.ModTime().Format(common.TimeFormatLomod))
		return err
	}); err != nil {
		return err
	}
	if !found {
		fmt.Fprintf(w, "!!! NOT FOUND\n")
	}
	return nil
}

// DebugAsset format and print asset information
func DebugAsset(w io.Writer, typ int, ia InconsistentAsset, beauty bool) error {
	d1, n1 := filepath.Split(ia.Asset1Path)
	switch typ {
	case GeneralError:
		fmt.Fprintf(w, "%s\n", ia.Asset1Error)
		//return outputAsset(w, ia.Asset1Name, ia.Asset1Path)
		return nil
	case LivePhotoZIPNotExist:
		fmt.Fprintf(w, "%s : live photo image file is exist, but zip file is not exist\n", n1)
		return outputAsset(w, ia.Asset1Name, d1, beauty)
	case LivePhotoImageNotExist:
		fmt.Fprintf(w, "%s : live photo zip file is exist, but image file is not exist\n", n1)
		return outputAsset(w, ia.Asset1Name, d1, beauty)
	case LivePhotoImageDuplicate:
		d2, n2 := filepath.Split(ia.Asset2Path)
		fmt.Fprintf(w, "%s = %s: live photo image has same hash %s\n", n1, n2, ia.Asset1Hash)
		if err := outputAsset(w, ia.Asset1Name, d1, beauty); err != nil {
			return err
		}
		return outputAsset(w, ia.Asset2Name, d2, beauty)
	case LivePhotoImageZeroSize:
		fmt.Fprintf(w, "%s : live photo image file size is 0\n", n1)
		return outputAsset(w, ia.Asset1Name, d1, beauty)
	case AssetZeroSize:
		fmt.Fprintf(w, "%s : asset file size is 0\n", n1)
		return outputAsset(w, ia.Asset1Name, d1, beauty)
	case AssetMissInFS:
		fmt.Fprintf(w, "%s : asset in DB not found in file system or wrong SHA\n", n1)
		return outputAsset(w, n1, d1, beauty)
	case AssetMissInDB:
		fmt.Fprintf(w, "%s : asset in file system not found in DB\n", n1)
		return outputAsset(w, ia.Asset1Name, d1, beauty)
	case AssetDateDiff:
		d2, n2 := filepath.Split(ia.Asset2Path)
		fmt.Fprintf(w, "%s(%s) != %s(%s): expected creation date: %s\n", n1, ia.Asset1Hash, n2, ia.Asset2Hash, ia.Asset1Error)
		// only print files in file system because DB is not exist
		return outputAsset(w, ia.Asset2Name, d2, beauty)
	case AssetExtDiff:
		d2, n2 := filepath.Split(ia.Asset2Path)
		fmt.Fprintf(w, "%s != %s: expected extension: %s\n", n1, n2, ia.Asset1Error)
		return outputAsset(w, ia.Asset2Name, d2, beauty)
	case AssetPathDiff:
		d2, n2 := filepath.Split(ia.Asset2Path)
		fmt.Fprintf(w, "%s != %s: filepath in DB is different from file system\n", n1, n2)
		if err := outputAsset(w, ia.Asset1Name, d1, beauty); err != nil {
			return err
		}
		return outputAsset(w, ia.Asset2Name, d2, beauty)
	case AssetHashDuplicate:
		d2, n2 := filepath.Split(ia.Asset2Path)
		fmt.Fprintf(w, "%s(good) == %s(wrong): hash duplicate: %s\n", n1, n2, ia.Asset1Hash)
		if err := outputAsset(w, ia.Asset1Name, d1, beauty); err != nil {
			return err
		}
		return outputAsset(w, ia.Asset2Name, d2, beauty)
	case PreviewMiss:
		fmt.Fprintf(w, "%s : miss preview file\n", n1)
		return outputAsset(w, ia.Asset1Name, ia.Asset2Path, beauty)
	}

	return nil
}

// Report write check result into given writer
func (r *Runner) Report(w io.Writer, plain bool) {
	if !plain {
		fmt.Fprintf(w, "Scan start at %s\n", r.Begin.Local())
		fmt.Fprintf(w, "Total scan time: %s\n", r.ScanDuration)
		fmt.Fprintf(w, "Total check time: %s\n", r.CheckDuration)
		for uid, total := range r.TotalScan {
			uname := r.usernames[uid]
			fmt.Fprintf(w, "%s has total %d assets (%d live photo images) in file system, %d in DB\n",
				uname, total[0], total[1], total[2])
		}
	}

	for uid, uas := range r.BadAssets {
		uname := r.usernames[uid]
		fmt.Fprintf(w, "--------------- %s ---------------\n", uname)
		for typ, as := range uas {
			if !plain {
				fmt.Fprintf(w, "----> ")
			}
			if len(as) == 0 {
				fmt.Fprintf(w, "%s is good\n", String(typ))
				continue
			}
			switch typ {
			case UserMissInFS:
				fmt.Fprintf(w, "!!! no such a user in file system\n")
			case UserMissInDB:
				fmt.Fprintf(w, "!!! no such a user in database\n")
			case MasterDirMiss:
				fmt.Fprintf(w, "!!! no master directory in file system\n")
			case PreviewDirMiss:
				fmt.Fprintf(w, "!!! no preview directory in file system\n")
			default:
				fmt.Fprintf(w, "%s has %d exception:\n", String(typ), len(as))
				for _, a := range as {
					DebugAsset(w, typ, a, !plain)
				}
			}
		}
	}
}

// Start starts one run
func (r *Runner) Start(users []user.User, assetsDB map[int]map[int][][][]types.Asset) error {
	r.TotalScan = map[int][]int{}
	r.BadAssets = map[int][][]InconsistentAsset{}
	r.Unverified = map[int]int{}
	r.usernames = map[int]string{}
	for _, user := range users {
		r.usernames[user.ID] = user.Name
		r.BadAssets[user.ID] = make([][]InconsistentAsset, TotalCheck)
		_, ok := assetsDB[user.ID]
		if !ok {
			// create one empty entry to avoid UserMissInDB
			assetsDB[user.ID] = map[int][][][]types.Asset{}
		}
	}

	r.Begin = time.Now()
	r.logger.Info("start scanning file system")
	assetsFS, badAssets, missAssets, dupAssets, err := r.scanAssetsFS(users)
	if err != nil {
		return err
	}
	defer func() {
		// set nil explicitly for GC
		assetsFS = nil
		badAssets = nil
		missAssets = nil
		dupAssets = nil
	}()

	r.logger.Info("finish scanning file system")
	checkBegin := time.Now()
	r.logger.Info("start comparing DB / file system, based on DB")
	r.compareDBWithFS(users, assetsDB, assetsFS, badAssets, missAssets)
	r.logger.Info("start comparing DB / file system, based on file system")
	r.compareFSWithDB(users, assetsDB, assetsFS, badAssets, dupAssets)

	for key, assets := range badAssets {
		for _, a := range assets {
			uas := r.BadAssets[a.UserID]
			uas[key] = append(uas[key], a)
			r.BadAssets[a.UserID] = uas
		}
	}

	r.End = time.Now()

	r.ScanDuration = checkBegin.Sub(r.Begin)
	r.CheckDuration = r.End.Sub(checkBegin)
	return nil
}

func getBadAssets(typ int, badAssets map[int][]InconsistentAsset) []InconsistentAsset {
	as, ok := badAssets[typ]
	if ok {
		return as
	}
	return []InconsistentAsset{}
}

// Inconsistent check will do below:
//   1. each file is zero size or not
//   2. each live photo raw file has corresponding live photo image
//   3. each live photo image has corresponding live photo raw file
//   4. no duplicate live photo image by their Hash
//   5. no duplicate assets their Hash or ID in file system
//   6. each asset in db has corresponding file in local file system, including same name/create time/Hash
//   7. each asset in db has at least one preview image in local file system
//   8. each asset in local file system has corresponding record in db, including same name/create time/Hash

// scanAssetsFS scans local file system, and build file tree
func (r *Runner) scanAssetsFS(users []user.User) (map[int]map[string]AssetFileInfo,
	map[int][]InconsistentAsset, map[string]struct{}, map[int]map[string]InconsistentAsset, error) {
	badAssets := map[int][]InconsistentAsset{}
	dupAssets := map[int]map[string]InconsistentAsset{}
	missAssets := map[string]struct{}{} // index: assetID.ext, which is unique
	assets := map[int]map[string]AssetFileInfo{}
	for _, u := range users {
		assetsU := map[string]AssetFileInfo{}
		dupAssetsU := map[string]InconsistentAsset{}
		livephotoImages := map[string]string{}
		livephotoImageCheck := map[string]bool{}
		livephotos := map[string]string{}
		livephotoCheck := map[string]bool{}

		r.logger.Infof("start scanning user %s", u.Name)
		master, preview := common.GetUserPhotoDir(u.HomeDir)
		r.logger.Infof("start scanning directory %s", master)

		ok, err := common.IsFileExist(master)
		if err != nil {
			as := getBadAssets(GeneralError, badAssets)
			badAssets[GeneralError] = append(as, InconsistentAsset{UserID: u.ID})
			r.TotalScan[u.ID] = []int{0, 0, 0}
			continue
		}
		if !ok {
			as := getBadAssets(MasterDirMiss, badAssets)
			badAssets[MasterDirMiss] = append(as, InconsistentAsset{UserID: u.ID})
			r.TotalScan[u.ID] = []int{0, 0, 0}
			continue
		}

		ok, err = common.IsFileExist(preview)
		if err != nil {
			as := getBadAssets(GeneralError, badAssets)
			badAssets[GeneralError] = append(as, InconsistentAsset{UserID: u.ID})
		} else if !ok {
			as := getBadAssets(PreviewDirMiss, badAssets)
			badAssets[PreviewDirMiss] = append(as, InconsistentAsset{UserID: u.ID})
		}

		total := 0
		totalLivePhotoImage := 0
		if err := filepath.Walk(master, func(p string, info os.FileInfo, err error) error {
			if info.IsDir() {
				if common.IsHiddenFile(p) {
					return filepath.SkipDir
				}
				return err
			}
			if common.IsHiddenFile(p) {
				return err
			}

			total++
			if info.Size() == 0 {
				code := AssetZeroSize
				n := ""
				if ext.IsLivePhotoImage(info.Name()) {
					totalLivePhotoImage++
					code = LivePhotoImageZeroSize
					n = ext.TrimLivePhotoImage(info.Name())
				} else {
					n = strings.TrimSuffix(info.Name(), filepath.Ext(info.Name()))
				}
				as := getBadAssets(code, badAssets)
				badAssets[code] = append(as, InconsistentAsset{UserID: u.ID, Asset1Name: n, Asset1Path: p})

				// calculate other info, and put into missing table
				meta, err := r.extractAssetMeta(p, false)
				if err != nil {
					as := getBadAssets(GeneralError, badAssets)
					badAssets[GeneralError] = append(as, InconsistentAsset{UserID: u.ID, Asset1Path: p, Asset1Error: err.Error()})
					return nil
				}
				missAssets[strconv.Itoa(meta.ID)+"."+meta.extension] = struct{}{}
				return nil
			}
			if ext.IsLivePhotoImage(info.Name()) {
				totalLivePhotoImage++
				k := ext.TrimLivePhotoImage(info.Name())
				livephotoImageCheck[k] = false
				livephotoImages[k] = p
				return nil
			}
			meta, err := r.extractAssetMeta(p, true)
			if err != nil {
				as := getBadAssets(GeneralError, badAssets)
				badAssets[GeneralError] = append(as, InconsistentAsset{UserID: u.ID, Asset1Path: p, Asset1Error: err.Error()})
				return nil
			}
			if ext.IsLivePhoto(info.Name()) {
				livephotoCheck[meta.nameNoExt] = false
				livephotos[meta.nameNoExt] = p
			}

			meta2, ok := assetsU[meta.Hash]
			if !ok {
				assetsU[meta.Hash] = meta
			} else {
				dupAssetsU[meta.Hash] = InconsistentAsset{UserID: u.ID,
					Asset1Path: p, Asset1Name: strings.TrimSuffix(info.Name(), filepath.Ext(info.Name())), Asset1Hash: meta.Hash,
					Asset2Path: meta2.Path, Asset2Name: meta2.nameNoExt, Asset2Hash: meta2.Hash,
				}
			}

			return nil
		}); err != nil {
			return nil, nil, nil, nil, err
		}

		r.logger.Info("start comparing live photos")
		r.compareLivephotos(u.ID, badAssets, missAssets, livephotos, livephotoImages, livephotoCheck, livephotoImageCheck)
		r.logger.Info("start collecting live photo SHA")
		r.compareLivephotoImageHASH(u.ID, badAssets, livephotoImages)

		assets[u.ID] = assetsU
		dupAssets[u.ID] = dupAssetsU

		r.TotalScan[u.ID] = []int{total, totalLivePhotoImage, total}
	}
	return assets, badAssets, missAssets, dupAssets, nil
}

func (r *Runner) compareLivephotos(uid int, badAssets map[int][]InconsistentAsset, missAssets map[string]struct{},
	livephotos, livephotoImages map[string]string, livephotoCheck, livephotoImageCheck map[string]bool) {
	// all live photos should have corresponding master image
	missLivePhotoImages := map[string]struct{}{}
	for k, v := range livephotos {
		_, ok := livephotoImageCheck[k]
		if ok {
			livephotoImageCheck[k] = true
			continue
		}
		r.logger.Warnf("%s[%s] no live photo image file", k, v)
		missLivePhotoImages[k] = struct{}{}
		as := getBadAssets(LivePhotoImageNotExist, badAssets)
		badAssets[LivePhotoImageNotExist] = append(as, InconsistentAsset{UserID: uid, Asset1Name: k, Asset1Path: v})
	}
	// all live photos image should have corresponding zip file
	for k, v := range livephotoImages {
		_, ok := livephotoCheck[k]
		if ok {
			livephotoCheck[k] = true
			continue
		}
		r.logger.Warnf("%s[%s] no live photo zip file", k, v)
		as := getBadAssets(LivePhotoZIPNotExist, badAssets)
		badAssets[LivePhotoZIPNotExist] = append(as, InconsistentAsset{UserID: uid, Asset1Name: k, Asset1Path: v})

		parts := strings.Split(k, "_")
		if len(parts) == 1 {
			r.logger.Warnf("invalid asset filename: %s", v)
			as := getBadAssets(GeneralError, badAssets)
			badAssets[GeneralError] = append(as, InconsistentAsset{UserID: uid, Asset1Name: k, Asset1Path: v, Asset1Error: "invalid asset filename"})
			continue
		}
		missAssets[ext.MkAssetName(parts[1], ext.ZIPString)] = struct{}{}
	}

	for k, ok := range livephotoCheck {
		if ok {
			continue
		}
		_, ok := missLivePhotoImages[k]
		if ok {
			continue
		}
		r.logger.Warnf("2nd: %s no corresponding live photo image file", k)
		as := getBadAssets(LivePhotoImageNotExist, badAssets)
		badAssets[LivePhotoImageNotExist] = append(as, InconsistentAsset{UserID: uid, Asset1Name: k, Asset1Path: livephotos[k]})
	}
	for k, ok := range livephotoImageCheck {
		if ok {
			continue
		}
		v := livephotoImages[k]
		parts := strings.Split(v, "_")
		if len(parts) == 1 {
			r.logger.Warnf("invalid asset filename: %s", v)
			as := getBadAssets(GeneralError, badAssets)
			badAssets[GeneralError] = append(as, InconsistentAsset{UserID: uid, Asset1Name: k, Asset1Path: v, Asset1Error: "invalid asset filename"})
			continue
		}
		_, ok = missAssets[ext.MkAssetName(parts[1], ext.ZIPString)]
		if ok {
			continue
		}
		r.logger.Warnf("2nd: %s no live photo zip file: %s", k, parts[1])
		as := getBadAssets(LivePhotoZIPNotExist, badAssets)
		badAssets[LivePhotoZIPNotExist] = append(as, InconsistentAsset{UserID: uid, Asset1Name: k, Asset1Path: livephotoImages[k]})
	}
}

func (r *Runner) compareLivephotoImageHASH(uid int, badAssets map[int][]InconsistentAsset, livephotoImages map[string]string) {
	hashs := map[string]string{}
	for k, v := range livephotoImages {
		Hash, err := common.GetFileSHA(v)
		if err != nil {
			as := getBadAssets(GeneralError, badAssets)
			badAssets[GeneralError] = append(as, InconsistentAsset{UserID: uid, Asset1Name: k, Asset1Path: v, Asset1Error: err.Error()})
			continue
		}
		oldHash, ok := hashs[Hash]
		if !ok {
			hashs[Hash] = v
			continue
		}
		r.logger.Warnf("collision Hash between <%s, %s>", v, oldHash)
		as := getBadAssets(LivePhotoImageDuplicate, badAssets)
		k2 := ext.TrimLivePhotoImage(filepath.Base(oldHash))
		badAssets[LivePhotoImageDuplicate] = append(as, InconsistentAsset{UserID: uid,
			Asset1Name: k, Asset1Hash: Hash, Asset1Path: v,
			Asset2Name: k2, Asset2Hash: Hash, Asset2Path: oldHash})
	}
}

func (r *Runner) extractAssetMeta(assetPath string, calcHash bool) (AssetFileInfo, error) {
	var err error
	info := AssetFileInfo{}

	dir, assetBaseName := filepath.Split(assetPath)
	dir1, Day := filepath.Split(filepath.Dir(dir))
	dir2, Month := filepath.Split(filepath.Dir(dir1))
	_, Year := filepath.Split(filepath.Dir(dir2))

	parts := strings.Split(assetBaseName, ".")
	info.nameNoExt = parts[0]
	if len(parts) < 2 {
		return info, errors.Errorf("no file extension, wrong asset Path: %s", assetPath)
	}
	info.extension = strings.ToLower(parts[1])

	info.Day, err = strconv.Atoi(Day)
	if err != nil {
		return info, errors.Wrapf(err, "while convert Day: %s, wrong asset Path: %s", Day, assetPath)
	}
	info.Month, err = strconv.Atoi(Month)
	if err != nil {
		return info, errors.Wrapf(err, "while convert Month: %s, wrong asset Path: %s", Month, assetPath)
	}
	info.Year, err = strconv.Atoi(Year)
	if err != nil {
		return info, errors.Wrapf(err, "while convert Year: %s, wrong asset Path: %s", Year, assetPath)
	}

	info.eid, err = ext.GetExtID(info.extension)
	if err != nil {
		return info, errors.Errorf("invalid file extension, wrong asset Path: %s", assetPath)
	}

	if ext.IsLivePhotoImage(assetBaseName) {
		fs, err := types.GetLivePhotoFileSHA(assetPath)
		if err != nil {
			return info, errors.Wrapf(err, "while get file sha for asset Path: %s", assetPath)
		}
		info.Hash = fs.ImageSHA1

		// trim _image suffix as well
		info.nameNoExt = ext.TrimLivePhotoImage(assetBaseName)
	} else if info.extension == ext.ZIPString {
		fs, err := types.GetLivePhotoFileSHA(assetPath)
		if err != nil {
			return info, errors.Wrapf(err, "while get file sha for asset Path: %s", assetPath)
		}
		info.Hash = fs.TotalSHA1
	} else if info.eid == ext.MOV && calcHash {
		// only do this for mov format for now
		// probe lomo origin SHA firstly
		tags, err := exif.NewTags(assetPath, r.exiftool, "")
		if err != nil {
			r.logger.Warnf("load QuickTime:ComLomorageOriginhash got: %v", err)
		}
		info.Hash = tags.GetLomoOriginSHA()
		if info.Hash == "" {
			// try to load SHA by file sha
			info.Hash, err = common.GetFileSHA(assetPath)
			if err != nil {
				return info, errors.Wrapf(err, "while get file sha for asset Path: %s", assetPath)
			}
		}
	} else if calcHash {
		info.Hash, err = common.GetFileSHA(assetPath)
		if err != nil {
			return info, errors.Wrapf(err, "while get file sha for asset Path: %s", assetPath)
		}
	}

	info.Path = assetPath
	info.dbMatches = false // check later
	info.dbExists = true   // check later

	// check filename with Day, Month, Year, and also extract ID
	parts = strings.Split(info.nameNoExt, "_")
	if len(parts) < 2 {
		return info, errors.Errorf("invalid filename %s", info.nameNoExt)
	}
	if len(parts[0]) != 8 {
		return info, errors.Errorf("invalid timestamp at filename %s", info.nameNoExt)
	}
	d, err := strconv.Atoi(parts[0][6:8])
	if err != nil {
		return info, errors.Wrapf(err, "while convert Day from filename %s", assetPath)
	}
	m, err := strconv.Atoi(parts[0][4:6])
	if err != nil {
		return info, errors.Wrapf(err, "while convert Month from filename %s", assetPath)
	}
	y, err := strconv.Atoi(parts[0][:4])
	if err != nil {
		return info, errors.Wrapf(err, "while convert Year from filename %s", assetPath)
	}

	if y != info.Year || m != info.Month || d != info.Day {
		return info, errors.Wrapf(err, "inconsistent timestamp in filename %s", assetPath)
	}
	info.ID, err = strconv.Atoi(parts[1])
	return info, err
}

// compareDBWithFS compares assets db with file system. DB is the base to compare
func (r *Runner) compareDBWithFS(users []user.User, assetsDB map[int]map[int][][][]types.Asset, assetsFS map[int]map[string]AssetFileInfo,
	badAssets map[int][]InconsistentAsset, missAssets map[string]struct{}) {
	for uid, userAssetsDB := range assetsDB {
		userAssetsFS, ok := assetsFS[uid]
		if !ok {
			as := getBadAssets(UserMissInFS, badAssets)
			badAssets[UserMissInFS] = append(as, InconsistentAsset{UserID: uid})
			r.markAllUnverified(userAssetsDB, UserMissInFS)
			continue
		}
		var user user.User
		found := false
		for _, user = range users {
			if user.ID == uid {
				found = true
				break
			}
		}
		if !found {
			as := getBadAssets(UserMissInDB, badAssets)
			badAssets[UserMissInDB] = append(as, InconsistentAsset{UserID: uid})
			r.markAllUnverified(userAssetsDB, UserMissInDB)
			continue
		}

		total := 0
		for y, assetsY := range userAssetsDB {
			for m, assetsM := range assetsY {
				for d, assetsD := range assetsM {
					for _, asset := range assetsD {
						total++
						r.compareAssetDBWithFS(user, asset, userAssetsFS, badAssets, missAssets, y, m+1, d+1)
					}
				}
			}
		}
		totalScan, ok := r.TotalScan[user.ID]
		if !ok {
			r.TotalScan[user.ID] = []int{0, 0, total}
		} else {
			r.TotalScan[user.ID] = append(totalScan, total)
		}
	}
}

func (r *Runner) compareAssetDBWithFS(user user.User, asset types.Asset, assetsFS map[string]AssetFileInfo,
	badAssets map[int][]InconsistentAsset, missAssets map[string]struct{}, y, m, d int) {
	master, preview, _ := common.GetUserPhotoMasterPreviewDirCreate(user.HomeDir, y, m, d, 0)
	ok, err := common.IsFileExist(master)
	if err != nil {
		as := getBadAssets(GeneralError, badAssets)
		badAssets[GeneralError] = append(as, InconsistentAsset{UserID: user.ID})
	}
	if !ok {
		as := getBadAssets(MasterDirMiss, badAssets)
		badAssets[MasterDirMiss] = append(as, InconsistentAsset{UserID: user.ID})
		r.markUnverified(asset, MasterDirMiss)
		return
	}

	// note that is possible to have duplicate hash, so not use the result here
	_, ok = assetsFS[asset.Hash]
	if !ok {
		_, ok := missAssets[asset.Name]
		if ok {
			// already reported from the file system side (zero size, live photo without zip)
			r.markUnverified(asset, AssetZeroSize)
			return
		}
		r.markUnverified(asset, AssetMissInFS)
		as := getBadAssets(AssetMissInFS, badAssets)
		nn := ext.NormalizeAssetNameString(y, m, d, asset.Name)
		badAssets[AssetMissInFS] = append(as,
			InconsistentAsset{
				UserID:     user.ID,
				Asset1Name: nn,
				Asset1Hash: asset.Hash,
				Asset1Path: filepath.Join(master, nn),
			})
		return
	}

	ok, err = common.IsFileExist(preview)
	if err != nil {
		as := getBadAssets(GeneralError, badAssets)
		badAssets[GeneralError] = append(as, InconsistentAsset{UserID: user.ID})
		return
	}
	if !ok {
		as := getBadAssets(PreviewDirMiss, badAssets)
		badAssets[PreviewDirMiss] = append(as, InconsistentAsset{UserID: user.ID, Asset1Path: master, Asset2Path: preview})
		return
	}

	exist := false
	hasZeroFileSize := false
	masterFilename := ext.NormalizeAssetNameString(y, m, d, asset.Name)
	previewPrefix := strings.TrimSuffix(masterFilename, filepath.Ext(masterFilename))
	masterPath := filepath.Join(master, masterFilename)
	if err := filepath.Walk(preview, func(p string, info os.FileInfo, err error) error {
		if info.IsDir() {
			if common.IsHiddenFile(p) {
				return filepath.SkipDir
			}
			return err
		}
		if !strings.HasPrefix(info.Name(), previewPrefix) {
			return nil
		}
		if info.Size() == 0 {
			err = os.Remove(p)
			if err != nil {
				logrus.Warnf("remove preview file %s: %s", p, err)
			}
			hasZeroFileSize = true
			return nil
		}
		exist = true
		return nil
	}); err != nil {
		as := getBadAssets(GeneralError, badAssets)
		badAssets[GeneralError] = append(as, InconsistentAsset{UserID: user.ID, Asset1Path: masterPath, Asset1Error: err.Error()})
	}
	if !hasZeroFileSize && exist {
		return
	}
	as := getBadAssets(PreviewMiss, badAssets)
	if ext.IsLivePhoto(masterFilename) {
		prefix := strings.TrimSuffix(masterPath, ".zip")
		// try jpg and then heic
		_, err := os.Stat(ext.MkLivePhotoImageNameJPG(prefix))
		if err == nil {
			masterPath = ext.MkLivePhotoImageNameJPG(prefix)
			masterFilename = ext.MkLivePhotoImageNameJPG(strings.TrimSuffix(masterFilename, ".zip"))
		} else {
			_, err := os.Stat(ext.MkLivePhotoImageNameHEIC(prefix))
			if err == nil {
				masterPath = ext.MkLivePhotoImageNameHEIC(prefix)
				masterFilename = ext.MkLivePhotoImageNameHEIC(strings.TrimSuffix(masterFilename, ".zip"))
			}
		}
	}
	badAssets[PreviewMiss] = append(as, InconsistentAsset{UserID: user.ID, Asset1Name: masterFilename, Asset1Path: masterPath,
		Asset2Path: preview})
}

func (r *Runner) markUnverified(asset types.Asset, typ int) {
	id, err := ext.GetAssetIDByName(asset.Name)
	if err != nil {
		r.logger.Warnf("unverified asset with invalid name %q: %v", asset.Name, err)
		return
	}
	r.Unverified[id] = typ
}

func (r *Runner) markAllUnverified(userAssetsDB map[int][][][]types.Asset, typ int) {
	for _, assetsY := range userAssetsDB {
		for _, assetsM := range assetsY {
			for _, assetsD := range assetsM {
				for _, asset := range assetsD {
					r.markUnverified(asset, typ)
				}
			}
		}
	}
}

// compareFSWithDB compares assets in file system with DB. File system is the base to compare
func (r *Runner) compareFSWithDB(users []user.User, assetsDB map[int]map[int][][][]types.Asset, assetsFS map[int]map[string]AssetFileInfo,
	badAssets map[int][]InconsistentAsset, dupAssets map[int]map[string]InconsistentAsset) {
	assetsDBHashMap := memdbToHashmap(users, assetsDB, badAssets)
	for uid, userAssetsFS := range assetsFS {
		userAssetsDB, ok := assetsDBHashMap[uid]
		if !ok {
			as := getBadAssets(UserMissInDB, badAssets)
			badAssets[UserMissInDB] = append(as, InconsistentAsset{UserID: uid})
			continue
		}
		userDupAssets, ok := dupAssets[uid]
		if !ok {
			for _, u := range users {
				if u.ID == uid {
					r.logger.Infof("%d has no duplicate assets", uid)
					break
				}
			}
		} else {
			r.compareDupAssetDBWithFS(userAssetsDB, badAssets, userDupAssets)
		}

		for Hash, fi := range userAssetsFS {
			fiDB, ok := userAssetsDB[Hash]
			if !ok {
				as := getBadAssets(AssetMissInDB, badAssets)
				badAssets[AssetMissInDB] = append(as, InconsistentAsset{UserID: uid, Asset1Name: fi.nameNoExt, Asset1Hash: fi.Hash, Asset1Path: fi.Path})
				continue
			}
			// skip same hash ones because it is checked in compare DB with FS
			if fiDB.Hash == fi.Hash {
				parts1 := strings.Split(fiDB.nameNoExt, "_")
				parts2 := strings.Split(fi.nameNoExt, "_")

				if len(parts1) == 1 || len(parts2) == 1 {
					as := getBadAssets(GeneralError, badAssets)
					badAssets[GeneralError] = append(as, InconsistentAsset{UserID: uid,
						Asset1Name: fiDB.nameNoExt, Asset1Hash: fiDB.Hash, Asset1Path: fiDB.Path,
						Asset2Name: fi.nameNoExt, Asset2Hash: fi.Hash, Asset2Path: fi.Path,
					})
					continue
				}
				if parts1[1] != parts2[1] {
					continue
				}
			}
			if fi.Year != fiDB.Year || fi.Month != fiDB.Month || fi.Day != fiDB.Day {
				as := getBadAssets(AssetDateDiff, badAssets)
				badAssets[AssetDateDiff] = append(as,
					InconsistentAsset{
						UserID:      uid,
						Asset1Name:  fiDB.nameNoExt,
						Asset1Hash:  fiDB.Hash,
						Asset1Path:  fiDB.Path,
						Asset1Error: fmt.Sprintf("%d%02d%02d", fiDB.Year, fiDB.Month, fiDB.Day),
						Asset2Name:  fi.nameNoExt,
						Asset2Hash:  fi.Hash,
						Asset2Path:  fi.Path,
					})
				continue
			}
			if fi.extension != fiDB.extension || fi.eid != fiDB.eid {
				as := getBadAssets(AssetExtDiff, badAssets)
				_, n1 := filepath.Split(fiDB.Path)
				_, n2 := filepath.Split(fi.Path)
				badAssets[AssetExtDiff] = append(as,
					InconsistentAsset{
						UserID:      uid,
						Asset1Name:  n1,
						Asset1Path:  fiDB.Path,
						Asset1Error: fiDB.extension,
						Asset2Name:  n2,
						Asset2Path:  fi.Path,
					})
				continue
			}
			if fi.nameNoExt != fiDB.nameNoExt || fi.Path != fiDB.Path {
				as := getBadAssets(AssetPathDiff, badAssets)
				badAssets[AssetPathDiff] = append(as,
					InconsistentAsset{
						UserID:     uid,
						Asset1Name: fiDB.nameNoExt,
						Asset1Path: fiDB.Path,
						Asset2Name: fi.nameNoExt,
						Asset2Path: fi.Path,
					})
				continue
			}
		}
	}
}

func (r *Runner) compareDupAssetDBWithFS(userAssetsDB map[string]AssetFileInfo, badAssets map[int][]InconsistentAsset, dupAssets map[string]InconsistentAsset) {
	for hash, a := range dupAssets {
		assetInDB, ok := userAssetsDB[hash]
		if !ok {
			as := getBadAssets(GeneralError, badAssets)
			a.Asset1Error = fmt.Sprintf("The duplicate asset is not in DB: %v", a)
			badAssets[GeneralError] = append(as, a)
			continue
		}
		// always make DB as no.1
		if assetInDB.Path == a.Asset2Path {
			tmp := a.Asset1Name
			a.Asset1Name = a.Asset2Name
			a.Asset2Name = tmp

			tmp = a.Asset1Path
			a.Asset1Path = a.Asset2Path
			a.Asset2Path = tmp
		}
		as := getBadAssets(AssetHashDuplicate, badAssets)
		badAssets[AssetHashDuplicate] = append(as, a)
	}
}

func memdbToHashmap(users []user.User, assetsDB map[int]map[int][][][]types.Asset, badAssets map[int][]InconsistentAsset) map[int]map[string]AssetFileInfo {
	assetsMap := make(map[int]map[string]AssetFileInfo)
	for uid, userAssetsDB := range assetsDB {
		var user user.User
		found := false
		for _, user = range users {
			if user.ID == uid {
				found = true
				break
			}
		}
		if !found {
			as := getBadAssets(UserMissInDB, badAssets)
			badAssets[UserMissInDB] = append(as, InconsistentAsset{UserID: uid})
			continue
		}
		assetsU := make(map[string]AssetFileInfo)
		for y, assetsY := range userAssetsDB {
			for m, assetsM := range assetsY {
				for d, assetsD := range assetsM {
					for _, asset := range assetsD {
						fi, err := mkAssetFileInfo(asset, y, m+1, d+1, user.HomeDir)
						if err != nil {
							as := getBadAssets(GeneralError, badAssets)
							badAssets[GeneralError] = append(as, InconsistentAsset{UserID: user.ID, Asset1Name: asset.Name, Asset1Hash: asset.Hash, Asset1Error: err.Error()})
							continue
						}
						assetsU[asset.Hash] = fi
					}
				}
			}
		}
		assetsMap[uid] = assetsU
	}
	return assetsMap
}

func mkAssetFileInfo(a types.Asset, y, m, d int, homedir string) (AssetFileInfo, error) {
	info := AssetFileInfo{Year: y, Month: m, Day: d, Hash: a.Hash}

	parts := strings.Split(a.Name, ".")
	if len(parts) < 2 {
		return info, errors.Errorf("no file extension, wrong asset: %s", a.Name)
	}
	info.nameNoExt = ext.NormalizeAssetNameString(y, m, d, parts[0])
	info.extension = parts[1]

	var err error
	info.eid, err = ext.GetExtID(info.extension)
	if err != nil {
		return info, errors.Errorf("invalid file extension, wrong asset: %s", a.Name)
	}

	master, _, err := common.GetUserPhotoMasterPreviewDirCreate(homedir, y, m, d, 0)
	info.Path = filepath.Join(master, ext.NormalizeAssetNameString(y, m, d, a.Name))
	return info, err
}

/*
// ScanAssetsDB scans db asset table, and compare with local file system
func ScanAssetsDB(ctx context.Context, tx *sql.Tx, uid int, memdb *asset.Memdb, verbose bool) ([]string, map[string][]string, error) {
	homedir, err := user.GetHomedir(ctx, tx, uid)
	if err != nil {
		return nil, nil, err
	}
	assets, err := memdb.GetUserAssets(uid)
	if err != nil {
		return nil, nil, err
	}
	badAssets := []string{}
	badHashAssets := map[string][]string{}
	count := 0
	for y, assetsY := range assets {
		for m, assetsM := range assetsY {
			for d, assetsD := range assetsM {
				for _, asset := range assetsD {
					parts := strings.Split(asset.Name, ".")
					ID, err := strconv.Atoi(parts[0])
					if err != nil {
						return nil, nil, err
					}
					ok, name, hashes, err := checkAsset(ID, y, m+1, d+1, homedir, parts[1], asset.Hash)
					if err != nil {
						return nil, nil, err
					}
					if !ok {
						if hashes == nil {
							badAssets = append(badAssets, name)
						} else {
							badHashAssets[name] = hashes
						}
					}

					count++
				}
			}
		}
	}
	logrus.Infof("Total scanned assets: %d", count)

	return badAssets, badHashAssets, nil
}

func checkAsset(ID, y, m, d int, homedir, extension, Hash string) (bool, string, []string, error) {
	masterBaseDir, previewBaseDir, err := common.GetUserPhotoMasterPreviewDir(homedir, y, m, d)
	if err != nil {
		return false, "", nil, err
	}
	name := ext.NormalizeAssetName(y, m, d, ID)

	ok, filename, hashs, err := checkAssetMaster(masterBaseDir, name, extension, Hash)
	if err != nil {
		return ok, filename, hashs, err
	}
	ok, err = checkAssetPreview(previewBaseDir, name)
	if err != nil {
		return false, name, nil, err
	}
	if !ok {
		return false, name + "_preview", nil, nil
	}
	return true, name, nil, nil
}

func checkAssetMaster(masterBaseDir, assetName, extension, Hash string) (bool, string, []string, error) {
	master := ext.MkAssetName(assetName, extension)
	filename := filepath.Join(masterBaseDir, master)
	exist, err := common.IsFileExist(filename)
	if err != nil {
		return false, "", nil, err
	}
	if !exist {
		return false, filename, nil, nil
	}
	if extension != ext.ZIPString {
		// try _image.jpg firstly
		exist, err := common.IsFileExist(filepath.Join(masterBaseDir, ext.MkLivePhotoImageNameJPG(master)))
		if err != nil {
			return false, "", nil, err
		}
		if !exist {
			// try _image.heic
			exist, err = common.IsFileExist(filepath.Join(masterBaseDir, ext.MkLivePhotoImageNameHEIC(master)))
			if err != nil {
				return false, "", nil, err
			}
			if !exist {
				return false, filename + "_image", nil, nil
			}
		}
	}

	fileHash, err := common.GetFileSHA(filename)
	if err != nil {
		return false, "", nil, err
	}
	//logrus.Infof("[%4d-%02d-%02d] Hash: %s, %s", y, m, d, Hash, assetPath)
	if fileHash != Hash {
		return false, filename, []string{Hash, fileHash}, nil
	}

	return true, "", nil, nil
}

func checkAssetPreview(previewBaseDir, assetName string) (bool, error) {
	stop := fmt.Errorf("stop")
	err := filepath.Walk(previewBaseDir, func(Path string, info os.FileInfo, err error) error {
		if !info.IsDir() && strings.HasPrefix(info.Name(), assetName) {
			return stop
		}
		return err
	})

	if err.Error() == stop.Error() {
		return true, nil
	}
	return false, err
}
*/
