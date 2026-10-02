package atlomod

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/client"
	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/cmd"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/testutil"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	. "gopkg.in/check.v1"
)

func (ts *lomodAPISuite) rmMount(useSystemMethod bool) error {
	cmd.Exec("sync", "-f", defaultHomeDir) // force sync all pending io to avoid unexpected failure
	testutil.Unmount(defaultHomeDir)
	testutil.UnloadMassStorage()

	// retry in case mount is notified
	for i := 0; i < 10; i++ {
		time.Sleep(500 * time.Millisecond)
		if useSystemMethod {
			defaultSubMntDirs, err := ts.lomoc.ListMounts()
			if err != nil {
				if os.IsNotExist(err) || strings.Contains(err.Error(), "input/output error") ||
					strings.Contains(err.Error(), common.ErrDeviceNotMount.Error()) {
					return nil
				}
				logrus.Warnf("while rmMount, probe system api: %v", err)
				continue
			}
			switch len(defaultSubMntDirs) {
			case 1:
				return nil
			case 2:
				if defaultSubMntDirs[1].Dir == "media/home" &&
					(strings.Contains(defaultSubMntDirs[1].Error, "mkdir /media/home/lomod_probe_write: permission denied") ||
						strings.Contains(defaultSubMntDirs[1].Error, "mkdir /media/home/lomod_probe_write: input/output error")) {
					return nil
				}
			}
		} else {
			_, err := os.Stat(getAliceHome())
			if err != nil && (os.IsNotExist(err) || strings.Contains(err.Error(), "input/output error")) {
				return nil
			}
			logrus.Warnf("while rmMount, probe alice home: %v", err)
		}
	}

	return errors.New("remove mount fail")
}

