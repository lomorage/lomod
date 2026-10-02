package atlomod

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/testutil"
	"github.com/pkg/errors"
	. "gopkg.in/check.v1"
)

func (ts *lomodAPISuite) waitUntilNoMaintenance(ctx context.Context) error {
	for {
		after := time.After(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			return errors.New("system is alway in maintenance mode")
		case <-after:
			maint, err := ts.lomoc.IsMaintenanceMode()
			if err != nil {
				return err
			}
			if maint {
				continue
			}
			return nil
		}
	}
}

func (ts *lomodAPISuite) TestInconsistentCheckMissPreviewFile(c *C) {
	c.Assert(ts.insertDefaultAssets(), IsNil)

	_, previewDir := common.GetUserPhotoDir(getAliceHome())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c.Assert(testutil.WaitWithFileCounts(ctx, previewDir, defaultPreviewFilesCount), IsNil)
	c.Assert(testutil.CompareDirsByTreeCmd(previewDir, fmt.Sprintf(previewTree1, runtime.GOARCH, ts.osRelease)), IsNil)

	// delete all preview files
	for _, y := range ts.category.Years {
		for _, m := range y.Months {
			for _, d := range m.Days {
				_, dir, err := common.GetUserPhotoMasterPreviewDir(getAliceHome(), y.Year, m.Month, d.Day, 0755)
				c.Assert(err, IsNil)
				files, err := ioutil.ReadDir(dir)
				c.Assert(err, IsNil)
				for _, file := range files {
					c.Assert(os.Remove(filepath.Join(dir, file.Name())), IsNil)
				}
			}
		}
	}

	// trigger ccheck
	c.Assert(ts.lomoc.StartCCheck(), IsNil)

	// wait again
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel2()
	c.Assert(testutil.WaitWithFileCounts(ctx2, previewDir, defaultPreviewFilesCount), IsNil)
	c.Assert(testutil.CompareDirsByTreeCmd(previewDir, fmt.Sprintf(previewTree1, runtime.GOARCH, ts.osRelease)), IsNil)

	c.Assert(ts.waitUntilNoMaintenance(ctx2), IsNil)

	reply, err := ts.lomoc.GetCCheckResult()
	c.Assert(err, IsNil)
	fmt.Printf("------- Consistent Check MissPreview:\n%s\n", reply)
}
