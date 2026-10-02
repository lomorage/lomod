package net

import (
	"context"
	"time"

	"github.com/leslie-wang/zeroconf"
	"github.com/pkg/errors"
)

// DiscoverMDNSServices discover announced dns service via its service name and domain name.
func DiscoverMDNSServices(c context.Context, service, domain string,
	timeout time.Duration) ([]*zeroconf.ServiceEntry, error) {
	// Discover all services on the network (e.g. _workstation._tcp)
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create resolver")
	}

	entries := []*zeroconf.ServiceEntry{}
	entryChan := make(chan *zeroconf.ServiceEntry)
	go func(results <-chan *zeroconf.ServiceEntry) {
		for entry := range results {
			entries = append(entries, entry)
		}
	}(entryChan)

	ctx, cancel := context.WithTimeout(c, timeout)
	defer cancel()
	err = resolver.Browse(ctx, service, domain, entryChan)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to browse")
	}

	<-ctx.Done()
	return entries, nil
}
