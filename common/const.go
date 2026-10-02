package common

import "time"

const (
	//LomodHTTPPort is http listen port.
	LomodHTTPPort = 8000
	//LomodHTTPSPort is https listen port.
	LomodHTTPSPort = 8443
	//LomoCloudPort is lomo cloud listen port.
	LomoCloudPort = 8002
	//LomoFramePort is http listen port.
	LomoFramePort = 8003
	//LomodWebdevPort is webdev listen port.
	LomodWebdevPort = 8004
)

const (
	// LomodDbFilename is lomod database filename.
	LomodDbFilename = "assets.db"
	// LomodAdminFilename is lomod admin filename.
	LomodAdminFilename = "admin.json"
)

const (
	// DBTimeOut is for context timeout.
	DBTimeOut = 5 * time.Minute

	// DefaultFilePermission is default file permission, same as os.Create function call.
	DefaultFilePermission = 0644

	// DefaultFolderPermission is default folder permission.
	DefaultFolderPermission = 0750

	// SambaUserFolderPermissionString is default folder permission in string mode.
	SambaUserFolderPermissionString = "0770"

	// BasicAuthPrefix is basic auth prefix.
	BasicAuthPrefix = "Basic "
)

const (
	// DefaultPreviewWidth is default preview width.
	DefaultPreviewWidth = 200
	// SmallPreviewWidth is median preview width.
	SmallPreviewWidth = 75
	// MedianPreviewWidth is median preview width.
	MedianPreviewWidth = 320
	// MaxPreviewWidth is max preview width.
	MaxPreviewWidth = 640
	// DefaultVideoPreviewWidth is default video preview width.
	DefaultVideoPreviewWidth = 480
	// DefaultImagePreviewDims is default image preview dimensions.
	// TODO: remove 320x0 after jpg migration is complete
	DefaultImagePreviewDims = "75x0;320x0;640x0"
	// DefaultVideoPreviewDims is default video preview dimensions.
	DefaultVideoPreviewDims = "480x0"
)

// SystemStatus means the status of the system.
type SystemStatus int

const (
	// SystemStatusAbnormal means the system has something wrong.
	SystemStatusAbnormal SystemStatus = iota - 1
	// SystemStatusNew means the system is not initialized yet.
	SystemStatusNew
	// SystemStatusInited means the system has been initialized.
	SystemStatusInited
	// SystemStatusNoLomod means lomod is not reachable.
	SystemStatusNoLomod
	// SystemStatusLoginFail means login lomod fail.
	SystemStatusLoginFail
	// SystemStatusLoginSuccess means login lomod success.
	SystemStatusLoginSuccess
)

// TaskStatus means the status of task.
type TaskStatus int

const (
	// TaskFail means task fail.
	TaskFail TaskStatus = iota - 1
	// TaskSuccess means task is success.
	TaskSuccess
	// TaskInProgress means task is in progress.
	TaskInProgress
)

// UserStatus meas the status of the user.
type UserStatus int

const (
	// UserStatusUnknown means the user status is unknown.
	UserStatusUnknown UserStatus = iota
	// UserStatusOffline means the user status is offline.
	UserStatusOffline
	// UserStatusOnline means the user status is online.
	UserStatusOnline
)

const (
	// ConfLocalTunnelSubDomain is local tunnel subdomain.
	ConfLocalTunnelSubDomain = "local_tunnel_subdomain"

	// ConfRootCACert is root ca certificate.
	ConfRootCACert = "root_ca_cert"

	// ConfRootCAKey is root ca key.
	ConfRootCAKey = "root_ca_key"

	// ConfPortMapPublicIP is public facing ip of the app.
	ConfPortMapPublicIP = "portmap_public_ip"

	// ConfPortMapPublicPort is public facing port of the app.
	ConfPortMapPublicPort = "portmap_public_port"

	// ConfPortMapLocalPort is local facing port of the app.
	ConfPortMapLocalPort = "portmap_local_port"

	// ConfCloudUsername is username of lomod in lomo cloud.
	ConfCloudUsername = "cloud_username"

	// ConfCloudPassword is password of lomod in lomo cloud.
	ConfCloudPassword = "cloud_password"

	// ConfCloudSubDomain is subdomain of lomod in lomo cloud.
	ConfCloudSubDomain = "cloud_subdomain"

	// ConfCloudToken is token of lomod in lomo cloud.
	ConfCloudToken = "cloud_token"

	// ConfUserPrefix is prefix of user conf
	ConfUserPrefix = "user"

	// ConfWebdavDirLayout is webdev layout setting
	ConfWebdavDirLayout = "webdav_layout"

	// ConfUUID is device uuid
	ConfUUID = "uuid"

	// ConfJWTSecret is this install's random JWT signing key
	ConfJWTSecret = "jwt_secret"

	// ConfCloudIPHelper is to use lomod-cloud as IP discover coordinator
	ConfCloudIPHelper = "cloud_ip_helper"
)

