package common

import (
	"os"
	"os/user"
	"strconv"

	"bitbucket.org/lomoware/lomo-backend/common/cmd"
	"github.com/sirupsen/logrus"
)

// GetLomoGroupName returns lomogroup name
func GetLomoGroupName() string {
	g, err := user.LookupGroupId(strconv.Itoa(os.Getgid()))
	if err != nil {
		logrus.Warnf("get lomo group %d: %v", os.Getgid(), err)
		return lomoGroupName
	}
	return g.Name
}

// SetLomoGroupName sets lomo group name
func SetLomoGroupName(g string) {
	lomoGroupName = g
}

// MoveFile move files using system command
func MoveFile(src, dst string) error {
	return cmd.Exec("mv", src, dst)
}
