package types

import (
	"net"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
)

const (
	// MountLocal means disk is local file system
	MountLocal = "LocalFS"
	// MountUSB means disk is usb mount
	MountUSB = "USB"
	// MountNW means disk is network mount
	MountNW = "Network"

	// UUIDFilename means default uuid filename
	UUIDFilename = "disk_uuid"
	// UUIDDelimiter is content of uuid file btw mount type and uuid
	UUIDDelimiter = ":"
)

// BackupCreateRequest is structure to create backup disk for one user
type BackupCreateRequest struct {
	Username string
	DestDisk string
}

// BackupResult is structure for backup result
type BackupResult struct {
	DBRetCode            *string    `json:",omitempty"`
	LastDBBackup         *time.Time `json:",omitempty"`
	LastDBSuccess        *time.Time `json:",omitempty"`
	AssetRetCode         string
	LastAssetSuccess     time.Time
	LastAssetBackupBegin time.Time
	LastAssetBackupEnd   time.Time
}

// UserDisk is user disk information
type UserDisk struct {
	Username string
	FreeSize uint64
	Error    string
}

// CPU is structure for os status
type CPU struct {
	Count int
}

// Disk is structure for os status
type Disk struct {
	Status       string
	ErrorCode    int
	FreeSizeInMB uint64
}

// Network is structure for network
type Network struct {
	Status      string
	PublicAddrs []string
	ListenIPs   []net.IP
}

// TimeZone is structure for timezone
type TimeZone struct {
	Name   string
	Offset int
}

// OSStatus is os status
type OSStatus struct {
	Uptime   string
	CPU      CPU
	Memory   Memory
	Disk     Disk
	Network  Network
	TimeZone TimeZone
}

// AssetSummary is asset Summary
type AssetSummary struct {
	Count int64
	Size  int64
}

// UserStatus is user status
type UserStatus struct {
	AssetSummary map[string]AssetSummary
	HomeDisk     Disk
	BackupDisk   *Disk `json:",omitempty"`
}

// SystemInfo is system information
type SystemInfo struct {
	OS           string
	Arch         string
	APIVersion   string
	LomodVersion string
	UUID         string
	TunnelURL    *string `json:",omitempty"`
	AllowRegistration bool
	SystemStatus common.SystemStatus
	OSStatus     OSStatus
	UserStatus   map[string]UserStatus
	LastBackup   map[string]BackupResult

	// deprecated
	WebpPreview    bool
	PublicAddr     []string
	ListenIPs      []net.IP
	OSDiskFreeSize uint64
	NetworkStatus  string
	DiskStatus     string
	TimezoneName   string
	TimezoneOffset int
	UserDisks      []UserDisk
}

// MountDir is mount directory information
type MountDir struct {
	Type      string
	UUID      string
	Dir       string
	FreeSize  uint64
	TotalSize uint64
	Error     string
}
