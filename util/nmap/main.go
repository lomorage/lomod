package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"

	nmap "github.com/lair-framework/go-nmap"
)

func localIPs() ([]string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	ips := []string{}
	for _, addr := range addrs {
		//fmt.Println(addr.Network())
		parts := strings.SplitN(addr.String(), "/", 2)
		if len(parts) != 2 || parts[0] == "127.0.0.1" || parts[0] == "::1" || parts[1] == "64" || parts[1] == "128" {
			continue
		}
		ips = append(ips, addr.String())
	}
	return ips, nil
}

type remoteHost struct {
	name string
	ip   string
	port int
}

func scanHost(host nmap.Host) []remoteHost {
	remotes := []remoteHost{}
	for _, port := range host.Ports {
		if port.State.State != "open" {
			continue
		}
		remote := remoteHost{port: port.PortId}
		if host.Hostnames != nil && len(host.Hostnames) > 0 {
			remote.name = host.Hostnames[0].Name
		}
		if host.Addresses != nil && len(host.Addresses) > 0 {
			remote.ip = host.Addresses[0].Addr
		}
		if remote.name == "" && remote.ip != "" {
			// try MDNS to find out hostname
			out, err := exec.Command("dig", "+short", "-x", remote.ip, "@224.0.0.251", "-p", "5353").CombinedOutput()
			if err != nil {
				log.Println(err)
			}
			remote.name = string(out)
		}

		remotes = append(remotes, remote)
	}
	return remotes
}

func scanHosts(addr string) ([]remoteHost, error) {
	tmpfile, err := ioutil.TempFile("", "scan-")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpfile.Name())
	if err := tmpfile.Close(); err != nil {
		return nil, err
	}

	out, err := exec.Command("nmap", "-sS", "-oX", tmpfile.Name(), "-p445,548", addr).CombinedOutput()
	if err != nil {
		log.Println(out)
		return nil, err
	}

	content, err := ioutil.ReadFile(tmpfile.Name())
	if err != nil {
		return nil, err
	}
	parse, err := nmap.Parse(content)
	if err != nil {
		return nil, err
	}
	remotes := []remoteHost{}
	for _, host := range parse.Hosts {
		rh := scanHost(host)

		remotes = append(remotes, rh...)
	}
	return remotes, nil
}

func main() {
	addrs, err := localIPs()
	if err != nil {
		log.Fatal(err)
	}
	for _, addr := range addrs {
		hosts, err := scanHosts(addr)
		if err != nil {
			fmt.Println(err)
			continue
		}
		for _, host := range hosts {
			fmt.Println(host)
		}
	}
}
