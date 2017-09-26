package test

import (
	"testing"

	"github.com/miekg/dns"
)

func TestParseDigResponse(t *testing.T) {

	r := `; <<>> DiG 9.4.1-P1 <<>> mt-example.com
;; global options:  printcmd
;; Got answer:
;; ->>HEADER<<- opcode: QUERY, status: NXDOMAIN, id: 25550
;; flags: qr rd ra; QUERY: 1, ANSWER: 0, AUTHORITY: 0, ADDITIONAL: 0

;; QUESTION SECTION:
;svc-1-a.test-1.svc.cluster.local.			IN	A

;; AUTHORITY SECTION:
cluster.local.	303	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1502313310 7200 1800 86400 60

;; Query time: 4 msec
;; SERVER: 64.207.129.21#53(64.207.129.21)
;; WHEN: Thu Aug  7 16:49:35 2008
;; MSG SIZE  rcvd: 48


; <<>> DiG 9.4.1-P1 <<>> mt-example.com
;; global options:  printcmd
;; Got answer:
;; ->>HEADER<<- opcode: QUERY, status: NOERROR, id: 25550
;; flags: qr rd ra; QUERY: 1, ANSWER: 3, AUTHORITY: 1, ADDITIONAL: 2

;; QUESTION SECTION:
;svc-1-a.test-1.svc.cluster.local.			IN	A

;; ANSWER SECTION:
cname.test-1.svc.cluster.local.		5	IN	CNAME	svc-1-a.test-1.svc.cluster.local.
svc-1-a.test-1.svc.cluster.local.		5	IN	A	10.0.0.100
svc-2-a.test-1.svc.cluster.local.		5	IN	A	10.0.0.101

;; AUTHORITY SECTION:
cluster.local.	303	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1502313310 7200 1800 86400 60

;; ADDITIONAL SECTION:
svc-1-a.test-1.svc.cluster.local.		5	IN	A	10.0.0.100
svc-2-a.test-1.svc.cluster.local.		5	IN	A	10.0.0.101

;; Query time: 4 msec
;; SERVER: 64.207.129.21#53(64.207.129.21)
;; WHEN: Thu Aug  7 16:49:35 2008
;; MSG SIZE  rcvd: 48`

	tcs := []Case{
		{
			Qname: "svc-1-a.test-1.svc.cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeNameError,
			Ns: []dns.RR{
				SOA("cluster.local.	303	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1502313310 7200 1800 86400 60"),
			},
		}, {
			Qname: "svc-1-a.test-1.svc.cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				CNAME("cname.test-1.svc.cluster.local.      303    IN      CNAME       svc-1-a.test-1.svc.cluster.local."),
				A("svc-1-a.test-1.svc.cluster.local.      303    IN      A       10.0.0.100"),
				A("svc-2-a.test-1.svc.cluster.local.      303    IN      A       10.0.0.101"),
			},
			Ns: []dns.RR{
				SOA("cluster.local.	303	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1502313310 7200 1800 86400 60"),
			},
			Extra: []dns.RR{
				A("svc-1-a.test-1.svc.cluster.local.      303    IN      A       10.0.0.100"),
				A("svc-2-a.test-1.svc.cluster.local.      303    IN      A       10.0.0.101"),
			},
		},
	}

	ms, err := ParseDigResponse(r)
	if err != nil {
		t.Fatalf("failed test: %s", err)
	}

	if len(ms) != 2 {
		t.Fatalf("failed test: got %v results, expected 2", len(ms))
	}

	SortAndCheck(t, ms[0], tcs[0])
	SortAndCheck(t, ms[1], tcs[1])

}
