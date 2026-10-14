package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsLoopbackAddr(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1:5000":   true,
		"[::1]:5000":       true,
		"127.0.0.1":        true,
		"192.168.1.20:443": false,
		"[fe80::1]:80":     false,
		"garbage":          false,
	} {
		if got := isLoopbackAddr(addr); got != want {
			t.Errorf("isLoopbackAddr(%q) = %v, want %v", addr, got, want)
		}
	}
}

func TestRemotePathAllowed(t *testing.T) {
	for _, c := range []struct {
		path string
		want bool
	}{
		{"/", true},
		{"", true},
		{"/alice", true},
		{"/alice/Photos/2024/01/02/x.jpg", true},
		{"/bob", false},
		{"/bob/Photos", false},
		{"/alice/../bob/Photos", false},
		{"/alicex", false},
	} {
		if got := remotePathAllowed(c.path, "alice"); got != c.want {
			t.Errorf("remotePathAllowed(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func newGateSrv(calls *int) *srv {
	return &srv{verify: func(u, p string) bool {
		*calls++
		return u == "alice" && p == "pw"
	}}
}

func gateRequest(s *srv, remote, path, user, pass, dest string) (*httptest.ResponseRecorder, *http.Request, bool) {
	r := httptest.NewRequest("PROPFIND", path, nil)
	r.RemoteAddr = remote
	if user != "" {
		r.SetBasicAuth(user, pass)
	}
	if dest != "" {
		r.Header.Set("Destination", dest)
	}
	w := httptest.NewRecorder()
	r2, ok := s.gate(w, r)
	return w, r2, ok
}

func TestWebdavGate(t *testing.T) {
	calls := 0
	s := newGateSrv(&calls)

	// same computer: no sign-in, every user visible
	if _, r, ok := gateRequest(s, "127.0.0.1:1234", "/bob/Photos", "", "", ""); !ok || webdavUser(r.Context()) != "" {
		t.Fatalf("loopback should pass unscoped")
	}

	// from the network without credentials or with a wrong password: asked to sign in
	for _, pass := range []string{"", "wrong"} {
		user := ""
		if pass != "" {
			user = "alice"
		}
		w, _, ok := gateRequest(s, "192.168.1.20:5555", "/", user, pass, "")
		if ok || w.Code != http.StatusUnauthorized || w.Header().Get("WWW-Authenticate") == "" {
			t.Fatalf("pass %q: got ok=%v code=%d", pass, ok, w.Code)
		}
	}

	// signed in: own folder only, the root lists just them
	calls = 0
	_, r, ok := gateRequest(s, "192.168.1.20:5555", "/alice/Photos", "alice", "pw", "")
	if !ok || webdavUser(r.Context()) != "alice" {
		t.Fatalf("own folder should pass scoped to alice")
	}
	if w, _, ok := gateRequest(s, "192.168.1.20:5555", "/bob/Photos", "alice", "pw", ""); ok || w.Code != http.StatusForbidden {
		t.Fatalf("other user's folder: ok=%v code=%d", ok, w.Code)
	}
	if w, _, ok := gateRequest(s, "192.168.1.20:5555", "/alice/x.jpg", "alice", "pw", "http://nas:8004/bob/x.jpg"); ok || w.Code != http.StatusForbidden {
		t.Fatalf("move into another user's folder: ok=%v code=%d", ok, w.Code)
	}
	// the password was checked once; later requests use the cache
	if calls != 1 {
		t.Fatalf("verify called %d times, want 1", calls)
	}
	// a different password is checked again, not served from the cache
	if _, _, ok := gateRequest(s, "192.168.1.20:5555", "/alice", "alice", "pw2", ""); ok || calls != 2 {
		t.Fatalf("wrong password must not hit the cache: ok=%v calls=%d", ok, calls)
	}
}
