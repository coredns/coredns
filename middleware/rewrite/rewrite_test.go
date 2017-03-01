package rewrite

import (
	"bytes"
	"testing"

	"github.com/coredns/coredns/middleware"
	"github.com/coredns/coredns/middleware/pkg/dnsrecorder"
	"github.com/coredns/coredns/middleware/test"

	"github.com/miekg/dns"
	"golang.org/x/net/context"
)

func msgPrinter(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	w.WriteMsg(r)
	return 0, nil
}

func TestNewRule(t *testing.T) {
	r, err := newRule()
	if err == nil {
		t.Errorf("Expected error but got success for newRule()")
	}
	r, err = newRule("foo")
	if err == nil {
		t.Errorf("Expected error but got success for newRule(foo)")
	}

	r, err = newRule("name")
	if err == nil {
		t.Errorf("Expected error for newRule(name)")
	}
	r, err = newRule("name", "a.com.")
	if err == nil {
		t.Errorf("Expected error for newRule(name, a.com)")
	}
	r, err = newRule("name", "a.com.", "b.com.", "c.com.")
	if err == nil {
		t.Errorf("Expected error for newRule(name, a.com, b.com, c.com)")
	}
	r, err = newRule("name", "a.com.", "b.com.")
	if err != nil {
		t.Errorf("Expected name rule but got error: %s", err)
	}
	if _, ok := r.(*nameRule); !ok {
		t.Errorf("Expected name rule but got %v", r)
	}

	r, err = newRule("type")
	if err == nil {
		t.Errorf("Expected error for newRule(type)")
	}
	r, err = newRule("type", "a")
	if err == nil {
		t.Errorf("Expected error for newRule(type, a)")
	}
	r, err = newRule("type", "any", "a", "a")
	if err == nil {
		t.Errorf("Expected error for newRule(type, any, a, a)")
	}
	r, err = newRule("type", "any", "a")
	if err != nil {
		t.Errorf("Expected type rule but got error: %s", err)
	}
	if _, ok := r.(*typeRule); !ok {
		t.Errorf("Expected type rule but got %v", r)
	}
	_, err = newRule("type", "XY", "WV")
	if err == nil {
		t.Errorf("Expected error but got success for invalid type")
	}
	_, err = newRule("type", "ANY", "WV")
	if err == nil {
		t.Errorf("Expected error but got success for invalid type")
	}

	r, err = newRule("class")
	if err == nil {
		t.Errorf("Expected error for newRule(class)")
	}
	r, err = newRule("class", "IN")
	if err == nil {
		t.Errorf("Expected error for newRule(class, IN)")
	}
	r, err = newRule("class", "ch", "in", "in")
	if err == nil {
		t.Errorf("Expected error for newRule(class, ch, in, in)")
	}
	r, err = newRule("class", "ch", "in")
	if err != nil {
		t.Errorf("Expected class rule but got error: %s", err)
	}
	if _, ok := r.(*classRule); !ok {
		t.Errorf("Expected class rule but got %v", r)
	}
	_, err = newRule("class", "XY", "WV")
	if err == nil {
		t.Errorf("Expected error but got success for invalid class")
	}
	_, err = newRule("class", "IN", "WV")
	if err == nil {
		t.Errorf("Expected error but got success for invalid class")
	}
}

func TestRewrite(t *testing.T) {
	rules := []Rule{}
	r, _ := newNameRule("from.nl.", "to.nl.")
	rules = append(rules, r)
	r, _ = newClassRule("CH", "IN")
	rules = append(rules, r)
	r, _ = newTypeRule("ANY", "HINFO")
	rules = append(rules, r)

	rw := Rewrite{
		Next:     middleware.HandlerFunc(msgPrinter),
		Rules:    rules,
		noRevert: true,
	}

	tests := []struct {
		from  string
		fromT uint16
		fromC uint16
		to    string
		toT   uint16
		toC   uint16
	}{
		{"from.nl.", dns.TypeA, dns.ClassINET, "to.nl.", dns.TypeA, dns.ClassINET},
		{"a.nl.", dns.TypeA, dns.ClassINET, "a.nl.", dns.TypeA, dns.ClassINET},
		{"a.nl.", dns.TypeA, dns.ClassCHAOS, "a.nl.", dns.TypeA, dns.ClassINET},
		{"a.nl.", dns.TypeANY, dns.ClassINET, "a.nl.", dns.TypeHINFO, dns.ClassINET},
		// name is rewritten, type is not.
		{"from.nl.", dns.TypeANY, dns.ClassINET, "to.nl.", dns.TypeANY, dns.ClassINET},
		// name is not, type is, but class is, because class is the 2nd rule.
		{"a.nl.", dns.TypeANY, dns.ClassCHAOS, "a.nl.", dns.TypeANY, dns.ClassINET},
	}

	ctx := context.TODO()
	for i, tc := range tests {
		m := new(dns.Msg)
		m.SetQuestion(tc.from, tc.fromT)
		m.Question[0].Qclass = tc.fromC

		rec := dnsrecorder.New(&test.ResponseWriter{})
		rw.ServeDNS(ctx, rec, m)

		resp := rec.Msg
		if resp.Question[0].Name != tc.to {
			t.Errorf("Test %d: Expected Name to be '%s' but was '%s'", i, tc.to, resp.Question[0].Name)
		}
		if resp.Question[0].Qtype != tc.toT {
			t.Errorf("Test %d: Expected Type to be '%d' but was '%d'", i, tc.toT, resp.Question[0].Qtype)
		}
		if resp.Question[0].Qclass != tc.toC {
			t.Errorf("Test %d: Expected Class to be '%d' but was '%d'", i, tc.toC, resp.Question[0].Qclass)
		}
	}
}

