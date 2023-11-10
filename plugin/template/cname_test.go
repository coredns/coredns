package template

import (
	"context"
	"regexp"
	"testing"
	gotmpl "text/template"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

func TestTruncatedCNAME(t *testing.T) {
	up := &Upstub{
		Qclass:    dns.ClassINET,
		Truncated: true,
		Case: test.Case{
			Qname: "cname.test.",
			Qtype: dns.TypeA,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.CNAME("cname.test. 600 IN CNAME test.up"),
				test.A("test.up. 600 IN A 1.2.3.4"),
			},
		},
	}

	handler := Handler{
		Zones: []string{"."},
		Templates: []template{{
			regex:    []*regexp.Regexp{regexp.MustCompile(`^cname\.test\.$`)},
			answer:   []*gotmpl.Template{gotmpl.Must(gotmpl.New("answer").Parse(up.Answer[0].String()))},
			qclass:   dns.ClassINET,
			qtype:    dns.TypeA,
			zones:    []string{"test."},
			upstream: up,
		}},
	}

	r := &dns.Msg{Question: []dns.Question{{Name: up.Qname, Qclass: up.Qclass, Qtype: up.Qtype}}}
	w := dnstest.NewRecorder(&test.ResponseWriter{})

	_, err := handler.ServeDNS(context.TODO(), w, r)

	if err != nil {
		t.Fatalf("Unexpected error %q", err)
	}
	if w.Msg == nil {
		t.Fatalf("Unexpected empty response.")
	}
	if !w.Msg.Truncated {
		t.Error("Expected reply to be marked truncated.")
	}
	err = test.SortAndCheck(w.Msg, up.Case)
	if err != nil {
		t.Error(err)
	}
}

// Upstub implements an Upstreamer that returns a set response for test purposes
type Upstub struct {
	test.Case
	Truncated bool
	Qclass    uint16
}

// Lookup returns a set response
func (t *Upstub) Lookup(ctx context.Context, state request.Request, name string, typ uint16) (*dns.Msg, error) {
	var answer []dns.RR
	// if query type is not CNAME, remove any CNAME with same name as qname from the answer
	if t.Qtype != dns.TypeCNAME {
		for _, a := range t.Answer {
			if c, ok := a.(*dns.CNAME); ok && c.Header().Name == t.Qname {
				continue
			}
			answer = append(answer, a)
		}
	} else {
		answer = t.Answer
	}

	return &dns.Msg{
		MsgHdr: dns.MsgHdr{
			Response:  true,
			Truncated: t.Truncated,
			Rcode:     t.Rcode,
		},
		Question: []dns.Question{{Name: t.Qname, Qtype: t.Qtype, Qclass: t.Qclass}},
		Answer:   answer,
		Extra:    t.Extra,
		Ns:       t.Ns,
	}, nil
}
