package scan

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Link setup softlink topLinkDirectory and organize images by create time
func Link(topLinkDir, classifiedDir, unclassifiedDir string, rootFolder *File, filePermission os.FileMode) error {
	dup := map[string]int{}
	if classifiedDir != "" {
		for _, f := range rootFolder.Children {
			if err := linkFileClassify(topLinkDir, classifiedDir, unclassifiedDir, f, dup, filePermission); err != nil {
				return err
			}
		}
	} else {
		for _, f := range rootFolder.Children {
			if err := linkFileUnify(topLinkDir, f, dup, filePermission); err != nil {
				return err
			}
		}
	}
	return nil
}

func linkFileClassify(topLinkDir, classifiedDir, unclassifiedDir string, f *File, dup map[string]int, filePermission os.FileMode) error {
	if len(f.Children) > 0 {
		for _, f := range f.Children {
			if err := linkFileClassify(topLinkDir, classifiedDir, unclassifiedDir, f, dup, filePermission); err != nil {
				return err
			}
		}
		return nil
	} else if f.HasExifTag() {
		topLinkDir = filepath.Join(topLinkDir, classifiedDir)
		topLinkDir = filepath.Join(topLinkDir, strconv.Itoa(f.CreateTime.Year()))
		topLinkDir = filepath.Join(topLinkDir, fmt.Sprintf("%02d", f.CreateTime.Month()))
		topLinkDir = filepath.Join(topLinkDir, fmt.Sprintf("%02d", f.CreateTime.Day()))
	} else {
		topLinkDir = filepath.Join(topLinkDir, unclassifiedDir)
		topLinkDir = filepath.Join(topLinkDir, strings.TrimPrefix(filepath.Dir(f.Path()), f.RootPath()))
	}
	// check if folder is exist or not
	if _, err := os.Stat(topLinkDir); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err := os.MkdirAll(topLinkDir, filePermission); err != nil {
			return err
		}
	}

	// check if file is exist or not
	fn := filepath.Join(topLinkDir, f.Name)
	if _, err := os.Stat(fn); err == nil {
		curr, ok := dup[filepath.Base(f.Name)]
		if !ok {
			curr = 1
		}
		fn = filepath.Join(topLinkDir, filepath.Base(f.Name)+"_"+strconv.Itoa(curr)+filepath.Ext(f.Name))
		dup[filepath.Base(f.Name)] = curr + 1
	}

	return os.Symlink(f.Path(), fn)
}

func linkFileUnify(topLinkDir string, f *File, dup map[string]int, filePermission os.FileMode) error {
	if f.IsDir() {
		for _, f := range f.Children {
			if err := linkFileUnify(topLinkDir, f, dup, filePermission); err != nil {
				return err
			}
		}
		return nil
	}
	topLinkDir = filepath.Join(topLinkDir, strconv.Itoa(f.CreateTime.Year()))
	topLinkDir = filepath.Join(topLinkDir, fmt.Sprintf("%02d", f.CreateTime.Month()))
	topLinkDir = filepath.Join(topLinkDir, fmt.Sprintf("%02d", f.CreateTime.Day()))

	// check if folder is exist or not
	if _, err := os.Stat(topLinkDir); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err := os.MkdirAll(topLinkDir, filePermission); err != nil {
			return err
		}
	}

	// check if file is exist or not
	fn := filepath.Join(topLinkDir, f.Name)
	if _, err := os.Stat(fn); err == nil {
		curr, ok := dup[filepath.Base(f.Name)]
		if !ok {
			curr = 1
		}
		fn = filepath.Join(topLinkDir, filepath.Base(f.Name)+"_"+strconv.Itoa(curr)+filepath.Ext(f.Name))
		dup[filepath.Base(f.Name)] = curr + 1
	}

	return os.Symlink(f.Path(), fn)
}
