package handler

import (
	"path/filepath"

	"bitbucket.org/lomoware/lomo-backend/common/cmd"
)

func rsync(exeDir, src, dst string) ([]byte, error) {
	return cmd.Run(filepath.Join(exeDir, "rsync"), "-rt", "--inplace", "--exclude=.DS_Store",
		"--exclude=.AppleDouble", "--ignore-existing", src, dst)
}
