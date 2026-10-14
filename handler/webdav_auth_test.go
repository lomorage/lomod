package handler

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common/security"
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
		{"/alice/Albums/Trip", true},
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

// gateSrv: users alice/pw and bob/pw-b; reads counts storedPassword (DB) calls.
type gateSrv struct {
	*srv
	mu        sync.Mutex // the stub DB is read from parallel requests
	passwords map[string]string
	reads     int
	dbErr     error
}

func newGateSrv() *gateSrv {
	g := &gateSrv{passwords: map[string]string{
		"alice": security.EncryptPassword("alice", "pw"),
		"bob":   security.EncryptPassword("bob", "pw-b"),
	}}
	g.srv = newSrv(nil, func(u string) (string, error) {
		g.mu.Lock()
		defer g.mu.Unlock()
		g.reads++
		return g.passwords[u], g.dbErr
	})
	g.failDelay = 0
	g.maxFailures = 3
	g.blockFor = time.Hour
	return g
}

type gateReq struct {
	remote, path, user, pass string
	header                   map[string]string
}

func (g *gateSrv) do(q gateReq) (*httptest.ResponseRecorder, *http.Request, bool) {
	r := httptest.NewRequest("PROPFIND", q.path, nil)
	r.RemoteAddr = q.remote
	if q.user != "" {
		r.SetBasicAuth(q.user, q.pass)
	}
	for k, v := range q.header {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r2, ok := g.gate(w, r)
	return w, r2, ok
}

const lan = "192.168.1.20:5555"

func TestWebdavGateLocalAndRemote(t *testing.T) {
	g := newGateSrv()

	// same computer: no sign-in, every user visible
	if _, r, ok := g.do(gateReq{remote: "127.0.0.1:1234", path: "/bob/Photos"}); !ok || webdavUser(r.Context()) != "" {
		t.Fatalf("loopback should pass unscoped")
	}

	// from the network without credentials or with a wrong password: asked to sign in
	for _, q := range []gateReq{{path: "/"}, {path: "/", user: "alice", pass: "wrong"}, {path: "/", user: "nobody", pass: "pw"}} {
		q.remote = lan
		w, _, ok := g.do(q)
		if ok || w.Code != http.StatusUnauthorized || w.Header().Get("WWW-Authenticate") == "" {
			t.Fatalf("%+v: got ok=%v code=%d", q, ok, w.Code)
		}
	}

	// signed in: own folder only, the root lists just them
	_, r, ok := g.do(gateReq{remote: lan, path: "/alice/Photos", user: "alice", pass: "pw"})
	if !ok || webdavUser(r.Context()) != "alice" {
		t.Fatalf("own folder should pass scoped to alice")
	}
	if w, _, ok := g.do(gateReq{remote: lan, path: "/bob/Photos", user: "alice", pass: "pw"}); ok || w.Code != http.StatusForbidden {
		t.Fatalf("other user's folder: ok=%v code=%d", ok, w.Code)
	}
	if w, _, ok := g.do(gateReq{remote: lan, path: "/alice/x.jpg", user: "alice", pass: "pw",
		header: map[string]string{"Destination": "http://nas:8004/bob/x.jpg"}}); ok || w.Code != http.StatusForbidden {
		t.Fatalf("move into another user's folder: ok=%v code=%d", ok, w.Code)
	}
}

// A reverse proxy or tunnel on the same computer connects from loopback, but passes on requests
// from anywhere: those must sign in.
func TestWebdavGateProxiedLoopbackMustSignIn(t *testing.T) {
	g := newGateSrv()
	for _, h := range []string{"X-Forwarded-For", "Forwarded", "X-Real-IP"} {
		w, _, ok := g.do(gateReq{remote: "127.0.0.1:1234", path: "/bob/Photos", header: map[string]string{h: "203.0.113.9"}})
		if ok || w.Code != http.StatusUnauthorized {
			t.Fatalf("%s: got ok=%v code=%d", h, ok, w.Code)
		}
	}
}

func TestWebdavGateCachesUntilThePasswordChanges(t *testing.T) {
	g := newGateSrv()
	for i := 0; i < 3; i++ {
		if _, _, ok := g.do(gateReq{remote: lan, path: "/alice", user: "alice", pass: "pw"}); !ok {
			t.Fatalf("request %d refused", i)
		}
	}
	if len(g.authCache) != 1 {
		t.Fatalf("cache has %d entries, want 1", len(g.authCache))
	}

	// password changed (or the user deleted): the old one stops working at once
	g.passwords["alice"] = security.EncryptPassword("alice", "new-pw")
	if _, _, ok := g.do(gateReq{remote: lan, path: "/alice", user: "alice", pass: "pw"}); ok {
		t.Fatalf("old password still accepted after a change")
	}
	if _, _, ok := g.do(gateReq{remote: lan, path: "/alice", user: "alice", pass: "new-pw"}); !ok {
		t.Fatalf("new password refused")
	}
	delete(g.passwords, "alice")
	if _, _, ok := g.do(gateReq{remote: lan, path: "/alice", user: "alice", pass: "new-pw"}); ok {
		t.Fatalf("deleted user still accepted")
	}
}

func TestWebdavGateBlocksAfterRepeatedFailures(t *testing.T) {
	g := newGateSrv()
	for i := 0; i < g.maxFailures; i++ {
		if w, _, _ := g.do(gateReq{remote: lan, path: "/", user: "alice", pass: "guess"}); w.Code != http.StatusUnauthorized {
			t.Fatalf("failure %d: code %d", i, w.Code)
		}
	}
	// now refused without checking, even with the right password
	reads := g.reads
	w, _, ok := g.do(gateReq{remote: lan, path: "/", user: "alice", pass: "pw"})
	if ok || w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "" {
		t.Fatalf("blocked address: ok=%v code=%d", ok, w.Code)
	}
	if g.reads != reads {
		t.Fatalf("a blocked address still reached the DB")
	}
	// other addresses aren't affected
	if _, _, ok := g.do(gateReq{remote: "192.168.1.30:1", path: "/", user: "alice", pass: "pw"}); !ok {
		t.Fatalf("another address was blocked too")
	}
	// a success resets the count
	g2 := newGateSrv()
	g2.do(gateReq{remote: lan, path: "/", user: "alice", pass: "guess"})
	g2.do(gateReq{remote: lan, path: "/", user: "alice", pass: "guess"})
	g2.do(gateReq{remote: lan, path: "/", user: "alice", pass: "pw"})
	g2.do(gateReq{remote: lan, path: "/", user: "alice", pass: "guess"})
	if _, _, ok := g2.do(gateReq{remote: lan, path: "/", user: "alice", pass: "pw"}); !ok {
		t.Fatalf("failures before a success should not count")
	}
}

func TestWebdavGateDBError(t *testing.T) {
	g := newGateSrv()
	g.dbErr = errors.New("database is locked")
	if w, _, ok := g.do(gateReq{remote: lan, path: "/", user: "alice", pass: "pw"}); ok || w.Code != http.StatusServiceUnavailable {
		t.Fatalf("got ok=%v code=%d", ok, w.Code)
	}
}

// slowMatches stands in for the argon2 check: takes a while and records how many run at once.
func slowMatches(g *gateSrv, d time.Duration, running, peak, calls *int32) {
	real := g.matches
	g.matches = func(u, p, stored string) bool {
		atomic.AddInt32(calls, 1)
		n := atomic.AddInt32(running, 1)
		for {
			old := atomic.LoadInt32(peak)
			if n <= old || atomic.CompareAndSwapInt32(peak, old, n) {
				break
			}
		}
		time.Sleep(d)
		atomic.AddInt32(running, -1)
		return real(u, p, stored)
	}
}

func parallel(n int, f func(i int)) {
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); f(i) }(i)
	}
	wg.Wait()
}

