package logger

import (
	"net/url"
	"strings"
	"testing"
)

func TestLoggableURLDropsSecretsAndFileNames(t *testing.T) {
	u, _ := url.Parse("/asset/abc?ext=jpg&token=secret&origname=Anna%20passport.jpg&createtime=2003-11-23T12:00:00Z")
	got := loggableURL(u)
	for _, leaked := range []string{"secret", "Anna", "passport", "origname", "token"} {
		if strings.Contains(got, leaked) {
			t.Errorf("logged URL %q contains %q", got, leaked)
		}
	}
	if !strings.Contains(got, "ext=jpg") || !strings.HasPrefix(got, "/asset/abc?") {
		t.Errorf("logged URL %q lost what it should keep", got)
	}
}
