package handler

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/leslie-wang/zeroconf"
	. "gopkg.in/check.v1"
)

func (ts *mainSuite) TestMDNS(c *C) {
	// by default mdns is not enabled
	resolver, err := zeroconf.NewResolver(nil)
	c.Assert(err, IsNil)

	entries1 := make(chan *zeroconf.ServiceEntry)
	go func(results <-chan *zeroconf.ServiceEntry) {
		s := <-results
		c.Assert(s, IsNil)
	}(entries1)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	c.Assert(resolver.Browse(ctx, mdnsService, mdnsDomain, entries1), IsNil)

	<-ctx.Done()
	cancel()

	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	go ts.h.processMDNS(ctx, port, mdnsName, mdnsService, mdnsDomain, true)

	time.Sleep(time.Second + resolvTimeout)

	ts.testMDNSBasic(c, []string{"os=" + runtime.GOOS, "uuid=" + ts.h.uuid})

	// start second one, which should be success
	go ts.h.processMDNS(ctx, port, mdnsName, mdnsService, mdnsDomain, true)

	time.Sleep(time.Second + resolvTimeout)

	name := calculateInstanceName(ctx, mdnsName, mdnsService, mdnsDomain)
	c.Assert(name, Equals, mdnsName+delimit+"2")
}

func (ts *mainSuite) testMDNSBasic(c *C, text []string) {
	resolver, err := zeroconf.NewResolver(nil)
	c.Assert(err, IsNil)
	entries := make(chan *zeroconf.ServiceEntry)
	resolveResult := []*zeroconf.ServiceEntry{}
	go func(results <-chan *zeroconf.ServiceEntry) {
		s := <-results
		resolveResult = append(resolveResult, s)
	}(entries)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	c.Assert(resolver.Browse(ctx, mdnsService, mdnsDomain, entries), IsNil)

	<-ctx.Done()
	cancel()

	c.Assert(len(resolveResult), Equals, 1)
	c.Assert(resolveResult[0].Domain, Equals, mdnsDomain)
	c.Assert(resolveResult[0].Service, Equals, mdnsService)
	c.Assert(resolveResult[0].Instance, Equals, mdnsName)
	c.Assert(resolveResult[0].Port, Equals, port)
	c.Assert(resolveResult[0].Text, DeepEquals, text)
}

// To monitor ip change, probably receives many concurrent messages.
func (ts *mainSuite) TestMDNSStartStop(c *C) {
	text := []string{"txtv=0", "lo=1", "la=2"}
	for i := 0; i < 20; i++ {
		fmt.Printf("#%d mdns start stop test\n", i+1)
		server, err := zeroconf.Register(mdnsName, mdnsService, mdnsDomain, port, text, nil)
		c.Assert(err, IsNil)

		ts.testMDNSBasic(c, text)

		server.Shutdown()
	}
}

func (ts *mainSuite) TestMDNSMultiInstances(c *C) {
	// case 1: without any instance, or register 'test--123456789999', calculateInstanceName should return return 'test--123456789'
	// case 2: after register 'test--123456789', calculateInstanceName should return 'test--123456789-1'
	// case 3: after register both 'test--123456789' and 'test--123456789-1', calculateInstanceName should return 'test--123456789-2'
	// case 4: after register, 'test--123456789', 'test--123456789-1' and 'test--123456789-5', calculateInstanceName should return 'test--123456789-2'
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// case 1: without any instance, calculateInstanceName should return 'test--123456789'
	name := calculateInstanceName(ctx, mdnsName, mdnsService, mdnsDomain)
	c.Assert(name, Equals, mdnsName)

	server, err := zeroconf.Register(mdnsName+"xxx", mdnsService, mdnsDomain, port, []string{"txtv=0", "lo=1", "la=2"}, nil)
	c.Assert(err, IsNil)
	defer server.Shutdown()

	name = calculateInstanceName(ctx, mdnsName, mdnsService, mdnsDomain)
	c.Assert(name, Equals, mdnsName)

	// case 2: after register 'test--123456789', calculateInstanceName should 'test--123456789-1'
	server1, err := zeroconf.Register(mdnsName, mdnsService, mdnsDomain, port, []string{"txtv=0", "lo=1", "la=2"}, nil)
	c.Assert(err, IsNil)
	defer server1.Shutdown()

	name = calculateInstanceName(ctx, mdnsName, mdnsService, mdnsDomain)
	c.Assert(name, Equals, mdnsName+delimit+"1")

	// case 3: after register both 'test--123456789' and 'test--123456789-1', calculateInstanceName should 'test--123456789-2'
	server2, err := zeroconf.Register(name, mdnsService, mdnsDomain, port, []string{"txtv=0", "lo=1", "la=2"}, nil)
	c.Assert(err, IsNil)
	defer server2.Shutdown()

	name = calculateInstanceName(ctx, mdnsName, mdnsService, mdnsDomain)
	c.Assert(name, Equals, mdnsName+delimit+"2")

	// case 4: after register, 'test--123456789', 'test--123456789-1' and 'test--123456789-5', calculateInstanceName should 'test--123456789-2'
	server3, err := zeroconf.Register(mdnsName+delimit+"5", mdnsService, mdnsDomain, port, []string{"txtv=0", "lo=1", "la=2"}, nil)
	c.Assert(err, IsNil)
	defer server3.Shutdown()

	name = calculateInstanceName(ctx, mdnsName, mdnsService, mdnsDomain)
	c.Assert(name, Equals, mdnsName+delimit+"2")
}