func (ts *lomodAPISuite) validateBackup(bobLomoc *client.Lomod, previewCount int) error {
	// make sure preview is created before preview request
	master, _, err := common.GetUserPhotoMasterPreviewDir(filepath.Join(defaultMntDir, bob),
		ts.category.Years[0].Year, ts.category.Years[0].Months[0].Month,
		ts.category.Years[0].Months[0].Days[0].Day, 0)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	_, preview := common.GetUserPhotoDir(getBobHome())
	if err := testutil.WaitWithFileCounts(ctx, preview, previewCount); err != nil {
		return err
	}

	testAsset1Path := filepath.Join(master, ext.NormalizeAssetNameString(ts.category.Years[0].Year,
		ts.category.Years[0].Months[0].Month, ts.category.Years[0].Months[0].Days[0].Day,
		testAsset1.Name))

	if err := bobLomoc.StartBackup(); err != nil {
		return err
	}

	// retry in case backup is not finished
	var si *types.SystemInfo
	for i := 0; i < 10; i++ {
		si, err = ts.lomoc.System()
		if err == nil && len(si.LastBackup) > 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	bb, ok := si.LastBackup[bob]
	if ok != true {
		return errors.Errorf("last backup result doesn't have bob")
	}
	if bb.AssetRetCode != "" {
		return errors.Errorf("asset backup fail: %s", bb.AssetRetCode)
	}
	_, err = os.Stat(testAsset1Path)
	return err
}

func (ts *lomodAPISuite) TestUSBMountBasicNegative(c *C) {
	/*
		new user: alice (mount USB), bob (local FS)
			 - list mount return /media (should have mount type: local, and empty UUID) and /media/usb1 (should have mount type: USB, and expected UUID).
				* create alice under /media/usb
				* create bob under /medial
			 - system info should return corresponding mount type and UUID for each user (this is new fields in system - userDisk section)
			 - import 1 photo for both alice and bob, it should be success
			 - remove USB, import 2nd photo for both, alice should fail, and bob should success
			 - system info should return failure status for alice
			 - add USB again, import 3rd photo for both, it should be success
			 - system info should return success status for alice
	*/
	bobLomoc, err := ts.createBobUser()
	c.Assert(err, IsNil)

	testAsset2 := ts.category.Years[0].Months[0].Days[0].Assets[1]
	testAsset3 := ts.category.Years[0].Months[1].Days[0].Assets[0]

	_, err = ts.insertAsset(testAsset1)
	c.Assert(err, IsNil)

	_, err = ts.insertAssetWithLomoc(testAsset1, bobLomoc)
	c.Assert(err, IsNil)

	c.Assert(ts.rmMount(true), IsNil)

	si, err := ts.lomoc.System()
	c.Assert(err, IsNil)
	c.Assert(len(si.UserStatus), Equals, 2)

	a, ok := si.UserStatus[alice]
	c.Assert(ok, Equals, true)
	c.Assert(a.HomeDisk.Status, Equals, common.ErrDeviceNotMount.Error())

	b, ok := si.UserStatus[bob]
	c.Assert(ok, Equals, true)
	c.Assert(b.HomeDisk.Status, Equals, "")
	c.Assert(b.BackupDisk.Status, Equals, common.ErrDeviceNotMount.Error())

	_, err = ts.insertAsset(testAsset2)
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), common.ErrDeviceNotMount.Error()), Equals, true)

	_, err = ts.insertAssetWithLomoc(testAsset2, bobLomoc)
	c.Assert(err, IsNil)

	// alice can not delete / get master and preview photos
	_, _, err = ts.lomoc.IsAssetExist(testAsset1.Hash)
	c.Assert(err, NotNil)

	_, err = ts.lomoc.DownloadAsset(testAsset1.Name, false, nil)
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), common.ErrDeviceNotMount.Error()), Equals, true,
		Commentf(err.Error()))

	_, err = ts.lomoc.DownloadAsset(testAsset1.Name, true, nil)
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), common.ErrDeviceNotMount.Error()), Equals, true)

	err = ts.lomoc.DeleteAsset(testAsset1.Name)
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), common.ErrDeviceNotMount.Error()), Equals, true)

	// bob can delete / get master and preview photos
	c.Assert(ts.validateAssetExistLomoc(testAsset1, bobLomoc), IsNil)
	c.Assert(ts.validateAssetDownloadLomoc(testAsset1.Hash, testAsset1.Hash, false, nil, bobLomoc), IsNil)
	c.Assert(bobLomoc.DeleteAsset(testAsset1.Hash), IsNil)

	// mount again and insert
	mountEvent, tmpHomeDiskFile, err := ts.mountDevice(ts.tmpHomeDiskFile, getBobBackup(), defaultHomeDisk)
	c.Assert(err, IsNil)
	c.Assert(tmpHomeDiskFile, Equals, ts.tmpHomeDiskFile)
	logrus.Printf("home disk %s is remounted to %s", ts.tmpHomeDiskFile, mountEvent.MountPath)

	c.Assert(cmd.ExecWithSudo("chown", ts.lomoUser+":"+ts.lomoUser, defaultHomeDir), IsNil)

	// retry in case mount is notified
	for i := 0; i < 10; i++ {
		defaultSubMntDirs, err := ts.lomoc.ListMounts()
		c.Assert(err, IsNil)
		if len(defaultSubMntDirs) == 2 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	// get user status
	si, err = ts.lomoc.System()
	c.Assert(err, IsNil)
	c.Assert(len(si.UserStatus), Equals, 2)

	a, ok = si.UserStatus[alice]
	c.Assert(ok, Equals, true)
	c.Assert(a.HomeDisk.Status, Equals, "")

	b, ok = si.UserStatus[bob]
	c.Assert(ok, Equals, true)
	c.Assert(b.HomeDisk.Status, Equals, "")
	c.Assert(b.BackupDisk.Status, Equals, "")
	c.Assert(b.BackupDisk.FreeSizeInMB, Equals, a.HomeDisk.FreeSizeInMB)

	// insert again
	_, err = ts.insertAsset(testAsset3)
	c.Assert(err, IsNil)

	_, err = ts.insertAssetWithLomoc(testAsset3, bobLomoc)
	c.Assert(err, IsNil)

	// both alice and bob can delete photos now
	c.Assert(ts.validateAssetExist(testAsset3), IsNil)
	c.Assert(ts.validateAssetDownload(testAsset3.Hash, testAsset3.Hash, false, nil), IsNil)
	c.Assert(ts.lomoc.DeleteAsset(testAsset3.Hash), IsNil)

	c.Assert(ts.validateAssetExistLomoc(testAsset3, bobLomoc), IsNil)
	c.Assert(ts.validateAssetDownloadLomoc(testAsset3.Hash, testAsset3.Hash, false, nil, bobLomoc), IsNil)
	c.Assert(bobLomoc.DeleteAsset(testAsset3.Hash), IsNil)
}

