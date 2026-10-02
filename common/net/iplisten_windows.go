package net

// NewIPAddrListener creates ip address listener
func NewIPAddrListener() (*IPAddrListener, error) {
	return &IPAddrListener{}, nil
}

func (l *IPAddrListener) monitorIPChange(ch chan struct{}) {
}
