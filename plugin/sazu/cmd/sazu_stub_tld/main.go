// Command sazu-stub-tld is a minimal parent-zone stand-in for testing
// SAZU's chain-of-trust cross-check without a real TLD -- the Go/CoreDNS
// counterpart to the earlier Rust/rDNS port's sazu-stub-tld.
//
// It loads the same key file a sazuctl push uses, derives its DNSKEY and
// the real RFC 4034 §5.1.4 DS digest for it, and answers exactly two query
// shapes for the configured zone: DS and NS. Everything else gets
// NXDOMAIN/NODATA. It is a real, dig-able UDP DNS responder (built on
// miekg/dns's own server framework), but it only stands in for "what would
// the real parent say" for manual, byte-level verification -- it cannot
// substitute for a real delegation chain, so plugin/sazu.Validator's full
// root-to-parent walk still needs a real, DNSSEC-signed domain to test
// against end to end.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/coredns/coredns/plugin/sazu"

	"github.com/miekg/dns"
)

func main() {
	zone := flag.String("zone", "", "zone this stub is authoritative for, e.g. example.org")
	keyPath := flag.String("key", "", "key file -- the same one a sazuctl push used (created if missing)")
	ns := flag.String("ns", "", "nameserver name to answer NS queries with")
	listen := flag.String("listen", "127.0.0.1:8053", "address to listen on")
	flag.Parse()
	if *zone == "" || *keyPath == "" || *ns == "" {
		fmt.Fprintln(os.Stderr, "usage: sazu-stub-tld -zone <zone> -key <path> -ns <nameserver> [-listen host:port]")
		os.Exit(1)
	}

	key, _, generated, err := sazu.LoadOrGenerateKey(*keyPath, *zone, true, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sazu-stub-tld: error: %v\n", err)
		os.Exit(1)
	}
	if generated {
		fmt.Fprintf(os.Stderr, "No key found at %s -- generated a new one.\n", *keyPath)
	}
	ds := key.ToDS(dns.SHA256)

	fmt.Printf("sazu-stub-tld: serving DS/NS for %s on %s\n", *zone, *listen)
	fmt.Printf("  DS key tag: %d\n", ds.KeyTag)
	fmt.Printf("  DS digest (hex): %s\n", ds.Digest)

	zoneFQDN := dns.Fqdn(*zone)
	nsFQDN := dns.Fqdn(*ns)

	dns.HandleFunc(zoneFQDN, func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Authoritative = true

		if len(r.Question) != 1 {
			m.Rcode = dns.RcodeFormatError
			_ = w.WriteMsg(m)
			return
		}
		q := r.Question[0]
		if !strings.EqualFold(q.Name, zoneFQDN) {
			m.Rcode = dns.RcodeNameError
			_ = w.WriteMsg(m)
			return
		}
		switch q.Qtype {
		case dns.TypeDS:
			m.Answer = append(m.Answer, ds)
		case dns.TypeNS:
			m.Answer = append(m.Answer, &dns.NS{
				Hdr: dns.RR_Header{Name: zoneFQDN, Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: 3600},
				Ns:  nsFQDN,
			})
		default:
			// NOERROR/NODATA -- name exists, this type doesn't.
		}
		_ = w.WriteMsg(m)
	})

	srv := &dns.Server{Addr: *listen, Net: "udp"}
	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "sazu-stub-tld: error: %v\n", err)
		os.Exit(1)
	}
}
