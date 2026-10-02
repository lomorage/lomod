package net

import (
	snet "net"
)

func isWirelessInterface(addr snet.Addr) (bool, error) {
	return false, nil
}
