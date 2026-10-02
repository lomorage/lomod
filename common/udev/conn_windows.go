package udev

import "context"

// UEventConn is connection stub
type UEventConn struct{}

// NewConn allow to connect to system socket AF_NETLINK with family NETLINK_KOBJECT_UEVENT to
// catch events about block/char device
func NewConn(mode Mode) (*UEventConn, error) {
	return &UEventConn{}, nil
}

// Monitor run in background a worker to read netlink msg in loop and notify
// when msg receive inside a queue using channel.
// To be notified with only relevant message, use Matcher.
func (c *UEventConn) Monitor(ctx context.Context, queue chan UEvent) error {
	<-ctx.Done()
	return ctx.Err()
}

// Close allow to close file descriptor and socket bound
func (c *UEventConn) Close() error {
	return nil
}
