package user

import (
	"bytes"
	"io/ioutil"
	"os"
	"sort"
	"strings"

	"bitbucket.org/lomoware/lomo-backend/common/cmd"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	smbConf = "/etc/samba/smb.conf"
)

// ReloadSamba reload samba config
// Note: before calling this API, caller should be responsible to create linux user
func ReloadSamba(gconf string, uc SambaUsersConf) error {
	// sort user conf by user name
	sort.Sort(uc)

	newConf, err := generateConfig(gconf, uc)
	if err != nil {
		return err
	}
	oldConf, err := ioutil.ReadFile(smbConf)
	if err != nil {
		return err
	}
	if newConf == string(oldConf) {
		return nil
	}

	// write to one temp file, then sudo move
	tmp, err := ioutil.TempFile("", "samba")
	if err != nil {
		return err
	}
	defer func() {
		// close again but skip checking error
		tmp.Close()
		if err := os.Remove(tmp.Name()); err != nil {
			logrus.Warnf("while removing tmp samba config file: %s", err)
		}
	}()

	if _, err := tmp.Write([]byte(newConf)); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	if err := cmd.ExecWithSudo("mv", tmp.Name(), smbConf); err != nil {
		return err
	}

	// reload samba configuration
	return cmd.ExecWithSudo("smbcontrol", "all", "reload-config")
}

// CreateSambaUser adds samba user
func CreateSambaUser(username, password string) error {
	// delete in case it exist before
	if err := cmd.ExecWithSudo("smbpasswd", "-x", username); err != nil {
		if !strings.Contains(err.Error(), string("Failed to find entry for user")) {
			logrus.Warnf(err.Error())
		}
	}

	buf := bytes.NewBuffer([]byte(password + "\n" + password + "\n"))
	out, err := cmd.RunWithStdinSudo(buf, "smbpasswd", "-a", "-s", username)
	if err != nil {
		return err
	}
	if strings.HasPrefix(string(out), "Failed") {
		return errors.Errorf("add samba %s fail: %s", username, out)
	}

	out, err = cmd.RunWithSudo("smbpasswd", "-e", username)
	if err != nil {
		return err
	}
	if strings.HasPrefix(string(out), "Failed") {
		return errors.Errorf("enable samba %s fail: %s", username, out)
	}
	return nil
}

// DeleteSambaUser deletes samba user
func DeleteSambaUser(username string) error {
	return cmd.ExecWithSudo("smbpasswd", "-x", username)
}
