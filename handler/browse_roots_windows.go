package handler

import "golang.org/x/sys/windows"

// windowsDriveRoots lists the drive letters Windows reports as present,
// e.g. ["C:\", "E:\"], without touching the drives themselves.
func windowsDriveRoots() []string {
	roots := []string{}
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return roots
	}
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) != 0 {
			roots = append(roots, string(rune('A'+i))+`:\`)
		}
	}
	return roots
}
