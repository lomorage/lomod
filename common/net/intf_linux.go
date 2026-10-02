package net

import (
	snet "net"
	"os"
)

func isWirelessInterface(addr snet.Addr) (bool, error) {
	intfs, err := snet.Interfaces()
	if err != nil {
		return false, err
	}

	for _, intf := range intfs {
		addrs, err := intf.Addrs()
		if err != nil {
			return false, err
		}
		for _, a := range addrs {
			if a.String() != addr.String() {
				continue
			}
			_, err = os.Stat("/sys/class/net/" + intf.Name + "/wireless")
			return err == nil, nil
		}
	}
	return false, nil
}
