package test

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestCacheACLAuthorization(t *testing.T) {
	const (
		blockedSource = "127.0.0.1"
		allowedSource = "127.0.0.2"
		queryName     = "secret.protected.example."
	)

	exchange := func(server, source string) *dns.Msg {
		t.Helper()

		remoteAddr, err := net.ResolveUDPAddr("udp4", server)
		if err != nil {
			t.Fatalf("resolve server address %q: %v", server, err)
		}

		conn, err := net.DialUDP(
			"udp4",
			&net.UDPAddr{
				IP: net.ParseIP(source).To4(),
			},
			remoteAddr,
		)
		if err != nil {
			t.Fatalf("dial from %s to %s: %v", source, server, err)
		}
		defer conn.Close()

		request := new(dns.Msg)
		request.SetQuestion(queryName, dns.TypeA)

		client := &dns.Client{
			Net:     "udp4",
			Timeout: time.Second,
		}

		response, _, err := client.ExchangeWithConn(
			request,
			&dns.Conn{Conn: conn},
		)
		if err != nil {
			t.Fatalf("query from %s to %s: %v", source, server, err)
		}

		return response
	}

	corefile := func(withCache bool) string {
		cache := ""
		if withCache {
			cache = "\n\tcache 30"
		}

		return fmt.Sprintf(`.:0 {
	bind 127.0.0.1
	acl protected.example. {
		allow net %s/32
		block
	}%s
	template IN A protected.example. {
		match ^secret[.]protected[.]example[.]$
		answer "{{ .Name }} 30 IN A 192.0.2.124"
	}
}`, allowedSource, cache)
	}

	requireAnswer := func(response *dns.Msg, description string) {
		t.Helper()

		if response.Rcode != dns.RcodeSuccess {
			t.Fatalf(
				"%s: got rcode %s, want NOERROR",
				description,
				dns.RcodeToString[response.Rcode],
			)
		}

		if len(response.Answer) != 1 {
			t.Fatalf(
				"%s: got answers %v, want one A record",
				description,
				response.Answer,
			)
		}

		answer, ok := response.Answer[0].(*dns.A)
		if !ok || !answer.A.Equal(net.ParseIP("192.0.2.124")) {
			t.Fatalf(
				"%s: got answers %v, want 192.0.2.124",
				description,
				response.Answer,
			)
		}
	}

	// Control: prove that the ACL works correctly without cache.
	control, controlUDP, _, err := CoreDNSServerAndPorts(corefile(false))
	if err != nil {
		t.Fatalf("start no-cache control server: %v", err)
	}

	requireAnswer(
		exchange(controlUDP, allowedSource),
		"no-cache allowed query",
	)

	response := exchange(controlUDP, blockedSource)
	if response.Rcode != dns.RcodeRefused {
		control.Stop()
		t.Fatalf(
			"no-cache blocked query: got rcode %s, want REFUSED",
			dns.RcodeToString[response.Rcode],
		)
	}

	control.Stop()

	// Actual regression test: allowed client fills the shared cache.
	instance, udp, _, err := CoreDNSServerAndPorts(corefile(true))
	if err != nil {
		t.Fatalf("start cache server: %v", err)
	}
	defer instance.Stop()

	requireAnswer(
		exchange(udp, allowedSource),
		"cache-fill allowed query",
	)

	// This must remain REFUSED. Current vulnerable code returns the
	// cached NOERROR answer without calling ACL.
	response = exchange(udp, blockedSource)
	if response.Rcode != dns.RcodeRefused {
		t.Fatalf(
			"blocked query after allowed cache fill: got rcode %s, want REFUSED",
			dns.RcodeToString[response.Rcode],
		)
	}
}