// A burst of sign-ins must not run a 4 MB hash each all at once.
func TestWebdavGateBoundsConcurrentHashing(t *testing.T) {
	g := newGateSrv()
	var running, peak, calls int32
	slowMatches(g, 20*time.Millisecond, &running, &peak, &calls)
	parallel(20, func(i int) {
		g.do(gateReq{remote: fmt.Sprintf("192.168.1.%d:1", i+1), path: "/", user: "alice", pass: fmt.Sprintf("guess%d", i)})
	})
	if peak > int32(cap(g.hashSlots)) {
		t.Fatalf("%d hashes ran at once, want at most %d", peak, cap(g.hashSlots))
	}
}

// Explorer's parallel connections with the same right password: hashed once (or once per slot
// that started before the first finished), not once per connection.
func TestWebdavGateHashesParallelSameCredentialsOnce(t *testing.T) {
	g := newGateSrv()
	var running, peak, calls int32
	slowMatches(g, 20*time.Millisecond, &running, &peak, &calls)
	parallel(10, func(int) {
		if _, _, ok := g.do(gateReq{remote: lan, path: "/alice", user: "alice", pass: "pw"}); !ok {
			t.Errorf("refused")
		}
	})
	if calls > int32(cap(g.hashSlots)) {
		t.Fatalf("hashed %d times for 10 identical sign-ins", calls)
	}
}

func TestWebdavGateBusyWhenSlotsAreTaken(t *testing.T) {
	g := newGateSrv()
	g.hashSlots = make(chan struct{}, 1)
	g.hashWait = 10 * time.Millisecond
	release := make(chan struct{})
	g.matches = func(u, p, stored string) bool { <-release; return false }
	done := make(chan struct{})
	go func() { g.do(gateReq{remote: lan, path: "/", user: "alice", pass: "x"}); close(done) }()
	time.Sleep(20 * time.Millisecond) // the first holds the only slot
	w, _, ok := g.do(gateReq{remote: "192.168.1.30:1", path: "/", user: "alice", pass: "pw"})
	close(release)
	<-done
	if ok || w.Code != http.StatusServiceUnavailable || w.Header().Get("Retry-After") == "" {
		t.Fatalf("got ok=%v code=%d", ok, w.Code)
	}
}

func TestWebdavGateForgetsOldFailures(t *testing.T) {
	g := newGateSrv()
	g.failureMemory = 10 * time.Millisecond
	for i := 0; i < g.maxFailures-1; i++ {
		g.do(gateReq{remote: lan, path: "/", user: "alice", pass: "guess"})
	}
	time.Sleep(30 * time.Millisecond)
	for i := 0; i < g.maxFailures-1; i++ {
		g.do(gateReq{remote: lan, path: "/", user: "alice", pass: "guess"})
	}
	if w, _, ok := g.do(gateReq{remote: lan, path: "/", user: "alice", pass: "pw"}); !ok {
		t.Fatalf("blocked by failures that should have been forgotten: %d", w.Code)
	}
}
