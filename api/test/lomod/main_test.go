package atlomod

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	. "testing"
	"time"

	"bitbucket.org/lomoware/lomo-backend/client"
	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/cmd"
	"bitbucket.org/lomoware/lomo-backend/common/mountinfo"
	"bitbucket.org/lomoware/lomo-backend/common/testutil"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	. "gopkg.in/check.v1"
)

const (
	LomodEnvLomoUser  = "LOMOD_ENV_LOMO_USER"
	LomodEnvBaseDir   = "LOMOD_ENV_BASE_DIR"
	LomodEnvMaxUpload = "LOMOD_ENV_MAX_UPLOAD"
	LomodEnvHTTPPort  = "LOMOD_ENV_HTTP_PORT"
	LomodEnvOSRelease = "LOMOD_ENV_OS_RELEASE"
	LomodEnvOSDistro  = "LOMOD_ENV_OS_DISTRO"
	LomodEnvUnMount   = "LOMOD_ENV_UNMOUNT"

	defaultLomoUser  = "pi"
	defaultBaseDir   = "/opt/lomorage/apitest"
	defaultMaxUpload = 1
	defaultHTTPPort  = 8055
	defaultOSRelease = "focal"
	defaultOSDistro  = "ubuntu"

	defaultMntDir       = "/media"
	defaultHomeDisk     = "home"
	defaultHomeDir      = defaultMntDir + "/" + defaultHomeDisk
	defaultSubMntDir    = defaultHomeDir + "/" + "subdir"
	defaultMntBackupDir = defaultMntDir + "/" + "backup"
	bindMntBackupDir    = defaultSubMntDir + "/" + "backup"
	defaultUSBMockSize  = 64 * 1024 * 1024
	usbMockFreeSizeMB   = uint64(56) // 56M has one lost+found file
	usbMockTotalSizeMB  = uint64(57) // 57M after partition table

	alice    = "alice"
	alicePwd = "alice123"

	baseWorkdir  = "../../.."
	testAssetDir = "cmd/lomod/test"
	testDataDir  = "testdata"

	defaultPreviewFilesCount = 98
)

var (
	testAsset1      types.Asset
	assetInfo       = filepath.Join(testDataDir, "assets_info.json")
	categoryFile    = filepath.Join(testDataDir, "assets_all.json")
	categoryDelete1 = filepath.Join(testDataDir, "assets_delete1.json")
	categoryDelete2 = filepath.Join(testDataDir, "assets_delete2.json")
	masterTree      = filepath.Join(testDataDir, "tree_master.txt")
	trashTree       = filepath.Join(testDataDir, "tree_trash.txt")
	previewTree1    = filepath.Join(testDataDir, "tree_preview_insert_%s_%s.txt")
	previewTree2    = filepath.Join(testDataDir, "tree_preview_all_%s_%s.txt")
	previewTree3    = filepath.Join(testDataDir, "tree_preview_jitt_%s_%s.txt")

	defaultMntDirInfo = types.MountDir{
		Dir:  filepath.Base(defaultMntDir),
		Type: types.MountLocal,
	}
	defaultHomeDirInfo = types.MountDir{
		Dir:       filepath.Join(filepath.Base(defaultMntDir), defaultHomeDisk),
		Type:      types.MountUSB,
		TotalSize: usbMockTotalSizeMB,
	}
)

type lomodAPISuite struct {
	gCtx    context.Context
	gCancel context.CancelFunc

	lomoUser   string
	osDistro   string
	osRelease  string
	baseDir    string
	maxUpload  int
	listenPort int
	lomoc      *client.Lomod

	assetsInfo map[string]AssetInfo
	category   types.Years

	tmpHomeDiskFile   string
	tmpHomeDiskUUID   string
	tmpBackupDiskFile string
}

var _ = Suite(&lomodAPISuite{assetsInfo: map[string]AssetInfo{}})

func TestLomodAPISuite(t *T) {
	TestingT(t)
}