func (ts *lomodAPISuite) TestUSBMountMigrationWithMount(c *C) {
	/*
		migration - USB disk is mounted
		 - create alice and bob
		 - import 1 photo to simulate old data
		 - remove UUID from DB directly, then restart app
		 - after app starts, re-run case 1, it should pass
	*/
	bobLomoc, err := ts.createBobUser()
	c.Assert(err, IsNil)

	testAsset2 := ts.category.Years[0].Months[1].Days[0].Assets[0]

	// import bob's asset firstly to make sure backup asset path validation
	_, err = ts.insertAssetWithLomoc(testAsset1, bobLomoc)
	c.Assert(err, IsNil)

	_, err = ts.insertAsset(testAsset1)
	c.Assert(err, IsNil)

	// wait until alice preview is created
	_, previewDir := common.GetUserPhotoDir(getAliceHome())
	ctxPreview, cancelPreview := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancelPreview()
	c.Assert(testutil.WaitWithFileCounts(ctxPreview, previewDir, 6), IsNil)

	c.Assert(os.RemoveAll(common.GetUserConfDir(defaultHomeDir, alice)), IsNil)
	c.Assert(os.RemoveAll(common.GetUserConfDir(defaultMntDir, bob)), IsNil)

	c.Assert(ts.shutdownLomod(), IsNil)
	c.Assert(cmd.ExecWithSudo("sync"), IsNil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	c.Assert(ts.startLomod(ctx, cancel), IsNil)

	// retry in case lomod is still starting
	migrated := false
	for i := 0; i < 20; i++ {
		if err := ts.validateUUID(); err != nil {
			time.Sleep(500 * time.Millisecond)
			c.Assert(cmd.ExecWithSudo("sync"), IsNil)
			continue
		}
		migrated = true
		break
	}
	c.Assert(migrated, Equals, true)

	// now check system API
	si, err := ts.lomoc.System()
	c.Assert(err, IsNil)
	c.Assert(len(si.UserStatus), Equals, 2)
	c.Assert(len(si.LastBackup), Equals, 0)

	a, ok := si.UserStatus[alice]
	c.Assert(ok, Equals, true)
	c.Assert(a.HomeDisk.Status, Equals, "")

	b, ok := si.UserStatus[bob]
	c.Assert(ok, Equals, true)
	c.Assert(b.HomeDisk.Status, Equals, "")

	c.Assert(ts.validateAssetExist(testAsset1), IsNil)
	c.Assert(ts.validateAssetDownload(testAsset1.Hash, testAsset1.Hash, false, nil), IsNil)
	c.Assert(ts.lomoc.DeleteAsset(testAsset1.Hash), IsNil)

	// not delete bob's asset 1 for backup verification
	c.Assert(ts.validateAssetExistLomoc(testAsset1, bobLomoc), IsNil)
	c.Assert(ts.validateAssetDownloadLomoc(testAsset1.Hash, testAsset1.Hash, false, nil, bobLomoc), IsNil)

	// insert 2nd asset and validate download and delete
	_, err = ts.insertAsset(testAsset2)
	c.Assert(err, IsNil)

	_, err = ts.insertAssetWithLomoc(testAsset2, bobLomoc)
	c.Assert(err, IsNil)

	// both alice and bob can delete photos now
	c.Assert(ts.validateAssetExist(testAsset2), IsNil)
	c.Assert(ts.validateAssetDownload(testAsset2.Hash, testAsset2.Hash, false, nil), IsNil)
	c.Assert(ts.lomoc.DeleteAsset(testAsset2.Hash), IsNil)

	c.Assert(ts.validateAssetExistLomoc(testAsset2, bobLomoc), IsNil)
	c.Assert(ts.validateAssetDownloadLomoc(testAsset2.Hash, testAsset2.Hash, false, nil, bobLomoc), IsNil)
	c.Assert(bobLomoc.DeleteAsset(testAsset2.Hash), IsNil)

	// verify backup disk migration also success
	_, err = os.Stat(filepath.Join(common.GetUserConfDir(defaultHomeDir, bob), types.UUIDFilename))
	c.Assert(err, IsNil)

	c.Assert(ts.validateBackup(bobLomoc, 6), IsNil)
}

func (ts *lomodAPISuite) TestUSBMountMigrationWithoutMount(c *C) {
	/*
		migration - USB disk is removed
		- create alice and bob
		- import 1 photo to simulate old data
		- remove UUID from DB directly, remove USB disk, then restart app
		- after migration, system info should return failure status for alice
		- import 2nd photo for both, alice should fail, and bob should success
		- add USB again, import 3rd photo for both, it should be success
		- system info should return success status for alice
		- FS should have correct UUID for alice now
	*/
	bobLomoc, err := ts.createBobUser()
	c.Assert(err, IsNil)

	testAsset2 := ts.category.Years[0].Months[0].Days[0].Assets[1]
	testAsset3 := ts.category.Years[0].Months[1].Days[0].Assets[0]

	_, err = ts.insertAssetWithLomoc(testAsset1, bobLomoc)
	c.Assert(err, IsNil)

	_, err = ts.insertAsset(testAsset1)
	c.Assert(err, IsNil)

	// shutdown until preview generation is done, otherwise, unpredictable backup
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	_, preview := common.GetUserPhotoDir(getBobHome())
	c.Assert(testutil.WaitWithFileCounts(ctx, preview, 6), IsNil)

	c.Assert(ts.shutdownLomod(), IsNil)

	c.Assert(os.RemoveAll(common.GetUserConfDir(defaultHomeDir, alice)), IsNil)
	c.Assert(os.RemoveAll(common.GetUserConfDir(defaultMntDir, bob)), IsNil)

	c.Assert(ts.rmMount(false), IsNil)

	// now start lomod
	c.Assert(ts.startLomod(ctx, cancel), IsNil)

	// retry in case lomod is still starting
	for i := 0; i < 10; i++ {
		time.Sleep(500 * time.Millisecond)
		si, err := ts.lomoc.System()
		if err != nil || len(si.UserStatus) != 2 {
			continue
		}
		c.Assert(err, IsNil)
		c.Assert(len(si.UserStatus), Equals, 2)
		c.Assert(len(si.LastBackup), Equals, 0)

		a, ok := si.UserStatus[alice]
		c.Assert(ok, Equals, true)
		c.Assert(a.HomeDisk.Status, Equals, common.ErrDeviceNotMount.Error())

		b, ok := si.UserStatus[bob]
		c.Assert(ok, Equals, true)
		c.Assert(b.HomeDisk.Status, Equals, "")
		break
	}

	_, err = ts.insertAsset(testAsset2)
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), common.ErrDeviceNotMount.Error()), Equals, true)

	_, err = ts.insertAssetWithLomoc(testAsset2, bobLomoc)
	c.Assert(err, IsNil)

	// alice can not delete / get master and preview photos
	_, _, err = ts.lomoc.IsAssetExist(testAsset1.Hash)
	c.Assert(err, NotNil)

	_, err = ts.lomoc.DownloadAsset(testAsset1.Name, false, nil)
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), common.ErrDeviceNotMount.Error()), Equals, true,
		Commentf(err.Error()))

	_, err = ts.lomoc.DownloadAsset(testAsset1.Name, true, nil)
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), common.ErrDeviceNotMount.Error()), Equals, true)

	err = ts.lomoc.DeleteAsset(testAsset1.Name)
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), common.ErrDeviceNotMount.Error()), Equals, true)

	// bob can delete / get master and preview photos, but backup directory is not migrated
	// keep asset1 for backup testing purpose
	c.Assert(ts.validateAssetExistLomoc(testAsset1, bobLomoc), IsNil)
	c.Assert(ts.validateAssetDownloadLomoc(testAsset1.Hash, testAsset1.Hash, false, nil, bobLomoc), IsNil)

	bobBackupUUID := filepath.Join(common.GetUserConfDir(defaultHomeDir, bob), types.UUIDFilename)
	_, err = os.Stat(bobBackupUUID)
	c.Assert(err, NotNil)
	c.Assert(os.IsNotExist(err), Equals, true)

	c.Assert(testutil.WaitWithFileCounts(ctx, preview, 12), IsNil)

	c.Assert(bobLomoc.StartBackup(), IsNil)

	// retry in case backup is not finished
	var si *types.SystemInfo
	for i := 0; i < 10; i++ {
		si, err = ts.lomoc.System()
		if err == nil && len(si.LastBackup) > 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	bb, ok := si.LastBackup[bob]
	c.Assert(ok, Equals, true, Commentf("%v", si.LastBackup))
	c.Assert(strings.Contains(bb.AssetRetCode, common.ErrDeviceNotMount.Error()), Equals, true,
		Commentf("%v", si.LastBackup))

	// start re-mount
	mountEvent, tmpHomeDiskFile, err := ts.mountDevice(ts.tmpHomeDiskFile, getBobBackup(), defaultHomeDisk)
	c.Assert(err, IsNil)
	c.Assert(tmpHomeDiskFile, Equals, ts.tmpHomeDiskFile)
	logrus.Printf("home disk %s is remounted to %s", ts.tmpHomeDiskFile, mountEvent.MountPath)

	c.Assert(cmd.ExecWithSudo("chown", ts.lomoUser+":"+ts.lomoUser, defaultHomeDir), IsNil)

	// retry in case mount is notified
	for i := 0; i < 10; i++ {
		defaultSubMntDirs, err := ts.lomoc.ListMounts()
		c.Assert(err, IsNil)
		if len(defaultSubMntDirs) == 2 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	// get user status
	si, err = ts.lomoc.System()
	c.Assert(err, IsNil)
	c.Assert(len(si.UserStatus), Equals, 2)

	a, ok := si.UserStatus[alice]
	c.Assert(ok, Equals, true)
	c.Assert(a.HomeDisk.Status, Equals, "")

	b, ok := si.UserStatus[bob]
	c.Assert(ok, Equals, true)
	c.Assert(b.HomeDisk.Status, Equals, "")
	c.Assert(b.BackupDisk, NotNil)
	c.Assert(b.BackupDisk.Status, Equals, "")

	// insert again
	_, err = ts.insertAsset(testAsset3)
	c.Assert(err, IsNil)

	_, err = ts.insertAssetWithLomoc(testAsset3, bobLomoc)
	c.Assert(err, IsNil)

	// both alice and bob can delete photos now
	c.Assert(ts.validateAssetExist(testAsset3), IsNil)
	c.Assert(ts.validateAssetDownload(testAsset3.Hash, testAsset3.Hash, false, nil), IsNil)
	c.Assert(ts.lomoc.DeleteAsset(testAsset3.Hash), IsNil)

	c.Assert(ts.validateAssetExistLomoc(testAsset3, bobLomoc), IsNil)
	c.Assert(ts.validateAssetDownloadLomoc(testAsset3.Hash, testAsset3.Hash, false, nil, bobLomoc), IsNil)
	c.Assert(bobLomoc.DeleteAsset(testAsset3.Hash), IsNil)

	// verify backup directory migration
	_, err = os.Stat(bobBackupUUID)
	c.Assert(err, IsNil)

	// now trigger backup again and start validate result
	c.Assert(bobLomoc.StartBackup(), IsNil)

	// retry in case backup is not finished
	for i := 0; i < 10; i++ {
		si, err = ts.lomoc.System()
		if err != nil || len(si.LastBackup) == 0 {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		bb, ok := si.LastBackup[bob]
		if !ok {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if bb.AssetRetCode != "" {
			time.Sleep(500 * time.Millisecond)
			continue
		}
	}

	c.Assert(ts.validateBackup(bobLomoc, 12), IsNil)
}

func (ts *lomodAPISuite) TestUSBMountBackupBasic(c *C) {
	/*
		backup disk is mounted
		- bob's backup directory is /media/home
		- ingest the 1st content
		- trigger backup, which should be success from system API
		- remove mount disk
		- ingest the 2nd content
		- trigger backup, which should be fail
		- backup result should specify this failure from system API
		- mount disk and trigger backup again, which should be success
		- backup result should specify success now
	*/
	testAsset2 := ts.category.Years[0].Months[0].Days[0].Assets[1]
	master, _, err := common.GetUserPhotoMasterPreviewDir(filepath.Join(defaultHomeDir, bob),
		ts.category.Years[0].Year, ts.category.Years[0].Months[0].Month,
		ts.category.Years[0].Months[0].Days[0].Day, 0)
	c.Assert(err, IsNil)
	testAsset2Path := filepath.Join(master, ext.NormalizeAssetNameString(ts.category.Years[0].Year,
		ts.category.Years[0].Months[0].Month, ts.category.Years[0].Months[0].Days[0].Day,
		testAsset2.Name))

	bobLomoc, err := ts.createBobUser()
	c.Assert(err, IsNil)
	_, err = ts.insertAssetWithLomoc(testAsset1, bobLomoc)
	c.Assert(err, IsNil)

	c.Assert(ts.validateBackup(bobLomoc, 6), IsNil)

	// remove mount dir
	c.Assert(ts.rmMount(true), IsNil)

	// ingest 2nd asset
	_, err = ts.insertAssetWithLomoc(testAsset2, bobLomoc)
	c.Assert(err, IsNil)

	c.Assert(bobLomoc.StartBackup(), IsNil)

	// retry in case backup is not finished
	for i := 0; i < 10; i++ {
		si, err := ts.lomoc.System()
		if err == nil && si.LastBackup[bob].AssetRetCode != "" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	_, err = os.Stat(testAsset2Path)
	c.Assert(err, NotNil)

	// remount dir
	mountEvent, tmpHomeDiskFile, err := ts.mountDevice(ts.tmpHomeDiskFile, getBobBackup(), defaultHomeDisk)
	c.Assert(err, IsNil)
	c.Assert(tmpHomeDiskFile, Equals, ts.tmpHomeDiskFile)
	logrus.Printf("home disk %s is remounted to %s", ts.tmpHomeDiskFile, mountEvent.MountPath)

	c.Assert(bobLomoc.StartBackup(), IsNil)
	for i := 0; i < 10; i++ {
		si, err := ts.lomoc.System()
		if err == nil && si.LastBackup[bob].AssetRetCode == "" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	_, err = os.Stat(testAsset2Path)
	c.Assert(err, IsNil)
}

func (ts *lomodAPISuite) TestUmount(c *C) {
	c.Assert(ts.lomoc.Unmount(defaultHomeDir), IsNil)

	// probe user status
	unmounted := false
	for i := 0; i < 10; i++ {
		si, err := ts.lomoc.System()
		c.Assert(err, IsNil)
		a, ok := si.UserStatus[alice]
		c.Assert(ok, Equals, true)
		if a.HomeDisk.Status == common.ErrDeviceNotMount.Error() {
			unmounted = true
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	c.Assert(unmounted, Equals, true)
}

func (ts *lomodAPISuite) restartLomodWithSubMntdir(c *C) {
	c.Assert(os.Mkdir(defaultSubMntDir, 0755), IsNil)

	c.Assert(ts.shutdownLomod(), IsNil)

	ctx, cancel := context.WithCancel(context.Background())
	_, err := testutil.StartDaemon(ctx, cancel, nil, filepath.Join(common.GetBinDir(ts.baseDir), "lomod"),
		"-b", ts.baseDir, "--mount-dir", defaultSubMntDir, "-p", strconv.Itoa(ts.listenPort))
	c.Assert(err, IsNil)

	started := false
	for i := 0; i < 200; i++ {
		_, err := ts.lomoc.System()
		if err == nil {
			started = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	c.Assert(started, Equals, true)

}

func (ts *lomodAPISuite) TestMountSubDir(c *C) {
	// 1. restart lomod and change mount dir
	// 2. create bob with new mount dir
	// 3. insert 1 assets for both alice and bob should be success, and validate location
	// 4. download assets for both alice and bob should be success
	// 5. unmount, download should fail
	// 6. mount again, download should be success
	ts.restartLomodWithSubMntdir(c)

	// add bob user
	u := &user.User{Name: bob, Password: bobPwd, HomeDir: defaultSubMntDir}
	c.Assert(ts.lomoc.AddUser(u), IsNil)

	bobLomoc := client.NewLomod("127.0.0.1:" + strconv.Itoa(ts.listenPort))
	_, err := bobLomoc.Login(bob, bobPwd)
	c.Assert(err, IsNil)

	// insert asset for both alice and bob
	_, err = ts.insertAsset(testAsset1)
	c.Assert(err, IsNil)

	_, err = ts.insertAssetWithLomoc(testAsset1, bobLomoc)
	c.Assert(err, IsNil)

	// validate the asset for alice
	master, _, err := common.GetUserPhotoMasterPreviewDir(getAliceHome(),
		ts.category.Years[0].Year, ts.category.Years[0].Months[0].Month,
		ts.category.Years[0].Months[0].Days[0].Day, 0)
	c.Assert(err, IsNil)

	testAsset1Path := filepath.Join(master, ext.NormalizeAssetNameString(ts.category.Years[0].Year,
		ts.category.Years[0].Months[0].Month, ts.category.Years[0].Months[0].Days[0].Day,
		testAsset1.Name))

	_, err = os.Stat(testAsset1Path)
	c.Assert(err, IsNil)

	master, _, err = common.GetUserPhotoMasterPreviewDir(filepath.Join(defaultSubMntDir, bob),
		ts.category.Years[0].Year, ts.category.Years[0].Months[0].Month,
		ts.category.Years[0].Months[0].Days[0].Day, 0)
	c.Assert(err, IsNil)

	testAsset1Path = filepath.Join(master, ext.NormalizeAssetNameString(ts.category.Years[0].Year,
		ts.category.Years[0].Months[0].Month, ts.category.Years[0].Months[0].Days[0].Day,
		"2.jpg"))

	_, err = os.Stat(testAsset1Path)
	c.Assert(err, IsNil)

	// download assets now
	_, err = ts.lomoc.DownloadAsset(testAsset1.Hash, false, nil)
	c.Assert(err, IsNil)
	_, err = bobLomoc.DownloadAsset(testAsset1.Hash, false, nil)
	c.Assert(err, IsNil)

	si, err := ts.lomoc.System()
	c.Assert(err, IsNil)
	status, ok := si.UserStatus[alice]
	c.Assert(ok, Equals, true)
	c.Assert(status.HomeDisk.Status, Equals, "")
	status, ok = si.UserStatus[bob]
	c.Assert(ok, Equals, true)
	c.Assert(status.HomeDisk.Status, Equals, "")

	// remove mount
	c.Assert(ts.rmMount(true), IsNil)
	c.Assert(ts.rmMount(false), IsNil)

	//defaultSubMntDirs, err := ts.lomoc.ListMounts()

	si, err = ts.lomoc.System()
	c.Assert(err, IsNil)
	c.Assert(len(si.UserStatus), Equals, 2)

	a, ok := si.UserStatus[alice]
	c.Assert(ok, Equals, true)
	c.Assert(a.HomeDisk.Status, Equals, common.ErrDeviceNotMount.Error())

	b, ok := si.UserStatus[bob]
	c.Assert(ok, Equals, true)
	c.Assert(b.HomeDisk.Status, Equals, common.ErrDeviceNotMount.Error())

	_, err = ts.lomoc.DownloadAsset(testAsset1.Hash, false, nil)
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), common.ErrDeviceNotMount.Error()), Equals, true,
		Commentf(err.Error()))

	_, err = bobLomoc.DownloadAsset(testAsset1.Hash, false, nil)
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), common.ErrDeviceNotMount.Error()), Equals, true)

	// mount again
	mountEvent, tmpHomeDiskFile, err := ts.mountDevice(ts.tmpHomeDiskFile, "", defaultHomeDisk)
	c.Assert(err, IsNil)
	ts.tmpHomeDiskFile = tmpHomeDiskFile
	logrus.Printf("home disk %s is mounted to %s", ts.tmpHomeDiskFile, mountEvent.MountPath)
	c.Assert(mountEvent.MountPath, Equals, defaultHomeDir)

	mounted := false
	for i := 0; i < 200; i++ {
		time.Sleep(200 * time.Millisecond)
		si, err = ts.lomoc.System()
		if err != nil {
			logrus.Infof("system return %s", err)
			continue
		}
		logrus.Infof("system info: %+v", si)
		status, ok = si.UserStatus[alice]
		if !ok {
			continue
		}
		if status.HomeDisk.Status != "" {
			continue
		}
		status, ok = si.UserStatus[bob]
		if !ok {
			continue
		}
		if status.HomeDisk.Status != "" {
			continue
		}
		mounted = true
		break
	}
	c.Assert(mounted, Equals, true)

	_, err = ts.lomoc.DownloadAsset(testAsset1.Hash, false, nil)
	c.Assert(err, IsNil)
	_, err = bobLomoc.DownloadAsset(testAsset1.Hash, false, nil)
	c.Assert(err, IsNil)

	// ingest second file
	testAsset2 := ts.category.Years[0].Months[0].Days[0].Assets[1]
	_, err = ts.insertAsset(testAsset2)
	c.Assert(err, IsNil)
	_, err = ts.insertAssetWithLomoc(testAsset2, bobLomoc)
	c.Assert(err, IsNil)

	_, err = ts.lomoc.DownloadAsset(testAsset2.Hash, false, nil)
	c.Assert(err, IsNil)
	_, err = bobLomoc.DownloadAsset(testAsset2.Hash, false, nil)
	c.Assert(err, IsNil)
}

func (ts *lomodAPISuite) TestMountSubDirBackupCreateUser(c *C) {
	// 1. restart lomod and change mount dir
	// 2. create new mount and run bind mount to mount to mount dir specified at step 1
	// 3. create bob with new mount dir and backup dir
	// 4. insert the 1st asset for both alice and bob should be success, and validate location
	// 5. download assets for both alice and bob should be success
	// 6. backup should be success
	// 7. unmount and mount again
	// 8. insert the 2nd asset, and backup should be success

	ts.restartLomodWithSubMntdir(c)

	// now mount the 2nd device
	diskFile, loopDevice, err := testutil.CreateLoopDevice(defaultUSBMockSize)
	c.Assert(err, IsNil)
	c.Assert(cmd.ExecWithSudo("mkdir", "-p", defaultMntBackupDir), IsNil)
	c.Assert(cmd.ExecWithSudo("mount", loopDevice, defaultMntBackupDir), IsNil)
	logrus.Printf("the 2nd disk %s is created at %s, and mounted to %s", diskFile, loopDevice, defaultMntBackupDir)

	c.Assert(ts.checkMount(defaultMntBackupDir, ""), IsNil)
	c.Assert(cmd.ExecWithSudo("chown", ts.lomoUser+":"+ts.lomoUser, defaultMntBackupDir), IsNil)

	// bind mount the directory to home directory
	c.Assert(cmd.ExecWithSudo("mkdir", "-p", bindMntBackupDir), IsNil)
	c.Assert(cmd.ExecWithSudo("mount", "-o", "bind", defaultMntBackupDir, bindMntBackupDir), IsNil)
	defer cmd.ExecWithSudo("umount", bindMntBackupDir)
	c.Assert(ts.checkMount(bindMntBackupDir, ""), IsNil)

	c.Assert(cmd.ExecWithSudo("chown", ts.lomoUser+":"+ts.lomoUser, bindMntBackupDir), IsNil)

	mounts, err := ts.lomoc.ListMounts()
	c.Assert(err, IsNil)
	c.Assert(len(mounts), Equals, 2, Commentf("%v", mounts))
	c.Assert(mounts[0].Dir, Equals, "subdir")
	c.Assert(mounts[1].Dir, Equals, "subdir/backup")

	u := &user.User{Name: bob, Password: bobPwd, HomeDir: defaultSubMntDir, BackupDir: bindMntBackupDir}
	c.Assert(ts.lomoc.AddUser(u), IsNil)

	bobBackupDir := filepath.Join(bindMntBackupDir, "bob")
	c.Assert(ts.checkMount(bindMntBackupDir, bobBackupDir), IsNil)

	bobLomoc := client.NewLomod("127.0.0.1:" + strconv.Itoa(ts.listenPort))
	_, err = bobLomoc.Login(bob, bobPwd)
	c.Assert(err, IsNil)

	_, err = ts.insertAssetWithLomoc(testAsset1, bobLomoc)
	c.Assert(err, IsNil)

	// validate the asset for alice
	master, preview, err := common.GetUserPhotoMasterPreviewDir(filepath.Join(defaultSubMntDir, bob),
		ts.category.Years[0].Year, ts.category.Years[0].Months[0].Month,
		ts.category.Years[0].Months[0].Days[0].Day, 0)
	c.Assert(err, IsNil)

	testAsset1Path := filepath.Join(master, ext.NormalizeAssetNameString(ts.category.Years[0].Year,
		ts.category.Years[0].Months[0].Month, ts.category.Years[0].Months[0].Days[0].Day,
		testAsset1.Name))

	_, err = os.Stat(testAsset1Path)
	c.Assert(err, IsNil)

	// download assets now
	_, err = bobLomoc.DownloadAsset(testAsset1.Hash, false, nil)
	c.Assert(err, IsNil)

	// wait until preview finished because startup can not start until preview is done
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	c.Assert(testutil.WaitWithFileCounts(ctx, preview, 6), IsNil)

	c.Assert(bobLomoc.StartBackup(), IsNil)

	c.Assert(testutil.WaitWithFileCounts(ctx, bobBackupDir+"/Photos", 1), IsNil)

	var si *types.SystemInfo
	for i := 0; i < 10; i++ {
		si, err = ts.lomoc.System()
		if err == nil && len(si.LastBackup) > 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	bb, ok := si.LastBackup[bob]
	c.Assert(ok, Equals, true, Commentf("%v", si.LastBackup))
	c.Assert(bb.AssetRetCode, Equals, "")

	// umount
	c.Assert(cmd.ExecWithSudo("umount", bindMntBackupDir), IsNil)

	si, err = ts.lomoc.System()
	c.Assert(err, IsNil)
	b, ok := si.UserStatus[bob]
	c.Assert(ok, Equals, true)
	c.Assert(b.BackupDisk, NotNil)
	c.Assert(b.BackupDisk.Status, Equals, common.ErrDeviceNotMount.Error())

	// mount again, and everything should be good now
	c.Assert(cmd.ExecWithSudo("mount", "-o", "bind", defaultMntBackupDir, bindMntBackupDir), IsNil)
	si, err = ts.lomoc.System()
	c.Assert(err, IsNil)
	b, ok = si.UserStatus[bob]
	c.Assert(ok, Equals, true)
	c.Assert(b.BackupDisk, NotNil)
	c.Assert(b.BackupDisk.Status, Equals, "")

	testAsset2 := ts.category.Years[0].Months[0].Days[0].Assets[1]
	master, preview, err = common.GetUserPhotoMasterPreviewDir(filepath.Join(defaultSubMntDir, bob),
		ts.category.Years[0].Year, ts.category.Years[0].Months[0].Month,
		ts.category.Years[0].Months[0].Days[0].Day, 0)
	c.Assert(err, IsNil)
	testAsset2Path := filepath.Join(master, ext.NormalizeAssetNameString(ts.category.Years[0].Year,
		ts.category.Years[0].Months[0].Month, ts.category.Years[0].Months[0].Days[0].Day,
		testAsset2.Name))

	_, err = ts.insertAssetWithLomoc(testAsset2, bobLomoc)
	c.Assert(err, IsNil)

	_, err = os.Stat(testAsset2Path)
	c.Assert(err, IsNil)

	// download assets now
	_, err = bobLomoc.DownloadAsset(testAsset2.Hash, false, nil)
	c.Assert(err, IsNil)

	c.Assert(testutil.WaitWithFileCounts(ctx, preview, 12), IsNil)

	c.Assert(bobLomoc.StartBackup(), IsNil)

	// 2nd asset is live photo
	c.Assert(testutil.WaitWithFileCounts(ctx, bobBackupDir+"/Photos", 3), IsNil)
}
