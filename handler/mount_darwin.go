package handler

import (
	"io/ioutil"
	"path/filepath"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"github.com/sirupsen/logrus"
)

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

// extraStorageRoots is a no-op here: unlike Windows, MountDir isn't
// hard-wired to BaseDir on this platform (see initConfig) -- pointing
// --mount-dir at /Volumes already surfaces every attached disk as a
// subfolder via listMounts, with no separate mechanism needed.
func extraStorageRoots(base string) []*types.MountDir {
	return nil
}

func (h *Handler) monitorMount() {
	h.initMonitorMount()
	logrus.Info("monitor mount is not implemented in MAC OS")
}

func (h *Handler) checkUserMounts(mountDirs map[string]*types.MountDir) {
}
