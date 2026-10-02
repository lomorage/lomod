package net

import (
	"io/ioutil"
	"net"
	snet "net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// IPAddrListener listens ip address change message.
type IPAddrListener struct {
	fd int
}

// MonitorIPChange monitor ip changes, and send signal.
func (l *IPAddrListener) MonitorIPChange(ch chan struct{}) {
	// buffer messages 5 seconds if receiving too many concurrent change event
	newCh := make(chan struct{})
	go l.monitorIPChange(newCh)

	lock := sync.Mutex{}
	received := false
	for range newCh {
		lock.Lock()
		received = true
		lock.Unlock()

		go func() {
			time.Sleep(5 * time.Second)
			lock.Lock()
			if received {
				ch <- struct{}{}
				received = false
			}
			lock.Unlock()
		}()
	}
}

func getDefaultIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil
	}
	defer conn.Close()
	ipnet, ok := conn.LocalAddr().(*net.UDPAddr)
	if ok {
		return ipnet.IP
	}

	localAddr := conn.LocalAddr().String()
	idx := strings.LastIndex(localAddr, ":")
	return net.ParseIP(localAddr[0:idx])
}

// ListIPs list available ipv4 addresses.
func ListIPs() ([]snet.IP, error) {
	addrs, err := snet.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	ret := []snet.IP{}
	var dn16, dn24 *snet.IPNet
	defaultIP := getDefaultIP()
	if defaultIP != nil {
		_, dn16, err = net.ParseCIDR(defaultIP.String() + "/16")
		if err != nil {
			return nil, err
		}
		_, dn24, err = net.ParseCIDR(defaultIP.String() + "/24")
		if err != nil {
			return nil, err
		}
		ret = append(ret, defaultIP)
	}

	retOthers := []snet.IP{}
	ret16 := []snet.IP{}
	ret24 := []snet.IP{}
	for _, a := range addrs {
		if ipnet, ok := a.(*snet.IPNet); ok && ipnet.IP.IsGlobalUnicast() {
			ip := ipnet.IP.To4()
			if ip == nil || isKnownAddr(ip.String()) {
				continue
			}
			if defaultIP != nil && ip.String() == defaultIP.String() {
				continue
			}
			yes, err := isWirelessInterface(a)
			if err != nil {
				logrus.Warnf("check wireless interface %s got %v", a, err)
				continue
			}
			if yes {
				if dn24 != nil && dn24.Contains(ip) {
					ret24 = append(ret24, ip)
				} else if dn16 != nil && dn16.Contains(ip) {
					ret16 = append(ret16, ip)
				} else {
					retOthers = append(retOthers, ip)
				}
			} else {
				// put to the front
				if dn24 != nil && dn24.Contains(ip) {
					ret24 = append([]snet.IP{ip}, ret24...)
				} else if dn16 != nil && dn16.Contains(ip) {
					ret16 = append([]snet.IP{ip}, ret16...)
				} else {
					retOthers = append([]snet.IP{ip}, retOthers...)
				}
			}
		}
	}
	if len(ret24) != 0 {
		ret = append(ret, ret24...)
	}
	if len(ret16) != 0 {
		ret = append(ret, ret16...)
	}
	if len(retOthers) != 0 {
		ret = append(ret, retOthers...)
	}
	return ret, nil
}

// ListIPsAttrs wil return IP with its attributes.
func ListIPsAttrs() (map[string][]snet.IP, error) {
	ipAddrWireless := "wireless"
	ipAddrWired := "wired"
	addrs, err := snet.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	ret := map[string][]snet.IP{}
	for _, a := range addrs {
		if ipnet, ok := a.(*snet.IPNet); ok && ipnet.IP.IsGlobalUnicast() {
			if ipnet.IP.To4() == nil || isKnownAddr(ipnet.IP.To4().String()) {
				continue
			}
			yes, err := isWirelessInterface(a)
			if err != nil {
				logrus.Warnf("check wireless interface %s got %v", a, err)
				continue
			}
			key := ipAddrWired
			if yes {
				key = ipAddrWireless
			}
			ips, ok := ret[key]
			if !ok {
				ret[key] = []snet.IP{ipnet.IP}
			} else {
				ret[key] = append(ips, ipnet.IP)
			}
		}
	}
	return ret, nil
}

func isKnownAddr(addr string) bool {
	switch addr {
	case "172.17.0.1":
		return true
	}
	return false
}

// GetPublicIP gets my public internet ip.
func GetPublicIP() (string, error) {
	url := "https://api.ipify.org?format=text" // we are using a pulib IP API, we're using ipify here, below are some others
	// https://www.ipify.org
	// http://myexternalip.com
	// http://api.ident.me
	// http://whatismyipaddress.com/api
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	ip, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(ip), nil
}

// IsSameSubnet uses /16 to check if source ip is in the same subnet of list of ips
func IsSameSubnet(targetIP snet.IP, ips []snet.IP) bool {
	_, ipnet, err := net.ParseCIDR(targetIP.String() + "/16")
	if err != nil {
		logrus.Warnf("during check same subnet, got invalid IP %s:%v", targetIP, err)
		return false
	}

	for _, ip := range ips {
		if ipnet.Contains(ip) {
			return true
		}
	}
	return false
}
