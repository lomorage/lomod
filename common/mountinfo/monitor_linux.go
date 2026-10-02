package mountinfo

import (
	"context"

	"bitbucket.org/lomoware/lomo-backend/common/udev"
	"github.com/sirupsen/logrus"
)

// MonitorMount monitor udev events and send event
func MonitorMount(ctx context.Context, addCh, removeCh chan Info) error {
	conn, err := udev.NewConn(udev.UdevEvent)
	if err != nil {
		return err
	}
	defer conn.Close()

	done := make(chan error)
	queue := make(chan udev.UEvent)
	go func() {
		done <- conn.Monitor(ctx, queue)
	}()

	for {
		select {
		case err := <-done:
			return err
		case ev := <-queue:
			switch ev.Action {
			case udev.ADD:
				fallthrough
			case udev.REMOVE:
				info := analysisUdevEvent(ev)
				if info == nil {
					logrus.Infof("skip nonrelated event: %+v", ev)
					continue
				}
				if ev.Action == udev.ADD {
					addCh <- *info
				} else {
					removeCh <- *info
				}
			}
		}
	}
}

func analysisUdevEvent(ev udev.UEvent) *Info {
	switch ev.SubSystem {
	case udev.SubsystemBDI:
		// bdi subsystem has nothing to parse
		return &Info{Source: ev.GetDevPath()}
	case udev.SubsystemBlock:
		info := &Info{Source: ev.GetDevName()}
		info.FSType = ev.GetFsType()
		if info.FSType == "" {
			logrus.Infof("skip block device without fstype: %+v", ev)
			return nil
		}
		return info
	}
	return nil
}
