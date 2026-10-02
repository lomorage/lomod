package castagent

import (
	"context"
	"encoding/json"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/cast"
	pb "bitbucket.org/lomoware/lomo-backend/common/cast/proto"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	// 'CC1AD845' seems to be a predefined app; check link
	// https://gist.bitbucket.org/jloutsenhizer/8855258
	// https://bitbucket.org/thibauts/node-castv2
	defaultChromecastAppID = "CC1AD845"

	defaultSender = "sender-0"
	defaultRecv   = "receiver-0"

	namespaceConn  = "urn:x-cast:com.google.cast.tp.connection"
	namespaceRecv  = "urn:x-cast:com.google.cast.receiver"
	namespaceMedia = "urn:x-cast:com.google.cast.media"
)

// CastStatus is status of the cast notifitication
type CastStatus int

const (
	// CastFinished means cast finished
	CastFinished CastStatus = iota
	// CastLoadFail means cast load failure
	CastLoadFail
	// CastInterrupted means cast interuppted
	CastInterrupted
	// CastPlaying means cast is playing media
	CastPlaying
	// CastClose means virtual connection is closed
	// https://github.com/thibauts/node-castv2#communicating-with-receivers
	CastClose
)

func (cs CastStatus) String() string {
	switch cs {
	case CastFinished:
		return "finished"
	case CastLoadFail:
		return "load fail"
	case CastInterrupted:
		return "interrupted"
	case CastPlaying:
		return "playing"
	case CastClose:
		return "close"
	default:
		return "unknown"
	}
}

// Notify is the cast status notification
type Notify struct {
	Status CastStatus
}

// CastItem is the item for cast to slide show
type CastItem struct {
	PreloadTime      int
	PlaybackDuration int
	ContentURL       string
	ContentType      string
}

// Cast is agent for chromecast
type Cast struct {
	ctx            context.Context
	recvCtx        context.Context
	recvCancel     context.CancelFunc
	ip             string
	port           int
	resultChanMap  map[int]chan *pb.CastMessage
	recvMsgChan    chan *pb.CastMessage
	conn           *cast.Connection
	requestID      int // Global request id
	mediaSessionID int
	appID          string
	transportID    string
	currItemID     int // current playback position
	nextItemID     int
	notify         chan Notify
}

// New create ones new chromecast agent
func New(ctx context.Context, ip string, port int, n chan Notify) *Cast {
	return &Cast{ctx: ctx, ip: ip, port: port, notify: n,
		resultChanMap: map[int]chan *pb.CastMessage{},
		recvMsgChan:   make(chan *pb.CastMessage, 5)}
}

// GetIP returns the IP of the chromecast
func (c *Cast) GetIP() string {
	return c.ip
}

// GetPort returns the port of the chromecast
func (c *Cast) GetPort() int {
	return c.port
}

// Connect connects to remote ip port
func (c *Cast) Connect() error {
	c.recvCtx, c.recvCancel = context.WithCancel(c.ctx)

	c.conn = cast.NewConnection(c.recvMsgChan)
	c.conn.SetDebug(true)

	if err := c.conn.Start(c.ip, c.port); err != nil {
		return err
	}
	_, err := c.send(&cast.ConnectHeader, defaultSender, defaultRecv, namespaceConn)
	if err != nil {
		return err
	}

	go c.recvMessages()

	return c.update()
}

// Close cloes the connection, and reset context and request ID
func (c *Cast) Close() error {
	if c.conn == nil {
		return nil
	}
	c.recvCancel()
	c.requestID = 0
	return c.conn.Close()
}

func (c *Cast) send(payload cast.Payload, sourceID, destinationID, namespace string) (int, error) {
	// NOTE: Not concurrent safe, but currently only synchronous flow is possible
	// TODO(lomoware): just make concurrent safe regardless of current flow
	c.requestID++
	payload.SetRequestId(c.requestID)
	return c.requestID, c.conn.Send(c.requestID, payload, sourceID, destinationID, namespace)
}

func (c *Cast) sendAndWait(payload cast.Payload, sourceID, destinationID, namespace string) (*pb.CastMessage, error) {
	requestID, err := c.send(payload, sourceID, destinationID, namespace)
	if err != nil {
		return nil, err
	}

	// Set a timeout to wait for the response
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	// TODO(lomoware): not concurrent safe. Not a problem at the moment
	// because only synchronous flow currently allowed.
	resultChan := make(chan *pb.CastMessage, 1)
	c.resultChanMap[requestID] = resultChan
	defer func() {
		delete(c.resultChanMap, requestID)
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultChan:
		return result, nil
	}
}

