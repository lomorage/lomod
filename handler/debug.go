package handler

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/cmd"
	"bitbucket.org/lomoware/lomo-backend/common/logger"
	lnet "bitbucket.org/lomoware/lomo-backend/common/net"
	"github.com/jackpal/gateway"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	pcapExt        = ".pcap"
	pcapFilePrefix = "lomod_debug"
	pingExt        = ".csv"
	pingFilePrefix = "lomod_debug_ping"
	cmdFilePrefix  = "lomod_debug_cmd"
)

type pingTarget struct {
	file   *os.File
	local  *net.IPAddr
	remote *net.IPAddr
}

type debugCollector struct {
	ctx          context.Context
	cancel       context.CancelFunc
	currSize     uint
	oversized    bool
	pcapFilename string
	pingCount    int
	pingTargets  []pingTarget
}

func (h *Handler) getPcapFilename(devName string) string {
	return filepath.Join(h.conf.LogDir, pcapFilePrefix+"_"+devName+pcapExt)
}

func (h *Handler) getPingFilename(devName, dst string) string {
	return filepath.Join(h.conf.LogDir, fmt.Sprintf("%s_%s_%s%s", pingFilePrefix, devName, dst, pingExt))
}

func (h *Handler) getPcapDevices(listenIPs []net.IP) (map[string]*net.IPAddr, error) {
	devices, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	names := map[string]*net.IPAddr{}
	ips := map[string]net.IP{}
	for _, ip := range listenIPs {
		ips[ip.String()] = ip
	}
	for _, d := range devices {
		if strings.HasPrefix(d.Name, "br-") {
			logrus.Infof("skip docker bridge device: %s", d.Name)
			continue
		}
		addrs, err := d.Addrs()
		if err != nil {
			logrus.Infof("failed to get device %s address: %s", d.Name, err)
			continue
		}
		if len(addrs) == 0 {
			logrus.Infof("device %s no address", d.Name)
			continue
		}
		if len(listenIPs) == 0 {
			// find the first ip address
			for _, addr := range addrs {
				ipaddr, ok := addr.(*net.IPAddr)
				if !ok {
					continue
				}
				names[d.Name] = ipaddr
				break
			}
		}
		for _, addr := range addrs {
			var ipaddr *net.IPAddr
			switch v := addr.(type) {
			case *net.IPNet:
				ipaddr = &net.IPAddr{IP: v.IP}
			case *net.IPAddr:
				ipaddr = v
			}
			if ipaddr == nil {
				continue
			}
			_, ok := ips[ipaddr.IP.String()]
			if !ok {
				continue
			}
			names[d.Name] = ipaddr
			break
		}
	}
	return names, nil
}

func (h *Handler) startDebugCollect(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	duration, err := time.ParseDuration(r.URL.Query().Get(common.QueryKeyDuration))
	if err != nil {
		logrus.Warnf("received invalid duration %s: %v", r.URL.Query().Get(common.QueryKeyDuration), err)
		duration = h.conf.MaxCaptureDuration
	}

	listenIPs, err := lnet.ListIPs()
	if err != nil {
		logrus.Warnf("capture all devices because of listing ips: %v", err)
		common.WriteError(w, errors.Wrap(wl.Err, "listing listen IPs"))
		return
	}
	devices, err := h.getPcapDevices(listenIPs)
	if err != nil {
		common.WriteError(w, wl.Err)
		return
	}
	logrus.Infof("local devices: %v", devices)

	h.debugLock.Lock()
	defer h.debugLock.Unlock()

	for dname, c := range h.debugCollectors {
		for _, t := range c.pingTargets {
			if t.file != nil {
				if err := t.file.Close(); err != nil {
					logrus.Warnf("close ping file %s: %s", t.file.Name(), err)
				}
			}
		}
		delete(h.debugCollectors, dname)
	}

	// 3 ping target: public address, remote client and default gateway
	remoteAddrs := []*net.IPAddr{h.conf.RemotePing}
	addrs := strings.Split(r.RemoteAddr, ":")
	if ip, err := net.ResolveIPAddr("ip", addrs[0]); err != nil {
		logrus.Warnf("invalid remote address %s: %s", r.RemoteAddr, err)
	} else {
		remoteAddrs = append(remoteAddrs, ip)
	}

	gw, err := gateway.DiscoverGateway()
	if err != nil {
		logrus.Warnf("failed to discover gateway: %s", err)
	} else {
		remoteAddrs = append(remoteAddrs, &net.IPAddr{IP: gw})
	}
	logrus.Infof("ping remote addresses: %v", remoteAddrs)

	for dev, ipaddr := range devices {
		collector := &debugCollector{pingCount: int(duration / time.Second), pingTargets: []pingTarget{}}
		collector.ctx, collector.cancel = context.WithTimeout(h.gCtx, duration)

		// recreate pcap file, and make sure to remove the file
		filename := h.getPcapFilename(dev)
		if err := cmd.ExecWithSudo("rm", filename); err != nil {
			logrus.Warnf("rm pcap file %s: %v", filename, err)
		}
		file, err := os.Create(filename)
		if err != nil {
			logrus.Warnf("create pcap file %s: %v", filename, err)
		} else if err := file.Close(); err != nil {
			logrus.Warnf("close pcap file %s: %v", filename, err)
		} else {
			collector.pcapFilename = filename
		}

		// ping file
		for _, remote := range remoteAddrs {
			target := pingTarget{local: ipaddr, remote: remote}
			filename = h.getPingFilename(dev, remote.String())
			target.file, err = os.Create(filename)
			if err != nil {
				logrus.Warnf("create ping file %s: %v", filename, err)
				continue
			}
			collector.pingTargets = append(collector.pingTargets, target)
		}

		h.debugCollectors[dev] = collector
		go h.collectPcapPackets(dev, collector)
		for _, p := range collector.pingTargets {
			go h.collectPingResults(collector.ctx, dev, p, collector.pingCount)
		}
	}
	go h.collectDebugCommandResults()
}
func (h *Handler) collectPcapPackets(dev string, collector *debugCollector) {
	logrus.Infof("start write %s's pcap file: %s", dev, collector.pcapFilename)
	if err := runPacketCapture(collector.ctx, dev, collector.pcapFilename); err != nil {
		logrus.Warnf("fail collecting pcap %s: %s", collector.pcapFilename, err)
	}
	logrus.Infof("finish collect pcap %s", collector.pcapFilename)
}

func (h *Handler) collectPingResults(ctx context.Context, dev string, target pingTarget, count int) {
	logrus.Infof("start ping %s", target.file.Name())
	if err := runPing(ctx, dev, target.remote.String(), target.file, count); err != nil {
		logrus.Warnf("fail ping %s: %s", target.file.Name(), err)
	}
	logrus.Infof("finish ping %s", target.file.Name())
}

func (h *Handler) collectDebugCommandResults() {
	filename := filepath.Join(h.conf.LogDir, cmdFilePrefix+logExt)
	file, err := os.Create(filename)
	if err != nil {
		logrus.Warnf("create debug command result file %s: %v", filename, err)
		return
	}
	defer file.Close()
	if err := runDebugCommands(file); err != nil {
		logrus.Warnf("run debug command: %v", err)
	}
}

func (h *Handler) stopDebugCollect(w http.ResponseWriter, r *http.Request) {
	h.debugLock.Lock()
	defer h.debugLock.Unlock()

	for _, c := range h.debugCollectors {
		logrus.Info("stop collect debug files")
		c.cancel()
	}
}
