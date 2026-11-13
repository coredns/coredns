package test

import (
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

// localEmptyReplyDNSServer runs a DNS server that serves NOERROR, returning the addr as a string
func localEmptyReplyDNSServer(t *testing.T) string {
	t.Helper()

	s := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
		response := new(dns.Msg)

		response.SetReply(r)
		// Callers of this function don't care about the result very much
		_ = w.WriteMsg(response)
	})

	t.Cleanup(s.Close)
	return s.Addr
}

// localGTLDARecordDNSServer runs a DNS server that knows a single A record, returning the addr as a string
func localGTLDARecordDNSServer(t *testing.T) string {
	t.Helper()

	s := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
		response := new(dns.Msg)

		response.SetReply(r)
		if len(r.Question) > 0 && r.Question[0].Qtype == dns.TypeA && r.Question[0].Name == "h.gtld-servers.net." {
			response.Answer = []dns.RR{test.A("h.gtld-servers.net. 1 IN A 192.0.2.1")}
		}
		// Callers of this function *do* care about results
		if err := w.WriteMsg(response); err != nil {
			t.Error(err)
		}
	})

	t.Cleanup(s.Close)
	return s.Addr
}

// localExampleComDNSServer runs a DNS server that knows a single A record, returning the addr as a string
func localExampleComDNSServer(t *testing.T) string {
	t.Helper()

	s := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
		response := new(dns.Msg)

		response.SetReply(r)
		if len(r.Question) > 0 && r.Question[0].Qtype == dns.TypeA && r.Question[0].Name == "example.com." {
			response.Answer = []dns.RR{test.A("example.com. 1 IN A 192.0.2.99")}
		}
		// Callers of this function *do* care about results
		if err := w.WriteMsg(response); err != nil {
			t.Error(err)
		}
	})

	t.Cleanup(s.Close)
	return s.Addr
}
