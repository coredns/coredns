package kubernetes

import (
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"golang.org/x/net/context"

	"github.com/miekg/dns"
)

/*
func TestKubernetesTransfer(t *testing.T) {
	k := New([]string{"cluster.local."})
	k.APIConn = &APIConnServeTest{}

	state := request.Request{Zone: "cluster.local.", Req: new(dns.Msg)}

	for service := range k.Transfer(state) {
		what, _ := service.HostType()
		t.Logf("%v, %s, %s", service, dns.TypeToString[what], msg.Domain(service.Key))
	}
}
*/

func TestKubernetesXFR(t *testing.T) {
	k := New([]string{"cluster.local."})
	k.APIConn = &APIConnServeTest{}
	k.TransferTo = []string{"127.0.0.1"}

	ctx := context.TODO()
	w := dnstest.NewRecorder(&test.ResponseWriter{})
	dnsmsg := &dns.Msg{}
	dnsmsg.SetAxfr(k.Zones[0])

	_, err := k.ServeDNS(ctx, w, dnsmsg)
	if err != nil {
		t.Error(err)
	}

	if w.ReadMsg() == nil {
		t.Logf("%+v\n", w)
		t.Error("Did not get back a zone response")
	}

	for _, resp := range w.ReadMsg().Answer {
		if resp.Header().Rrtype == dns.TypeSOA {
			continue
		}

		found := false
		recs := []string{}

		for _, tc := range dnsTestCases {
			// Skip failures
			if tc.Rcode != dns.RcodeSuccess {
				continue
			}
			for _, ans := range tc.Answer {
				if resp.String() == ans.String() {
					found = true
					break
				}
				recs = append(recs, ans.String())
			}
			if found {
				break
			}
		}
		if !found {
			t.Errorf("Got back a RR we shouldnt have %+v\n%+v\n", resp, strings.Join(recs, "\n"))
		}
	}
	if w.ReadMsg().Answer[0].Header().Rrtype != dns.TypeSOA {
		t.Error("Invalid XFR, does not start with SOA record")
	}
	if w.ReadMsg().Answer[len(w.ReadMsg().Answer)-1].Header().Rrtype != dns.TypeSOA {
		t.Error("Invalid XFR, does not end with SOA record")
	}
}
