package types

import (
	"errors"
	"net"
)

// IPMapRequest is the structure for ip map request.
type IPMapRequest struct {
	PrivateIP    net.IP
	Port         int
	MAC          net.HardwareAddr
	UUID         string
	Name         string
	OS           string
	Arch         string
	LomodVersion string
}

// Validate validate IP map request
func (r *IPMapRequest) Validate() error {
	if r.MAC == nil {
		return errors.New("IPMap MAC address should not be empty")
	}
	if r.PrivateIP == nil {
		return errors.New("IPMap Private IP should not be empty")
	}
	return nil
}

// MkReply converts ip map request to reply
func (r *IPMapRequest) MkReply() *IPMapReply {
	return &IPMapReply{
		Name:      r.Name,
		UUID:      r.UUID,
		Port:      r.Port,
		//PublicIP:  publicIP,
		PrivateIP: r.PrivateIP,
		MAC:       r.MAC,
	}
}

// IPMapReply is the structure for ip map.
type IPMapReply struct {
	Name      string
	UUID      string
	Port      int
	//PublicIP  net.IP
	PrivateIP net.IP
	MAC       net.HardwareAddr
}

// PortMapReply is the structure for port mapping.
type PortMapReply struct {
	ClientIP string
}
