package user

// CreateOSUser check if user exist or not, if not, create user.
func CreateOSUser(username, passwd, home string) error {
	return nil
}

// DeleteOSUser deletes user
func DeleteOSUser(username string) error {
	return nil
}

// CheckAndCreateGroup check if group is exist or not, and create new one if not exist
func CheckAndCreateGroup(name string) error {
	return nil
}

// AddUserIntoGroup adds OS user into OS group
func AddUserIntoGroup(username, groupname string) error {
	return nil
}

// Chown changes owner
func Chown(username, group, home string) error {
	return nil
}

// Chmod changes mod
func Chmod(dir, mod string) error {
	return nil
}
