package common

import (
	"encoding/json"
	"io/ioutil"
	"os"

	"github.com/sirupsen/logrus"
)

type adminConf struct {
	AdminToken string `json:"admin-token"`
}

// LoadAndSaveAdminToken is to load and save admin token
func LoadAndSaveAdminToken(filename, adminToken string, filePerm os.FileMode) (string, error) {
	oldToken := ""
	ac := &adminConf{}
	_, err := os.Stat(filename)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}
	} else {
		data, err := ioutil.ReadFile(filename)
		if err != nil {
			return "", err
		}
		if err := json.Unmarshal(data, ac); err != nil {
			logrus.Warnf("%s has invalid data: %s", filename, string(data))
		} else {
			oldToken = ac.AdminToken
		}
	}
	if oldToken != "" {
		if oldToken == adminToken {
			// same token
			return adminToken, nil
		} else if adminToken == "" {
			// re-use old token
			return oldToken, nil
		}
	} else if adminToken == "" {
		adminToken = RandomString(12)
	}

	ac.AdminToken = adminToken
	data, err := json.Marshal(ac)
	if err != nil {
		logrus.Warnf("Can not marshall %v, and unable to save", *ac)
		return adminToken, nil
	}
	return adminToken, ioutil.WriteFile(filename, data, filePerm)
}