func (c *Cast) recvMessages() {
	for {
		select {
		case <-c.recvCtx.Done():
			if c.recvCtx.Err() != nil {
				logrus.Warnf("chromecast %s:%d disconnected: %s", c.ip, c.port, c.recvCtx.Err())
			} else {
				logrus.Infof("chromecast %s:%d disconnected", c.ip, c.port)
			}
			return
		case msg := <-c.recvMsgChan:
			header := cast.PayloadHeader{}
			messageBytes := []byte(*msg.PayloadUtf8)
			err := json.Unmarshal(messageBytes, &header)
			if err == nil {
				if resultChan, ok := c.resultChanMap[header.RequestId]; ok {
					resultChan <- msg
					continue
				} else {
					logrus.Warnf("Receive unwaited %d message: %s", header.RequestId, *msg.PayloadUtf8)
				}
			} else {
				logrus.Errorf("chromecast %s:%d parse message: %s, got: %v", c.ip, c.port, *msg.PayloadUtf8, err)
			}

			// This already gets checked in the cast.Connection.handleMessage function.
			switch header.Type {
			case "CLOSE":
				// the connection should be closed, and external app should re-connect
				c.notify <- Notify{Status: CastLoadFail}
			case "LOAD_FAILED":
				c.notify <- Notify{Status: CastLoadFail}
			case "MEDIA_STATUS":
				resp := cast.MediaStatusResponse{}
				if err := json.Unmarshal(messageBytes, &resp); err == nil {
					for _, status := range resp.Status {
						// The LoadingItemId is only set when there is a playlist and there
						// is an item being loaded to play next.
						if status.IdleReason == "FINISHED" && status.LoadingItemId == 0 {
							c.notify <- Notify{Status: CastFinished}
						} else if status.IdleReason == "INTERRUPTED" && status.Media.ContentId == "" {
							// This can happen
							// 1. when we go "next" in a playlist when it is playing the last track.
							// 2. lomod restarts
							c.notify <- Notify{Status: CastInterrupted}
						} else {
							c.mediaSessionID = status.MediaSessionId
							c.currItemID = status.CurrentItemId
							for i, item := range status.Items {
								if c.currItemID == item.ItemID && i < len(status.Items)-1 {
									c.nextItemID = status.Items[i+1].ItemID
								}
							}
						}
					}
				}
			case "RECEIVER_STATUS":
				resp := cast.ReceiverStatusResponse{}
				if err := json.Unmarshal(messageBytes, &resp); err != nil {
					logrus.Warnf("receiver status unmarshal got: %v", err)
				} else {
					logrus.Warnf("got receiver status : %v", resp)
				}
			}
		}
	}
}

func (c *Cast) getReceiverStatus() (*cast.ReceiverStatusResponse, error) {
	apiMessage, err := c.sendAndWait(&cast.GetStatusHeader, defaultSender, defaultRecv, namespaceRecv)
	if err != nil {
		return nil, err
	}
	var response cast.ReceiverStatusResponse
	if err := json.Unmarshal([]byte(*apiMessage.PayloadUtf8), &response); err != nil {
		return nil, errors.Wrap(err, "error unmarshaling json")
	}
	return &response, nil

}

func (c *Cast) getMediaStatus() (*cast.MediaStatusResponse, error) {
	apiMessage, err := c.sendAndWait(&cast.GetStatusHeader, defaultSender, c.transportID, namespaceMedia)
	if err != nil {
		return nil, err
	}
	var response cast.MediaStatusResponse
	if err := json.Unmarshal([]byte(*apiMessage.PayloadUtf8), &response); err != nil {
		return nil, errors.Wrap(err, "error unmarshaling json")
	}
	return &response, nil
}

func (c *Cast) updateMediaStatus() error {
	c.send(&cast.ConnectHeader, defaultSender, c.transportID, namespaceConn)

	mediaStatus, err := c.getMediaStatus()
	if err != nil {
		return err
	}
	for _, m := range mediaStatus.Status {
		c.mediaSessionID = m.MediaSessionId
	}

	return nil
}

func (c *Cast) update() error {
	var recvStatus *cast.ReceiverStatusResponse
	count := 10
	for i := 0; i < count; i++ {
		var err error
		recvStatus, err = c.getReceiverStatus()
		if err == nil {
			// need retry until get at least one application if reply is as below
			/*
				{
					"requestId": 2,
					"status": {
						"userEq": {},
						"volume": {
							"controlType": "attenuation",
							"level": 1,
							"muted": false,
							"stepInterval": 0.05000000074505806
						}
					},
					"type": "RECEIVER_STATUS"
				}
			*/
			if len(recvStatus.Status.Applications) >= 1 {
				break
			} else {
				logrus.Warnf("device return 0 applications; attempt %d/5, retrying...", i+1)
			}
		} else {
			logrus.Warnf("error getting receiever status: %v", err)
			logrus.Warnf("unable to get status from device; attempt %d/5, retrying...", i+1)
		}
		if i == count-1 {
			return common.ErrRetryFailure
		}
		time.Sleep(time.Second * 2)
	}

	if len(recvStatus.Status.Applications) > 1 {
		logrus.Warnf("more than 1 connected application on the chromecast: (%d)%#v",
			len(recvStatus.Status.Applications), recvStatus.Status.Applications)
	}

	needUpdateMediaStatus := true
	for _, app := range recvStatus.Status.Applications {
		c.appID = app.AppId
		c.transportID = app.TransportId

		if app.IsIdleScreen {
			needUpdateMediaStatus = false
		}
	}

	if !needUpdateMediaStatus {
		return nil
	}

	return c.updateMediaStatus()
}

