package udev

import (
	"testing"

	. "gopkg.in/check.v1"
)

type udevSuite struct {
}

var _ = Suite(&udevSuite{})

func TestUdevSuite(t *testing.T) {
	TestingT(t)
}

func TestConnect(t *testing.T) {
	conn, err := NewConn(UdevEvent)
	if err != nil {
		t.Fatal("unable to subscribe to netlink uevent, err:", err)
	}
	defer conn.Close()

	conn2, err := NewConn(UdevEvent)
	if err != nil {
		t.Fatal("unable to subscribe to netlink uevent a second time, err:", err)
	}
	defer conn2.Close()
}
