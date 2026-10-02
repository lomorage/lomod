package check

import (
	"path/filepath"

	"os"
	"strings"

	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"github.com/sirupsen/logrus"
)

// RecoverPreviewMiss recover missed preview file
func RecoverPreviewMiss(runner types.PreviewRunner, badAssets []InconsistentAsset,
	previewDims []types.Dimension, folderPerm os.FileMode) (bool, []InconsistentAsset) {
	errAsstes := []InconsistentAsset{}
	for _, a := range badAssets {
		extension := strings.TrimPrefix(filepath.Ext(a.Asset1Path), ".")
		if extension == ext.ZIPString {
			logrus.Infof("zip preview recover is done through image file: %v", a)
			continue
		}
		runner.Generate(a.Asset1Path, a.Asset2Path, "", extension, false, false)
		runner.Generate(a.Asset1Path, a.Asset2Path, "", extension, false, true)
	}

	return true, errAsstes
}

// RecoverPreviewDirMiss recover missed preview files under one director
func RecoverPreviewDirMiss(runner types.PreviewRunner, badAssets []InconsistentAsset,
	previewDims []types.Dimension, folderPerm os.FileMode) (bool, []InconsistentAsset) {
	errAsstes := []InconsistentAsset{}
	for _, a := range badAssets {
		if err := filepath.Walk(a.Asset1Path, func(p string, info os.FileInfo, err error) error {
			if info.IsDir() {
				return err
			}
			extension := strings.TrimPrefix(filepath.Ext(info.Name()), ".")
			if extension == ext.ZIPString {
				return nil
			}
			runner.Generate(p, a.Asset2Path, "", extension, false, false)
			runner.Generate(p, a.Asset2Path, "", extension, false, true)
			return nil
		}); err != nil {
			errAsstes = append(errAsstes, a)
		}
	}
	return true, errAsstes
}
