package handler

import (
	"net"
	"net/url"
	"strings"
	"testing"
)

func TestSetupQRContentIsHomepageLinkWithFragmentParams(t *testing.T) {
	h := &Handler{
		conf:      &Config{ListenPort: 8000, MdnsName: "Home NAS"},
		uuid:      "abc-123",
		listenIPs: []net.IP{net.ParseIP("192.168.1.194")},
	}
	link := h.setupQRContent()
	if !strings.HasPrefix(link, setupLinkBase+"#") {
		t.Fatalf("setup link %q does not start with %q#", link, setupLinkBase)
	}
	params, err := url.ParseQuery(strings.TrimPrefix(link, setupLinkBase+"#"))
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"server": "192.168.1.194:8000", "uuid": "abc-123", "name": "Home NAS"} {
		if got := params.Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}
