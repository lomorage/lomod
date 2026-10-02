package client

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/types"
)

// System call system api and return its status
func (ld *Lomod) System() (*types.SystemInfo, error) {
	resp, err := ld.requestReply(http.MethodGet, "/system", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusInternalServerError {
		return nil, makeStatusError(resp.Body)
	}
	si := &types.SystemInfo{}
	err = json.NewDecoder(resp.Body).Decode(si)
	return si, err
}

// IsMaintenanceMode checks if the system is in maintenance mode
func (ld *Lomod) IsMaintenanceMode() (bool, error) {
	_, err := ld.System()
	if err == nil {
		return false, nil
	} else if err.Error() == common.ErrMaintenance.Error() {
		return true, nil
	}
	return false, err
}

// StartCCheck starts consistency check
func (ld *Lomod) StartCCheck() error {
	return ld.request(http.MethodPost, "/system/ccheck", nil, nil)
}

// GetCCheckResult get consistency check result
func (ld *Lomod) GetCCheckResult() (string, error) {
	resp, err := ld.requestReply(http.MethodGet, "/system/ccheck", nil)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", makeStatusError(resp.Body)
	}
	defer resp.Body.Close()

	content, err := ioutil.ReadAll(resp.Body)
	return string(content), err
}

// SetBackupDisk set backup disk for one user
func (ld *Lomod) SetBackupDisk(username, disk string) error {
	return ld.request(http.MethodPost, "/system/backup", nil, &types.BackupCreateRequest{
		Username: username, DestDisk: disk})
}

// StartBackup starts backup
func (ld *Lomod) StartBackup() error {
	return ld.request(http.MethodPut, "/system/backup", nil, nil)
}

// ListMounts return current mount directories
func (ld *Lomod) ListMounts() ([]types.MountDir, error) {
	resp, err := ld.requestReply(http.MethodGet, "/system/mount", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusInternalServerError {
		return nil, makeStatusError(resp.Body)
	}
	md := []types.MountDir{}
	err = json.NewDecoder(resp.Body).Decode(&md)
	return md, err
}

// Unmount unmount one given path
func (ld *Lomod) Unmount(p string) error {
	return ld.request(http.MethodDelete, "/system/mount", map[string]string{
		common.QueryKeyPath: url.QueryEscape(p)}, nil)
}
