package handler

import (
	"context"
	"fmt"
	"net"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/client"
	"bitbucket.org/lomoware/lomo-backend/common"
	lnet "bitbucket.org/lomoware/lomo-backend/common/net"
	"bitbucket.org/lomoware/lomo-backend/common/release"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"github.com/leslie-wang/zeroconf"
	"github.com/miekg/dns"
	"github.com/sirupsen/logrus"
)

const (
	delimit       = "-"
	resolvTimeout = 10 * time.Second
)

func calculateInstanceName(ctx context.Context, name, service, domain string) string {
	entries, err := lnet.DiscoverMDNSServices(ctx, service, domain, resolvTimeout)
	if err != nil {
		// use input name if discover failure
		logrus.Errorf("while discover other MDNS services: %v", err)
		return name
	}
	idxs := []int{}
	prefix := name + delimit
	for _, entry := range entries {
		instance := entry.ServiceRecord.Instance
		if !strings.HasPrefix(instance, name) {
			continue
		}
		idx := 0
		if strings.HasPrefix(instance, prefix) {
			idx, err = strconv.Atoi(strings.TrimPrefix(instance, prefix))
			if err != nil {
				logrus.Warnf("resolved non-compliant instance name: %s", instance)
				continue
			}
		} else if instance != name {
			logrus.Infof("resolved similar instance name: %s", instance)
			continue
		}
		idxs = append(idxs, idx)
	}

	if len(idxs) == 0 {
		return name
	}

	sort.Ints(idxs)

	newIdx := len(idxs)
	for i, idx := range idxs {
		if i != idx {
			newIdx = i
			break
		}
	}
	return name + delimit + strconv.Itoa(newIdx)
}

func startMDNS(ctx context.Context, ips []net.IP, port int, name, service, domain string,
	texts []string, cb zeroconf.Callback) (*zeroconf.Server, error) {
	// recreate entry every time to avoid unlimited append domain name
	entry := zeroconf.NewServiceEntry(name, service, domain)
	entry.HostName = name
	entry.Port = port
	entry.Text = texts
	entry.CacheFlush = true
	entry.Callback = cb

	entry.AddrIPv4 = ips
	server, err := zeroconf.RegisterServiceEntry(entry, nil)
	if err != nil {
		return nil, err
	}
	logrus.Infof("Published service: %s, type: %s, domain: %s", name, service, domain)
	return server, nil
}

func (h *Handler) processMDNS(ctx context.Context, port int, name, service, domain string, publish bool) {
	ch := make(chan struct{})
	l, err := lnet.NewIPAddrListener()
	if err != nil {
		logrus.Errorf("while creating ip addr listener, got %v", err)
		return
	}
	go l.MonitorIPChange(ch)

	registeredName := calculateInstanceName(ctx, name, service, domain)
	newIPs := h.listenIPs
	currIPs := map[string]struct{}{}
	for {
		if len(newIPs) == 0 {
			newIPs, err = lnet.ListIPs()
			if err != nil {
				logrus.Errorf("while list ips: %v", err)
				// sleep and retry
				newIPs = nil
				time.Sleep(5 * time.Second)
				continue
			}
		}
		h.listenIPs = newIPs
		for _, ip := range h.listenIPs {
			currIPs[ip.String()] = struct{}{}
		}
		var (
			server     *zeroconf.Server
			mdnsCtx    context.Context
			mdnsCancel context.CancelFunc
		)
		if publish {
			mdnsCtx, mdnsCancel = context.WithCancel(ctx)
			defer mdnsCancel()
			server, err = startMDNS(mdnsCtx, h.listenIPs, port, registeredName, service, domain,
				[]string{"os=" + runtime.GOOS, "uuid=" + h.uuid}, h.mdnsLog)
			if err != nil {
				logrus.Errorf("while registering mdns service: %v", err)

				// sleep and retry
				time.Sleep(30 * time.Second)
				continue
			}

			if h.useCloudIPHelper {
				if err := h.registerCloudIPMap(port, registeredName, h.listenIPs); err != nil {
					logrus.Errorf("while registering listen IPs to cloud: %v", err)

					// sleep and retry
					time.Sleep(30 * time.Second)
					continue
				}
			}
		}

	wait:
		select {
		case <-ch:
			// compare new ip address and last listen IPs, if same, it may be due to dhcp refresh
			newIPs, err = lnet.ListIPs()
			if err != nil {
				logrus.Errorf("got ip address change notification, but list ips got: %v", err)
				// sleep and retry
				newIPs = nil
				time.Sleep(5 * time.Second)
				continue
			}
			if len(newIPs) == len(currIPs) {
				exist := false
				for _, ip := range newIPs {
					_, exist = currIPs[ip.String()]
					if !exist {
						break
					}
				}
				if exist {
					logrus.Infof("IP address not changed btw %v -%v, skip", currIPs, newIPs)
					goto wait
				}
			}

			logrus.Infof("IP address changed %v->%v, start re-register mdns service", currIPs, newIPs)
			if server != nil {
				server.Shutdown()
				mdnsCancel()
			}
		case <-ctx.Done():
			if server != nil {
				server.Shutdown()
			}
			return
		}
	}
}

