package types

// ShareType is the shared type, either share to user or to group
type ShareType int

const (
	// ShareToUser means the asset is shared to a user
	ShareToUser ShareType = iota
	// ShareToGroup means the asset is shared to a group
	ShareToGroup
	// CastToUser means the asset is cast to a chromecast device
	CastToUser
)

// Record is the record of one share
type Record struct {
	ID          uint64
	Type        ShareType
	SenderID    int
	ReceiverID  int
	AssetID     string
	MIME        string
	Codec       string
	AssetIDType *AssetIDType `json:"AssetIDType,omitempty"`
	ShareTime   LomoTime
	ReadFlag    bool
}

// ByRecordsShareTime implements sort.Interface
type ByRecordsShareTime []*Record

func (a ByRecordsShareTime) Len() int           { return len(a) }
func (a ByRecordsShareTime) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByRecordsShareTime) Less(i, j int) bool { return a[i].ShareTime.After(a[j].ShareTime.Time) }

// Records is list of share records
type Records struct {
	Records []*Record
}

// ReceiveRecords is list of all parties has shared assets to one user
type ReceiveRecords struct {
	Users  []int
	Groups []int
}