func (c *Cast) next() error {
	c.notify <- Notify{Status: CastPlaying}

	_, err := c.send(&cast.QueueUpdate{
		PayloadHeader:  cast.QueueUpdateHeader,
		MediaSessionId: c.mediaSessionID,
		Jump:           1,
	}, defaultSender, c.transportID, namespaceMedia)
	return err
}

func (c *Cast) ensureIsDefaultMediaReceiver() error {
	if c.appID != defaultChromecastAppID {
		_, err := c.sendAndWait(&cast.LaunchRequest{PayloadHeader: cast.LaunchHeader, AppId: defaultChromecastAppID}, defaultSender, defaultRecv, namespaceRecv)

		if err != nil {
			return errors.Wrap(err, "unable to change to default media receiver")
		}
		// Update the 'application' and 'media' field on the 'CastApplication'
		return c.update()
	}

	return nil
}

func (c *Cast) start(items []cast.QueueLoadItem) error {
	if err := c.ensureIsDefaultMediaReceiver(); err != nil {
		return err
	}

	// Send the command to the chromecast
	_, err := c.send(&cast.QueueLoad{PayloadHeader: cast.QueueLoadHeader, CurrentTime: 0, StartIndex: 0, RepeatMode: "REPEAT_ALL", Items: items},
		defaultSender, c.transportID, namespaceMedia)
	return err
}

// Cast cast one single item to chromecast
func (c *Cast) Cast(f CastItem) error {
	items := []cast.QueueLoadItem{{
		PreloadTime:      f.PreloadTime,
		Autoplay:         true,
		PlaybackDuration: f.PlaybackDuration,
		Media: cast.MediaItem{
			Autoplay:    true,
			ContentId:   f.ContentURL,
			StreamType:  "BUFFERED",
			ContentType: f.ContentType,
		}}}

	return c.start(items)
}

// SlideShow starts showing the content belong to me
func (c *Cast) SlideShow(files []CastItem, newItems chan CastItem) error {
	started := false
	if len(files) != 0 {
		items := make([]cast.QueueLoadItem, len(files))
		for i, f := range files {
			items[i] = cast.QueueLoadItem{
				PreloadTime:      f.PreloadTime,
				Autoplay:         true,
				PlaybackDuration: f.PlaybackDuration,
				Media: cast.MediaItem{
					Autoplay:    true,
					ContentId:   f.ContentURL,
					StreamType:  "BUFFERED",
					ContentType: f.ContentType,
				}}
		}

		if err := c.start(items); err != nil {
			return err
		}
		started = true
	}
	// Timer for when to call the next image
	newItemQueue := []cast.QueueLoadItem{}
	t := time.NewTicker(time.Second * 20)
	for {
		select {
		case <-c.ctx.Done():
			return c.ctx.Err()
		case newItem := <-newItems:
			newItemQueue = append(newItemQueue, cast.QueueLoadItem{
				PreloadTime:      newItem.PreloadTime,
				Autoplay:         true,
				PlaybackDuration: newItem.PlaybackDuration,
				Media: cast.MediaItem{
					Autoplay:    true,
					ContentId:   newItem.ContentURL,
					StreamType:  "BUFFERED",
					ContentType: newItem.ContentType,
				}})
		case <-t.C:
			if len(newItemQueue) == 0 {
				if started {
					if err := c.next(); err != nil {
						return err
					}
				}
				continue
			}
			if !started {
				if err := c.start(newItemQueue); err != nil {
					return err
				}
				started = true
				newItemQueue = []cast.QueueLoadItem{}
				continue
			}
			insert := cast.QueueInsert{
				PayloadHeader:  cast.QueueInsertHeader,
				MediaSessionId: c.mediaSessionID,
				InsertBefore:   c.nextItemID,
				Items:          newItemQueue,
			}
			if _, err := c.sendAndWait(&insert, defaultSender, c.transportID, namespaceMedia); err != nil {
				return err
			}
			if err := c.next(); err != nil {
				return err
			}
			newItemQueue = []cast.QueueLoadItem{}
		}
	}
}