// Query Parameters.
const (
	// QueryDelimiterMetadata is delimiter to separate metadata.
	QueryDelimiterMetadata = ";"
	// QueryDelimiterKV is delimiter to separate key/value in metadata.
	QueryDelimiterKV = ","
	// QueryKeyCreateTime is create time.
	QueryKeyCreateTime = "createtime"
	// QueryKeyModifiedtime is last modified time.
	QueryKeyModifiedtime = "modifiedtime"
	// QueryKeyWidth is to download preview asset's width.
	QueryKeyWidth = "width"
	// QueryKeyHeigh is to download preview asset's height.
	QueryKeyHeight = "height"
	// QueryKeyExt is extension for asset.
	QueryKeyExt = "ext"
	// QueryKeyOrig is to download original asset.
	QueryKeyOrig = "orig"
	// QueryKeyICodec is to download asset with specified image codec.
	QueryKeyICodec = "icodec"
	// QueryKeyHash is to hash of the object. It is SHA1 for now.
	QueryKeyHash = "sha1"
	// QueryKeyFileHash is to hash of the object. It is SHA1 for now.
	QueryKeyFileHash = "filesha"
	// QueryKeyBahash is to lookup asset by hash.
	QueryKeyBahash = "byhash"
	// QueryKeyRetHash is to return asset's hash info.
	QueryKeyRetHash = "hash"
	// QueryKeyInfo is overview asset information.
	QueryKeyInfo = "info"
	// QueryKeyMetadata is metadata.
	QueryKeyMetadata = "meta"
	// QueryKeyMetadataName is metadata name.
	QueryKeyMetadataName = "name"
	// QueryKeyPlainOutput is plain output.
	QueryKeyPlainOutput = "plain"
	// QueryKeyPath is to indicate operate path, such as mount path, scan path
	QueryKeyPath = "path"
	// QueryKeyMove means move assets to new location while import
	QueryKeyMove = "move"
	// QueryKeyCopy means copy assets into the library while import, leaving the originals untouched
	QueryKeyCopy = "copy"
	// QueryKeyScanVideo means scan video while import
	QueryKeyScanVideo = "scan-video"
	// QueryKeyUseExifTime means using exif time as asset create time
	QueryKeyUseExifTime = "exif-time"
	// QueryKeyReboot means reboot the host
	QueryKeyReboot = "reboot"
	// QueryKeyForce means force to do the request
	QueryKeyForce = "force"
	// QueryDuration means duration requested from api client
	QueryKeyDuration = "duration"
	// QueryKeyScanAndImport means scanning given directory and import directory
	QueryKeyScanAndImport = "import"
	// QueryKeyAll means return all data
	QueryKeyAll = "all"
)

const (
	// MdnsLomodDomain is lomod's mdns domain name.
	MdnsLomodDomain = "local."
	// MdnsLomodService is lomod's mdns service name.
	MdnsLomodService = "_lomod._tcp"
	// MdnsChromecastDomain is chromecast mdns domain name.
	MdnsChromecastDomain = "local"
	// MdnsChromecastService is chromcast mdns service name.
	MdnsChromecastService = "_googlecast._tcp"
)

// AlbumAuthorInternal is album user by internal
const AlbumAuthorInternal = "lomod"

var (
	lomoGroupName = "pi"
)

// StartYear is starting year for the system
const StartYear = 1970

// DaysInMonth is number of days in given month
var DaysInMonth = [12]int{31, 29, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
