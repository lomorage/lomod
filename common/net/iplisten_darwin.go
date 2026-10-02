package net

import (
	"syscall"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/route"
)

// NewIPAddrListener creates ip address listener
func NewIPAddrListener() (*IPAddrListener, error) {
	fd, err := syscall.Socket(syscall.AF_ROUTE, syscall.SOCK_RAW, syscall.AF_UNSPEC)
	if err != nil {
		return nil, errors.Wrap(err, "while creating socket")
	}
	return &IPAddrListener{fd: fd}, nil
}

func (l *IPAddrListener) monitorIPChange(ch chan struct{}) {
	pkt := make([]byte, 2048)
	for {
		msgs, err := l.readMsgs(pkt)
		if err != nil {
			logrus.Warnf("while reading message: %v", err)
		}

		for _, msg := range msgs {
			if _, ok := msg.(*route.RouteMessage); ok {
				ch <- struct{}{}
				break
			}
		}
	}
}

func (l *IPAddrListener) readMsgs(pkt []byte) ([]route.Message, error) {
	defer func() {
		recover()
	}()

	n, err := syscall.Read(l.fd, pkt)
	if err != nil {
		return nil, errors.Wrap(err, "while reading packet")
	}

	msgs, err := route.ParseRIB(route.RIBTypeRoute, pkt[:n])
	if err != nil {
		return nil, errors.Wrap(err, "while parsing packet")
	}

	return msgs, nil
}
