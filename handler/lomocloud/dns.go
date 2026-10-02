package lomocloud

import (
	"context"
	"database/sql"
	"net"
	"strings"

	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"github.com/miekg/dns"
	"github.com/sirupsen/logrus"
)

const (
	soa = "hub.lomorage.com"
	ns1 = "ns1.lomorage.com"
)

// ServeDNS is the main callback for miekg/dns. Collects information about the
// query, constructs a response, and returns it to the connector.
func (h *Handler) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	m := &dns.Msg{}
	m.SetReply(r)

	answers := []dns.RR{}
	skipNX := false

	for _, question := range r.Question {
		// nil records == not found
		switch question.Qtype {
		case dns.TypeA:
			fqdn := question.Name
			logrus.Infof("query A: %s", question.String())
			if strings.ToLower(strings.TrimSuffix(fqdn, ".")) == soa {
				continue
			}
			a, err := h.getA(fqdn)
			if err == nil && a != nil {
				for _, rr := range a {
					answers = append(answers, dns.RR(rr))
				}
			}
		case dns.TypeAAAA:
			logrus.Infof("query AAAA: %s", question.String())
			skipNX = true
		case dns.TypeCAA:
			logrus.Infof("query CAA: %s", question.String())
			skipNX = true
		case dns.TypeSOA:
			logrus.Infof("query SOA: %s", question.String())
			answers = []dns.RR{dns.RR(h.getSOA())}
		case dns.TypeNS:
			logrus.Infof("query NS: %s", question.String())
			skipNX = true
		default:
			logrus.Infof("unimplemented query: %s", question.String())
		}
	}

	// If we have no answers, that means we found nothing or didn't get a query
	// we can reply to. Reply with no answers so we ensure the query moves on to
	// the next server.

	if len(answers) == 0 && !skipNX { // for situations like the `host` tool
		m.SetRcode(r, dns.RcodeNameError)
		w.WriteMsg(m)
		return
	}

	// Without these the glibc resolver gets very angry.
	m.Authoritative = true
	m.RecursionAvailable = true
	m.Answer = answers
	m.SetRcode(r, dns.RcodeSuccess)
	w.WriteMsg(m)
}

func (h *Handler) getA(fqdn string) ([]*dns.A, error) {
	var ip string
	// also support domain search with www.
	parts := strings.Split(strings.TrimPrefix(fqdn, "www."), ".")
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "select ip from portmap where subdomain = ?", strings.ToLower(parts[0])).Scan(&ip)
	}); err != nil {
		return nil, err
	}

	records := []*dns.A{{
		Hdr: dns.RR_Header{
			Name:   fqdn,
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			// 0 TTL results in UB for DNS resolvers and generally causes problems.
			Ttl: 1,
		},
		A: net.ParseIP(ip),
	}}

	return records, nil
}

/*
func (h *Handler) getNS() *dns.NS {
	return &dns.NS{
		Hdr: dns.RR_Header{
			Name:   soa,
			Rrtype: dns.TypeNS,
			Class:  dns.ClassINET,
			// 0 TTL results in UB for DNS resolvers and generally causes problems.
			Ttl: 1,
		},
		Ns: ns1,
	}
}
*/

func (h *Handler) getSOA() *dns.SOA {
	return &dns.SOA{
		Hdr: dns.RR_Header{
			Name:   soa,
			Rrtype: dns.TypeSOA,
			Class:  dns.ClassINET,
			// 0 TTL results in UB for DNS resolvers and generally causes problems.
			Ttl: 1,
		},
		Ns:     soa,
		Mbox:   ns1,
		Serial: 6,
	}
}
