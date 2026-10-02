package net

import (
	"net"
	"syscall"
	"unsafe"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// NewIPAddrListener creates ip address listener.
func NewIPAddrListener() (*IPAddrListener, error) {
	// skip ipv6 listener
	//groups := (1 << (syscall.RTNLGRP_LINK - 1)) |
	//	(1 << (syscall.RTNLGRP_IPV4_IFADDR - 1)) |
	//	(1 << (syscall.RTNLGRP_IPV6_IFADDR - 1))
	groups := (1 << (syscall.RTNLGRP_LINK - 1)) |
		(1 << (syscall.RTNLGRP_IPV4_IFADDR - 1))

	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_DGRAM, syscall.NETLINK_ROUTE)
	if err != nil {
		return nil, errors.Wrap(err, "while creating socket")
	}

	saddr := &syscall.SockaddrNetlink{
		Family: syscall.AF_NETLINK,
		Pid:    uint32(0),
		Groups: uint32(groups),
	}
	l := &IPAddrListener{fd: fd}

	return l, syscall.Bind(fd, saddr)
}

func (l *IPAddrListener) monitorIPChange(ch chan struct{}) {
	pkt := make([]byte, 2048)
	for {
		msgs, err := l.readMsgs(pkt)
		if err != nil {
			logrus.Warnf("while reading message: %v", err)
		}

		for _, m := range msgs {
			if isNewAddr(&m) || isDelAddr(&m) {
				ch <- struct{}{}
				break
			}
		}
	}
}

func (l *IPAddrListener) readMsgs(pkt []byte) ([]syscall.NetlinkMessage, error) {
	defer func() {
		recover()
	}()

	n, err := syscall.Read(l.fd, pkt)
	if err != nil {
		return nil, errors.Wrap(err, "while reading packet")
	}

	msgs, err := syscall.ParseNetlinkMessage(pkt[:n])
	if err != nil {
		return nil, errors.Wrap(err, "while parsing packet")
	}

	return msgs, nil
}

func isNewAddr(msg *syscall.NetlinkMessage) bool {
	if msg.Header.Type == syscall.RTM_NEWADDR {
		addr := newAddr(msg)
		if addr == nil {
			logrus.Infof("No new addr found although receiving message")
			return false
		}
		logrus.Infof("Got new addr %v", addr)
		return true
	}

	return false
}

func isDelAddr(msg *syscall.NetlinkMessage) bool {
	if msg.Header.Type == syscall.RTM_DELADDR {
		addr := newAddr(msg)
		if addr == nil {
			logrus.Infof("No deleted addr found although receiving message")
			return false
		}
		logrus.Infof("Deleted addr %v", addr)
		return true
	}

	return false
}

func newAddr(msg *syscall.NetlinkMessage) net.Addr {
	ifam := (*syscall.IfAddrmsg)(unsafe.Pointer(&msg.Data[0]))
	attrs, err := syscall.ParseNetlinkRouteAttr(msg)
	if err != nil {
		logrus.Infof("while parsing addr change message, got %v", err)
		return nil
	}

	ipPointToPoint := false
	for _, a := range attrs {
		if a.Attr.Type == syscall.IFA_LOCAL {
			ipPointToPoint = true
			break
		}
	}
	for _, a := range attrs {
		if ipPointToPoint && a.Attr.Type == syscall.IFA_ADDRESS {
			continue
		}
		switch ifam.Family {
		case syscall.AF_INET:
			return &net.IPNet{IP: net.IPv4(a.Value[0], a.Value[1], a.Value[2], a.Value[3]), Mask: net.CIDRMask(int(ifam.Prefixlen), 8*net.IPv4len)}
			// skip ipv6 listener
			//case syscall.AF_INET6:
			//	ifa := &net.IPNet{IP: make(net.IP, net.IPv6len), Mask: net.CIDRMask(int(ifam.Prefixlen), 8*net.IPv6len)}
			//	copy(ifa.IP, a.Value[:])
			//	return ifa
		}
	}
	return nil
}
