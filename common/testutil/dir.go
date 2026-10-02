package testutil

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"github.com/kr/pretty"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ValidateFilesInDir validate number of files in one directory.
func ValidateFilesInDir(p string, numFiles int, dir bool) error {
	fis, err := ioutil.ReadDir(p)
	if err != nil {
		return err
	}
	count := 0
	allFiles := []string{}
	countedFiles := []string{}
	for _, fi := range fis {
		allFiles = append(allFiles, fi.Name())
		if dir {
			if fi.IsDir() {
				count++
				countedFiles = append(countedFiles, fi.Name())
			}
		} else {
			if !fi.IsDir() {
				count++
				countedFiles = append(countedFiles, fi.Name())
			}
		}
	}
	if count != numFiles {
		return errors.Errorf("p: %s, dir mode: %v, %v(%v)", p, dir, countedFiles, allFiles)
	}
	return nil
}

// ValidateFileSize validate asset size
// size -1 means asset not exist; 0 means not check size.
func ValidateFileSize(p string, size int64) error {
	stat, err := os.Stat(p)
	if size == -1 {
		if err == nil {
			return errors.Errorf("File %s should not exist, but found in system", p)
		} else if !os.IsNotExist(err) {
			return errors.Errorf("Validate non-exist file %s got %v", p, err)
		}
		return nil
	} else if err != nil {
		return errors.Errorf("Validate %s got %v", p, err)
	}
	if stat.Size() != size {
		return errors.Errorf("File %s expect size %d, got %d", p, size, stat.Size())
	}
	return nil
}

// WaitWithFileCounts validate preview complete.
func WaitWithFileCounts(ctx context.Context, root string, totalFiles int) error {
	done := false
	for {
		after := time.After(500 * time.Millisecond)
		select {
		case <-ctx.Done():
			done = true
		case <-after:
		}
		count, err := countFiles(root, done)
		if err != nil {
			if !os.IsNotExist(err) {
				return err
			}
			logrus.Warnf("probe %s file count: %v", root, err)
			continue
		}
		if count == totalFiles {
			// sleep 1 second to ensure all preview assets are written completely
			// seems docker may have some delay to sync data
			time.Sleep(time.Second)

			return nil
		}
		if done {
			logrus.Warnf("%s: expect %d while getting %d files, reach deadlone, return", root, totalFiles, count)
			return ctx.Err()
		}
		logrus.Warnf("%s: expect %d while getting %d files, wait again", root, totalFiles, count)
	}
}

func countFiles(root string, debug bool) (int, error) {
	count := 0
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || common.IsHiddenFile(path) {
			return nil
		}
		count++
		if debug {
			logrus.Infof("walk preview %s", path)
		}
		return nil
	})
	return count, err
}

// CompareDirsByTreeCmd compare one directory structure with given files using tree command.
func CompareDirsByTreeCmd(dir, filename string) error {
	content, err := exec.Command("tree", "-Ns", dir).CombinedOutput()
	if err != nil {
		return err
	}

	// write to temp file for backup
	ioutil.WriteFile("/tmp/test.txt", content, 0755)

	expectContent, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	obtain := strings.TrimSpace(string(content))
	expect := strings.TrimSpace(string(expectContent))
	if obtain == expect {
		return nil
	}
	fmt.Printf(`
------- obtain -------
%s
------- expect -------
%s`, obtain, expect)

	return errors.New(formatUnequal(obtain, expect))
}

// formatUnequal will dump the actual and expected values into a textual
// representation and return an error message containing a diff.
// take from gopkg.in check.v1.
func formatUnequal(obtained interface{}, expected interface{}) string {
	// We do not do diffs for basic types because go-check already
	// shows them very cleanly.
	if !diffworthy(obtained) || !diffworthy(expected) {
		return ""
	}

	// Handle strings, short strings are ignored (go-check formats
	// them very nicely already). We do multi-line strings by
	// generating two string slices and using kr.Diff to compare
	// those (kr.Diff does not do string diffs by itself).
	aStr, aOK := obtained.(string)
	bStr, bOK := expected.(string)
	if aOK && bOK {
		l1 := strings.Split(aStr, "\n")
		l2 := strings.Split(bStr, "\n")
		// the "2" here is a bit arbitrary
		if len(l1) > 2 && len(l2) > 2 {
			diff := pretty.Diff(l1, l2)
			return fmt.Sprintf(`String difference:
%s`, formatMultiLine(strings.Join(diff, "\n"), false))
		}
		// string too short
		return ""
	}

	// generic diff
	diff := pretty.Diff(obtained, expected)
	if len(diff) == 0 {
		// No diff, this happens when e.g. just struct
		// pointers are different but the structs have
		// identical values.
		return ""
	}

	return fmt.Sprintf(`Difference:
%s`, formatMultiLine(strings.Join(diff, "\n"), false))
}

func diffworthy(a interface{}) bool {
	t := reflect.TypeOf(a)
	switch t.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.Struct, reflect.String, reflect.Ptr:
		return true
	}
	return false
}

func formatMultiLine(s string, quote bool) []byte {
	b := make([]byte, 0, len(s)*2)
	i := 0
	n := len(s)
	for i < n {
		j := i + 1
		for j < n && s[j-1] != '\n' {
			j++
		}
		b = append(b, "...     "...)
		if quote {
			b = strconv.AppendQuote(b, s[i:j])
		} else {
			b = append(b, s[i:j]...)
			b = bytes.TrimSpace(b)
		}
		if quote && j < n {
			b = append(b, " +"...)
		}
		b = append(b, '\n')
		i = j
	}
	return b
}
