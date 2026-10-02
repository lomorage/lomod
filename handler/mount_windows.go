package handler

import (
	"io/ioutil"
	"path/filepath"
	"strings"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/mountinfo"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"github.com/sirupsen/logrus"
)

// lomorageDirName is the folder created on an extra local drive so an
// account can be stored there, mirroring the "Lomorage" folder name already
// used for the desktop default BaseDir.
const lomorageDirName = "Lomorage"

func umount(p string) error {
	return common.ErrNotImplementedFormat
}

func listMounts(base string) (map[string]*types.MountDir, error) {
	fis, err := ioutil.ReadDir(base)
	if err != nil {
		return nil, err
	}
	mounts := map[string]*types.MountDir{}
	for _, fi := range fis {
		if !fi.IsDir() {
			continue
		}
		dir := filepath.Join(base, fi.Name())
		mounts[dir] = &types.MountDir{Dir: dir, Type: types.MountLocal}
	}

	return mounts, nil
}

// extraStorageRoots surfaces every other local/removable drive as an
// additional storage root ("<drive>\Lomorage"), alongside the MountDir
// subfolders listMounts already returns. Windows has no /media-style mount
// pool for listMounts to walk (see initConfig), so a drive other than the
// one MountDir lives on is otherwise invisible to account creation -- e.g.
// there'd be no way to point a new account at an external hard drive.
func extraStorageRoots(base string) []*types.MountDir {
	baseVolume := filepath.VolumeName(filepath.Clean(base))
	drives, err := mountinfo.ListLocalDrives()
	if err != nil {
		logrus.Warnf("failed to list local drives: %v", err)
		return nil
	}
	var roots []*types.MountDir
	for _, d := range drives {
		if strings.EqualFold(filepath.VolumeName(d.Root), baseVolume) {
			// already covered by the MountDir subfolder listing above
			continue
		}
		roots = append(roots, &types.MountDir{
			// Type is intentionally MountLocal even for a removable drive --
			// this codebase doesn't track per-device UUIDs on Windows (see
			// migrateMntUUID), so there's no swap-detection to advertise.
			Type:      types.MountLocal,
			Dir:       filepath.Join(d.Root, lomorageDirName),
			FreeSize:  d.FreeSize,
			TotalSize: d.TotalSize,
		})
	}
	return roots
}

func (h *Handler) monitorMount() {
	h.initMonitorMount()
	logrus.Info("monitor mount is not implemented in Windows OS")
}

func (h *Handler) checkUserMounts(mountDirs map[string]*types.MountDir) {
}
