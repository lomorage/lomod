package cmd

func DeleteFile(file string) error {
	return Exec("del", file)
}

func Chown(owner string, files ...string) error {
	return nil
}

func ChownDir(owner, dir string) error {
	return nil
}

func Link(src, target string) error {
	return nil
}

func Poweroff(reboot bool) error {
	return nil
}