func (h *Handler) mdnsLog(cb zeroconf.CallbackHook, e error, a []interface{}) {
	if e != nil {
		logrus.Warnf("mdns got error: %v", e)
		return
	}
	respType := ""
	switch cb {
	case zeroconf.ResponseUnicast:
		respType = "unicast"
	case zeroconf.ResponseMulticast:
		respType = "multicast"
	default:
		return
	}

	if len(a) != 4 {
		logrus.Warnf("mdns got invalid length arguments: %v", a)
		return
	}
	rawAddr, ok := a[3].(net.Addr)
	if !ok {
		logrus.Warnf("mdns got invalid from address %v", a[3])
		return
	}
	addr := net.ParseIP(strings.Split(rawAddr.String(), ":")[0])
	if addr == nil {
		logrus.Tracef("mdns error parse address %v", rawAddr)
		return
	}
	addr = addr.To4()
	if addr == nil {
		// skip log request from non-ipv4 address to avoid log flood
		return
	}

	req, ok := a[0].(dns.Question)
	if !ok {
		logrus.Warnf("mdns got invalid request and response to %s", addr)
		return
	}
	resp, ok := a[1].(dns.Msg)
	if !ok {
		logrus.Warnf("mdns got request: %v, %s invalid response to %s", a[0], respType, addr)
		return
	}
	respString := ""
	for i, ans := range resp.Answer {
		respString += fmt.Sprintf("answer %d: %s\n", i, ans)
	}
	for i, ns := range resp.Ns {
		respString += fmt.Sprintf("ns %d: %s\n", i, ns)
	}
	for i, ex := range resp.Extra {
		respString += fmt.Sprintf("addition %d: %s\n", i, ex)
	}
	logrus.Tracef("mdns got %s: %s, %s response to %s with: %s", dns.TypeToString[req.Qtype],
		req.Name, respType, addr, respString)
}

func (h *Handler) registerCloudIPMap(port int, name string, ips []net.IP) error {
	request := []types.IPMapRequest{}
	intfs, err := net.Interfaces()
	if err != nil {
		return err
	}
	ipMap := map[string]net.HardwareAddr{}
	for _, intf := range intfs {
		addrs, err := intf.Addrs()
		if err != nil {
			return err
		}
		for _, addr := range addrs {
			ipaddr, ok := addr.(*net.IPNet)
			if !ok || ipaddr == nil {
				logrus.Warnf("skip empty address: %s", addr)
				continue
			}
			ipMap[ipaddr.IP.To4().String()] = intf.HardwareAddr
		}
	}

	for _, ip := range ips {
		var ok bool
		req := types.IPMapRequest{OS: runtime.GOOS, Arch: runtime.GOARCH,
			LomodVersion: release.Version, PrivateIP: ip, Port: port, Name: name, UUID: h.uuid}
		req.MAC, ok = ipMap[ip.String()]
		if !ok {
			logrus.Errorf("not found %s's corresponding MAC %v", ip, ipMap)
			continue
		}
		request = append(request, req)
	}

	hosts := strings.Split(h.lomocloudConf.host, common.QueryDelimiterKV)
	for _, host := range hosts {
		go func(host string, request []types.IPMapRequest) {
			cli := client.NewLomoCloud(host)
			if err := cli.SetIPMap(request); err != nil {
				logrus.Errorf("failed to register %+v against %s, err: %v", ips, host, err)
			}
		}(host, request)
	}
	return nil
}