func TestRewriteEDNS0Local(t *testing.T) {

	rw := Rewrite{
		Next:     middleware.HandlerFunc(msgPrinter),
		noRevert: true,
	}

	tests := []struct {
		fromOpts []*dns.EDNS0_LOCAL
		action   string
		code     string
		data     string
		toOpts   []*dns.EDNS0_LOCAL
	}{
		{
			[]*dns.EDNS0_LOCAL{},
			"set",
			"0xffee",
			"0xabcdef",
			[]*dns.EDNS0_LOCAL{{0xffee, []byte{0xab, 0xcd, 0xef}}},
		},
		{
			[]*dns.EDNS0_LOCAL{},
			"append",
			"0xffee",
			"abcdefghijklmnop",
			[]*dns.EDNS0_LOCAL{{0xffee, []byte("abcdefghijklmnop")}},
		},
	}

	ctx := context.TODO()
	for i, tc := range tests {
		m := new(dns.Msg)
		m.SetQuestion("example.com.", dns.TypeA)
		m.Question[0].Qclass = dns.ClassINET

		r, err := newEdns0LocalRule(tc.action, tc.code, tc.data)
		if err != nil {
			t.Errorf("Error creating test rule: %s", err)
			continue
		}
		rw.Rules = []Rule{r}

		rec := dnsrecorder.New(&test.ResponseWriter{})
		rw.ServeDNS(ctx, rec, m)

		resp := rec.Msg
		o := resp.IsEdns0()
		if o == nil {
			t.Errorf("Test %d: EDNS0 options not set", i)
			continue
		}
		if !localOptsEqual(o.Option, tc.toOpts) {
			t.Errorf("Test %d: Expected %v but got %v", i, tc.toOpts, o)
		}
	}
}

func localOptsEqual(a []dns.EDNS0, b []*dns.EDNS0_LOCAL) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if l, ok := a[i].(*dns.EDNS0_LOCAL); ok {
			if l.Code != b[i].Code {
				return false
			}
			if !bytes.Equal(l.Data, b[i].Data) {
				return false
			}
		} else {
			return false
		}
	}
	return true
}

func TestRewriteEDNS0NSID(t *testing.T) {

	rw := Rewrite{
		Next:     middleware.HandlerFunc(msgPrinter),
		noRevert: true,
	}

	tests := []struct {
		fromOpts []*dns.EDNS0_NSID
		action   string
		nsid     string
		toOpts   []*dns.EDNS0_NSID
	}{
		{
			[]*dns.EDNS0_NSID{},
			"set",
			"abcdef",
			[]*dns.EDNS0_NSID{{dns.EDNS0NSID, ""}},
		},
		{
			[]*dns.EDNS0_NSID{},
			"append",
			"",
			[]*dns.EDNS0_NSID{{dns.EDNS0NSID, ""}},
		},
	}

	ctx := context.TODO()
	for i, tc := range tests {
		m := new(dns.Msg)
		m.SetQuestion("example.com.", dns.TypeA)
		m.Question[0].Qclass = dns.ClassINET

		r := &edns0NsidRule{tc.action}
		rw.Rules = []Rule{r}

		rec := dnsrecorder.New(&test.ResponseWriter{})
		rw.ServeDNS(ctx, rec, m)

		resp := rec.Msg
		o := resp.IsEdns0()
		if o == nil {
			t.Errorf("Test %d: EDNS0 options not set", i)
			continue
		}
		if !nsidOptsEqual(o.Option, tc.toOpts) {
			t.Errorf("Test %d: Expected %v but got %v", i, tc.toOpts, o)
		}
	}
}

func nsidOptsEqual(a []dns.EDNS0, b []*dns.EDNS0_NSID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if l, ok := a[i].(*dns.EDNS0_NSID); ok {
			if l.Nsid != b[i].Nsid {
				return false
			}
		} else {
			return false
		}
	}
	return true
}