func (ts *lomodAPISuite) SetUpSuite(c *C) {
	content, err := ioutil.ReadFile(assetInfo)
	c.Assert(err, IsNil)

	c.Assert(json.Unmarshal(content, &ts.assetsInfo), IsNil)

	ts.lomoUser = os.Getenv(LomodEnvLomoUser)
	if ts.lomoUser == "" {
		ts.lomoUser = defaultLomoUser
	}
	ts.baseDir = os.Getenv(LomodEnvBaseDir)
	if ts.baseDir == "" {
		ts.baseDir = defaultBaseDir
	}
	ts.osDistro = os.Getenv(LomodEnvOSDistro)
	if ts.osDistro == "" {
		ts.osDistro = defaultOSDistro
	}
	ts.osRelease = os.Getenv(LomodEnvOSRelease)
	if ts.osRelease == "" {
		ts.osRelease = defaultOSRelease
	}
	ts.maxUpload, err = strconv.Atoi(os.Getenv(LomodEnvMaxUpload))
	if err != nil {
		ts.maxUpload = defaultMaxUpload
	}
	ts.listenPort, err = strconv.Atoi(os.Getenv(LomodEnvHTTPPort))
	if err != nil {
		ts.listenPort = defaultHTTPPort
	}

	ts.lomoc = client.NewLomod("127.0.0.1:" + strconv.Itoa(ts.listenPort))
}

func (ts *lomodAPISuite) SetUpTest(c *C) {
	if ts.gCtx != nil {
		ts.gCancel()
	}

	ts.shutdownLomod()

	ts.cleanup(c)

	ts.gCtx, ts.gCancel = context.WithCancel(context.Background())
	c.Assert(ts.startLomod(ts.gCtx, ts.gCancel), IsNil)

	ts.validateNewSystem(c)

	ts.initMount(c)

	ts.addInitUser(c)

	var err error
	ts.category, err = loadCategory(categoryFile)
	c.Assert(err, IsNil)

	testAsset1 = ts.category.Years[0].Months[0].Days[0].Assets[0]
}

func (ts *lomodAPISuite) TearDownTest(c *C) {
	if os.Getenv(LomodEnvUnMount) != "" {
		c.Assert(ts.rmMount(false), IsNil)
		if ts.tmpHomeDiskFile != "" {
			c.Assert(os.Remove(ts.tmpHomeDiskFile), IsNil)
		}
		if ts.tmpBackupDiskFile != "" {
			c.Assert(os.Remove(ts.tmpBackupDiskFile), IsNil)
		}
	}
}

func (ts *lomodAPISuite) shutdownLomod() error {
	if err := testutil.PgrepDaemon("lomod"); err != nil {
		logrus.Infof("pgrep -x lomod: %s", err)
		return nil
	}
	// kill and probe lomod 10 times to make sure it is killed
	for i := 0; i < 10; i++ {
		testutil.PkillBinary("lomod")
		time.Sleep(500 * time.Millisecond)
		if err := testutil.PgrepDaemon("lomod"); err != nil {
			return nil
		}
	}
	return errors.New("kill lomod fail after 10 retry")
}

func (ts *lomodAPISuite) startLomod(ctx context.Context, cancel context.CancelFunc) error {
	pid, err := testutil.StartDaemon(ctx, cancel, nil, filepath.Join(common.GetBinDir(ts.baseDir), "lomod"),
		"-b", ts.baseDir, "--max-upload", strconv.Itoa(ts.maxUpload), "-p", strconv.Itoa(ts.listenPort))
	logrus.Infof("Started lomod with pid %d", pid)
	return err
}

