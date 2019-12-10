package route53

import (
	"context"
	"fmt"
	"github.com/miekg/dns"
	"net"
	"testing"
	"time"
)

func TestAuthoritativeNsResolver_Resolve(t *testing.T) {
	resolver := authoritativeNsResolver{}

	dnsAddr := "127.0.0.1:31726"
	server := &dns.Server{
		Addr:      dnsAddr,
		Net:       "udp",
		ReusePort: true,
	}

	dns.HandleFunc("example.dev.", func(w dns.ResponseWriter, r *dns.Msg) {
		q := r.Question[0]
		m := new(dns.Msg)
		m.SetReply(r)
		var rr dns.RR
		switch q.Qtype {
		case dns.TypeA:
			rr = &dns.A{
				Hdr: dns.RR_Header{Name: "example.dev.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
				A:   net.ParseIP("10.0.0.0"),
			}
		case dns.TypeAAAA:
			rr = &dns.AAAA{
				Hdr:  dns.RR_Header{Name: "example.dev.", Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 60},
				AAAA: net.ParseIP("fdf8:f53b:82e4::53"),
			}
		}
		m.Answer = append(m.Answer, rr)
		_ = w.WriteMsg(m)
	})

	go func() {
		err := server.ListenAndServe()
		fmt.Println(err)
	}()
	time.Sleep(time.Millisecond * 50) // so we can start the server

	_, err := resolver.Resolve(context.Background(), "test.example.dev.", dns.TypeA, dnsAddr, 60)
	if err != nil {
		t.Fatalf("Failed to resolve A record: %v", err)
	}

	_, err = resolver.Resolve(context.Background(), "test.example.dev.", dns.TypeAAAA, dnsAddr, 60)
	if err != nil {
		t.Fatalf("Failed to resolve AAAA record: %v", err)
	}
}

func TestAuthoritativeNsResolver_ResolveInvalidPort(t *testing.T) {
	resolver := authoritativeNsResolver{}
	_, err := resolver.Resolve(context.Background(), "test.example.dev.", dns.TypeA, "127.0.0.1:8054", 60)
	if err == nil {
		t.Fatalf("Why did it resolve the record, should have failed")
	}
}
