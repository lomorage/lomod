package atlomod

import "bitbucket.org/lomoware/lomo-backend/common/types"

// XcodeInfo is asset's preview info
type XcodeInfo struct {
	Size     int64
	SHA1     string
	WebpSize int64
	WebpSHA1 string
	IsVideo  bool
}

// AssetInfo is test assets information
type AssetInfo struct {
	Path         string
	FileSHA1     *string         `json:",omitempty"`
	ImageFile    *string         `json:",omitempty"`
	ModifiedTime *types.LomoTime `json:",omitempty"`
	// arch - distribution - release - dimension - (size, sha)
	Previews map[string]map[string]map[string]map[string]XcodeInfo `json:",omitempty"`
	ImgXcode map[string]map[string]map[string]XcodeInfo            `json:",omitempty"`
}