func (ts *lomodAPISuite) cleanup(c *C) {
	for _, d := range []string{
		common.GetConfDir(ts.baseDir),
		common.GetVarDir(ts.baseDir),
		common.GetSambaUserDir(ts.baseDir),
	} {
		c.Assert(os.RemoveAll(d), IsNil)
	}

	c.Assert(cmd.ExecWithSudo("mkdir", "-p", defaultMntDir), IsNil)
	c.Assert(cmd.ExecWithSudo("chmod", "0755", defaultMntDir), IsNil)

	// umount and unload storage module firstly
	testutil.Unmount(defaultMntBackupDir)
	c.Assert(ts.rmMount(false), IsNil)

	deleted := false
	// retry rm in case unload is not finished
	for i := 0; i < 60; i++ {
		err := cmd.ExecWithSudo("rm", "-rf", defaultMntDir)
		if err == nil {
			deleted = true
			break
		}
		logrus.Warnf("while cleanup, rm %s: %s", defaultMntDir, err)
		time.Sleep(500 * time.Millisecond)
	}
	c.Assert(deleted, Equals, true)
	c.Assert(cmd.ExecWithSudo("sync"), IsNil)
	c.Assert(cmd.ExecWithSudo("mkdir", "-p", defaultMntDir), IsNil)
	c.Assert(cmd.ExecWithSudo("chmod", "0755", defaultMntDir), IsNil)
}

func (ts *lomodAPISuite) validateNewSystem(c *C) {
	// lomod maybe not start yet, retry several times
	var (
		si  *types.SystemInfo
		err error
	)
	for i := 0; i < 200; i++ {
		si, err = ts.lomoc.System()
		if err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	c.Assert(si, NotNil)

	// skip version compare
	si.LomodVersion = ""
	expectNewSystemStatus := types.SystemInfo{
		SystemStatus: common.SystemStatusNew, OS: runtime.GOOS, Arch: runtime.GOARCH, APIVersion: "1.0",
		UserStatus: map[string]types.UserStatus{}, ListenIPs: []net.IP{}, PublicAddr: []string{},
		UserDisks: []types.UserDisk{}, WebpPreview: true,
	}
	c.Assert(si.UUID, Not(Equals), "")
	_, err = uuid.Parse(si.UUID)
	c.Assert(err, IsNil)
	expectNewSystemStatus.UUID = si.UUID
	c.Assert(*si, DeepEquals, expectNewSystemStatus)
}

func (ts *lomodAPISuite) validateMount(c *C, expectMntDirs []types.MountDir) {
	mntDirs, err := ts.lomoc.ListMounts()
	c.Assert(err, IsNil)

	// skip free size validation which may change during test
	for i := 0; i < len(mntDirs); i++ {
		mntDirs[i].FreeSize = 0
	}
	c.Assert(mntDirs, DeepEquals, expectMntDirs)
}

func (ts *lomodAPISuite) mountDevice(diskFile, file, mntDir string) (testutil.MountNotify, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	done := make(chan testutil.MountNotify)
	defer close(done)
	go func() {
		if err := testutil.MonitorMassStorageMount(ctx, defaultMntDir, done); err != nil {
			logrus.Warnf("monitor mass storage mount: %s", err)
			if !strings.Contains(err.Error(), context.Canceled.Error()) &&
				!strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
				done <- testutil.MountNotify{MountPath: "failure"}
			}
		}
	}()
	if diskFile == "" {
		var err error
		diskFile, err = testutil.CreateAndMount(defaultUSBMockSize, mntDir)
		if err != nil {
			return testutil.MountNotify{}, "", err
		}
	} else {
		if err := testutil.MountWithFile(diskFile, defaultHomeDisk); err != nil {
			return testutil.MountNotify{}, "", err
		}
	}

	mountEvent := <-done

	return mountEvent, diskFile, ts.checkMount(defaultHomeDir, file)
}

