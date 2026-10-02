package cast

// Known Payload headers
var (
	// ConnectHeader is connect header
	ConnectHeader = PayloadHeader{Type: "CONNECT"}
	// CloseHeader is close header
	CloseHeader = PayloadHeader{Type: "CLOSE"}
	// GetStatusHeader get status
	GetStatusHeader = PayloadHeader{Type: "GET_STATUS"}
	// PongHeader is header to response to PING payload
	PongHeader = PayloadHeader{Type: "PONG"}
	// LaunchHeader launches a new chromecast app
	LaunchHeader = PayloadHeader{Type: "LAUNCH"}
	// StopHeader stop playing current media
	StopHeader = PayloadHeader{Type: "STOP"}
	// PlayHeader plays / unpauses the running app
	PlayHeader = PayloadHeader{Type: "PLAY"}
	// PauseHeader pauses the running app
	PauseHeader = PayloadHeader{Type: "PAUSE"}
	// SeekHeader seek into the running app
	SeekHeader = PayloadHeader{Type: "SEEK"}
	// VolumeHeader sets the volume
	VolumeHeader = PayloadHeader{Type: "SET_VOLUME"}
	// LoadHeader loads an application onto the chromecast
	LoadHeader = PayloadHeader{Type: "LOAD"}
	// QueueLoadHeader loads an application onto the chromecast
	QueueLoadHeader = PayloadHeader{Type: "QUEUE_LOAD"}
	// QueueInsertHeader loads an application onto the chromecast
	QueueInsertHeader = PayloadHeader{Type: "QUEUE_INSERT"}
	// QueueUpdateHeader loads an application onto the chromecast
	QueueUpdateHeader = PayloadHeader{Type: "QUEUE_UPDATE"}
)

// Payload is interface for chromecast payload
type Payload interface {
	SetRequestId(id int)
}

// PayloadHeader is header for payload
type PayloadHeader struct {
	Type      string `json:"type"`
	RequestId int    `json:"requestId,omitempty"`
}

// SetRequestId sets request id
func (p *PayloadHeader) SetRequestId(id int) {
	p.RequestId = id
}

// QueueUpdate update queue
type QueueUpdate struct {
	PayloadHeader
	MediaSessionId int `json:"mediaSessionId,omitempty"`
	Jump           int `json:"jump,omitempty"`
}

// QueueInsert insert queue items
type QueueInsert struct {
	PayloadHeader
	MediaSessionId int             `json:"mediaSessionId,omitempty"`
	InsertBefore   int             `json:"insertBefore,omitempty"`
	Items          []QueueLoadItem `json:"items"`
}

type QueueLoad struct {
	PayloadHeader
	MediaSessionId int             `json:"mediaSessionId,omitempty"`
	CurrentTime    float32         `json:"currentTime"`
	StartIndex     int             `json:"startIndex"`
	RepeatMode     string          `json:"repeatMode"`
	Items          []QueueLoadItem `json:"items"`
}

type QueueLoadItem struct {
	ItemID           int       `json:"itemID"`
	PreloadTime      int       `json:"preloadTime"`
	Media            MediaItem `json:"media"`
	Autoplay         bool      `json:"autoplay"`
	PlaybackDuration int       `json:"playbackDuration"`
}

type MediaHeader struct {
	PayloadHeader
	MediaSessionId int     `json:"mediaSessionId"`
	CurrentTime    float32 `json:"currentTime"`
	RelativeTime   float32 `json:"relativeTime,omitempty"`
	ResumeState    string  `json:"resumeState"`
}

type Volume struct {
	Level float32 `json:"level,omitempty"`
	Muted bool    `json:"muted"`
}

type ReceiverStatusResponse struct {
	PayloadHeader
	Status struct {
		Applications []Application `json:"applications"`
		Volume       Volume        `json:"volume"`
	} `json:"status"`
}

type Application struct {
	AppId        string `json:"appId"`
	DisplayName  string `json:"displayName"`
	IsIdleScreen bool   `json:"isIdleScreen"`
	SessionId    string `json:"sessionId"`
	StatusText   string `json:"statusText"`
	TransportId  string `json:"transportId"`
}

type ReceiverStatusRequest struct {
	PayloadHeader
	Applications []Application `json:"applications"`

	Volume Volume `json:"volume"`
}

type LaunchRequest struct {
	PayloadHeader
	AppId string `json:"appId"`
}

type LoadMediaCommand struct {
	PayloadHeader
	Media       MediaItem   `json:"media"`
	CurrentTime int         `json:"currentTime"`
	Autoplay    bool        `json:"autoplay"`
	QueueData   QueueData   `json:"queueData"`
	CustomData  interface{} `json:"customData"`
}

type QueueData struct {
	StartIndex int `json:"startIndex"`
}

type MediaItem struct {
	ContentId   string        `json:"contentId"`
	ContentType string        `json:"contentType"`
	StreamType  string        `json:"streamType"`
	Autoplay    bool          `json:"autoplay"`
	Duration    float32       `json:"duration"`
	Metadata    MediaMetadata `json:"metadata"`
}

type MediaMetadata struct {
	MetadataType int     `json:"metadataType`
	Artist       string  `json:"artist"`
	Title        string  `json:"title"`
	Subtitle     string  `json:"subtitle"`
	Images       []Image `json:"images"`
	ReleaseDate  string  `json:"releaseDate"`
}

type Image struct {
	URL    string `json:"url"`
	Height int    `json:"height"`
	Width  int    `json:"width"`
}

type Media struct {
	MediaSessionId int     `json:"mediaSessionId"`
	PlayerState    string  `json:"playerState"`
	CurrentTime    float32 `json:"currentTime"`
	IdleReason     string  `json:"idleReason"`
	Volume         Volume  `json:"volume"`
	CurrentItemId  int     `json:"currentItemId"`
	LoadingItemId  int     `json:"loadingItemId"`

	Media MediaItem `json:"media"`

	Items []QueueLoadItem `json:"items"`
}

type MediaStatusResponse struct {
	PayloadHeader
	Status []Media `json:"status"`
}

type SetVolume struct {
	PayloadHeader
	Volume Volume `json:"volume"`
}
