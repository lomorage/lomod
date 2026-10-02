package migrator

import "os"

// UserPermission is to migrate user permission
func UserPermission(dbFile string) error {
	return nil
}

func chown(userName, groupName, binFile, home string) error {
	return nil
}

func chmod(username, home string, folderPerm os.FileMode) error {
	return nil
}