func (ts *lomodAPISuite) checkMount(expectMntPoint, validateFile string) error {
	for i := 0; i < 20; i++ {
		mounts, err := mountinfo.GetMounts(nil)
		if err != nil {
			return err
		}
		found := false
		for _, m := range mounts {
			if m.Mountpoint == expectMntPoint {
				found = true
			}
		}
		if found {
			if validateFile == "" {
				return nil
			}
			// try to validate file
			_, err := os.Stat(validateFile)
			if err == nil {
				si, err := ts.lomoc.System()
				if err == nil && si.UserStatus[alice].HomeDisk.Status == "" && si.UserStatus[bob].BackupDisk.Status == "" {
					return nil
				}
				logrus.Warnf("file exist, but system status is not ready: %+v, %v", si, err)
			} else {
				logrus.Warnf("mount exist: %+v, but file probe: %+v", mounts, err)
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	return errors.New("usb disk is not mounted")
}

func (ts *lomodAPISuite) initMount(c *C) {
	m, err := mountinfo.GetMountDir(defaultMntDir, defaultMntDir)
	// without mounting any disk, it should be only default dir
	baseMnt := defaultMntDirInfo
	baseMnt.TotalSize = m.TotalSize
	baseMnt.Error = "mkdir " + defaultMntDir + "/lomod_probe_write: permission denied"
	ts.validateMount(c, []types.MountDir{baseMnt})

	// change /media permission to 777
	c.Assert(cmd.ExecWithSudo("chmod", "0777", defaultMntDir), IsNil)
	baseMnt.Error = ""
	ts.validateMount(c, []types.MountDir{baseMnt})

	// mount home dir, it should return both default and home dir
	mountEvent, tmpHomeDiskFile, err := ts.mountDevice("", "", defaultHomeDisk)
	c.Assert(err, IsNil)
	ts.tmpHomeDiskFile = tmpHomeDiskFile
	logrus.Printf("home disk %s is mounted to %s", ts.tmpHomeDiskFile, mountEvent.MountPath)
	c.Assert(mountEvent.MountPath, Equals, defaultHomeDir)

	ts.tmpHomeDiskUUID = mountEvent.UUID

	// run pull again to find out the latest size
	m, err = mountinfo.GetMountDir(defaultMntDir, defaultMntDir)
	c.Assert(err, IsNil)

	// by default home directory is not writtable
	homeDir := defaultHomeDirInfo
	homeDir.UUID = mountEvent.UUID
	homeDir.Error = "mkdir " + defaultHomeDir + "/lomod_probe_write: permission denied"
	ts.validateMount(c, []types.MountDir{baseMnt, homeDir})

	// change owner to pi
	c.Assert(cmd.ExecWithSudo("chown", ts.lomoUser+":"+ts.lomoUser, defaultHomeDir), IsNil)
	homeDir.Error = ""
	ts.validateMount(c, []types.MountDir{baseMnt, homeDir})
}

func (ts *lomodAPISuite) addInitUser(c *C) {
	u := &user.User{Name: alice, Password: alicePwd, HomeDir: defaultHomeDir, NickName: alice + "-nick"}
	c.Assert(ts.lomoc.AddUser(u), IsNil)

	// validate UUID
	uuid, err := getAliceUUID()
	c.Assert(err, IsNil)
	c.Assert(uuid, Equals, types.MountUSB+types.UUIDDelimiter+ts.tmpHomeDiskUUID)

	_, err = ts.lomoc.Login(alice, alicePwd)
	c.Assert(err, IsNil)

	si, err := ts.lomoc.System()
	c.Assert(err, IsNil)
	c.Assert(len(si.UserStatus), Equals, 1)
	status, ok := si.UserStatus[alice]
	c.Assert(ok, Equals, true)
	c.Assert(status.HomeDisk.Status, Equals, "")
	c.Assert(status.HomeDisk.FreeSizeInMB, Equals, usbMockFreeSizeMB)
	c.Assert(status.BackupDisk, IsNil)

	c.Assert(len(si.UserDisks), Equals, 1)
	c.Assert(si.UserDisks[0].Username, Equals, alice)
	c.Assert(si.UserDisks[0].FreeSize, Equals, usbMockFreeSizeMB)
	c.Assert(si.UserDisks[0].Error, Equals, "")
}

func (ts *lomodAPISuite) insertAsset(a types.Asset) (*types.Asset, error) {
	return ts.insertAssetWithLomoc(a, ts.lomoc)
}

func (ts *lomodAPISuite) insertAssetWithLomoc(a types.Asset, lomoc *client.Lomod) (*types.Asset, error) {
	baseMediaDir := filepath.Join(baseWorkdir, testAssetDir)
	file, ok := ts.assetsInfo[a.Name]
	if !ok {
		return nil, errors.Errorf("not found %s in asset map", a.Name)
	}
	f, err := os.Open(filepath.Join(baseMediaDir, file.Path))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return lomoc.UploadAsset(f, nil, a.Hash, a.Date.Time, true, nil)
}

func (ts *lomodAPISuite) insertDefaultAssets() error {
	for _, y := range ts.category.Years {
		for _, m := range y.Months {
			for _, d := range m.Days {
				for _, a := range d.Assets {
					_, err := ts.insertAsset(a)
					if err != nil {
						return err
					}
				}
			}
		}
	}

	// compare tree
	masterDir, _ := common.GetUserPhotoDir(getAliceHome())
	return testutil.CompareDirsByTreeCmd(masterDir, masterTree)
}

func loadCategory(fname string) (types.Years, error) {
	years := types.Years{}
	contents, err := ioutil.ReadFile(fname)
	if err != nil {
		return years, err
	}

	return years, json.Unmarshal(contents, &years)
}

func getAliceHome() string {
	return filepath.Join(defaultHomeDir, alice)
}

func getUUID(confDir string) (string, error) {
	contents, err := ioutil.ReadFile(filepath.Join(confDir, types.UUIDFilename))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(contents)), nil
}

func getAliceUUID() (string, error) {
	return getUUID(common.GetUserConfDir(defaultHomeDir, alice))
}

func (ts *lomodAPISuite) validateUUID() error {
	// validate home directory UUID
	uuid, err := getBobUUID()
	if err != nil {
		return err
	}
	exp := types.MountLocal + types.UUIDDelimiter
	if exp != uuid {
		return errors.Errorf("expect uuid %s, got %s", exp, uuid)
	}

	// validate backup directory UUID
	uuid, err = getBobBackupUUID()
	if err != nil {
		return err
	}
	if !strings.HasPrefix(uuid, types.MountUSB+types.UUIDDelimiter) {
		return errors.Errorf("expect uuid start with UBS, got %s", uuid)
	}
	// it should be equal to alice's home UUID
	exp, err = getAliceUUID()
	if err != nil {
		return err
	}
	if exp != uuid {
		return errors.Errorf("expect uuid %s, got %s", exp, uuid)
	}
	return nil
}

func (ts *lomodAPISuite) createBobUser() (*client.Lomod, error) {
	// bob always created under default mnt dir
	u := &user.User{Name: bob, Password: bobPwd, HomeDir: defaultMntDir, BackupDir: defaultHomeDir}
	if err := ts.lomoc.AddUser(u); err != nil {
		return nil, err
	}

	if err := ts.validateUUID(); err != nil {
		return nil, err
	}

	// create bobLomoc
	bobLomoc := client.NewLomod("127.0.0.1:" + strconv.Itoa(ts.listenPort))
	_, err := bobLomoc.Login(bob, bobPwd)
	if err != nil {
		return nil, err
	}

	si, err := ts.lomoc.System()
	if err != nil {
		return nil, err
	}
	if len(si.UserStatus) != 2 {
		return nil, errors.Errorf("unable to list correct user status: %+v", *si)
	}
	status, ok := si.UserStatus[bob]
	if !ok {
		return nil, errors.Errorf("unable to find bob in system user status: %+v", *si)
	}
	if status.HomeDisk.Status != "" {
		return nil, errors.Errorf("bob home disk status: %s", status.HomeDisk.Status)
	}
	if status.HomeDisk.FreeSizeInMB == usbMockFreeSizeMB {
		return nil, errors.Errorf("bob home size is wrong: %+v", *si)
	}
	if status.BackupDisk == nil {
		return nil, errors.Errorf("bob backup disk is nil: %+v", *si)
	}
	return bobLomoc, nil
}

func getBobHome() string {
	return filepath.Join(defaultMntDir, bob)
}

func getBobBackup() string {
	return filepath.Join(defaultHomeDir, bob)
}

func getBobUUID() (string, error) {
	return getUUID(common.GetUserConfDir(defaultMntDir, bob))
}

func getBobBackupUUID() (string, error) {
	return getUUID(common.GetUserConfDir(defaultHomeDir, bob))
}
