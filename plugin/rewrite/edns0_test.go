package rewrite

import (
	"context"
	"net"
	"reflect"
	"testing"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestEdns0Rewrite(t *testing.T) {
	tests := []struct {
		ruleArgs []string
		reqOPT   *dns.OPT
		expOPT   *dns.OPT
	}{
		//NSID tests
		{
			[]string{"nsid", "set"}, nil, nil,
		},
		{
			[]string{"nsid", "set"},
			newOpt(nil),
			newOpt([]dns.EDNS0{&dns.EDNS0_NSID{Code: dns.EDNS0NSID, Nsid: ""}}),
		},
		{
			[]string{"nsid", "append"},
			newOpt(nil),
			newOpt([]dns.EDNS0{&dns.EDNS0_NSID{Code: dns.EDNS0NSID, Nsid: ""}}),
		},
		{
			[]string{"nsid", "replace"},
			newOpt(nil),
			newOpt(nil),
		},
		{
			[]string{"nsid", "set"},
			newOpt([]dns.EDNS0{&dns.EDNS0_NSID{Code: dns.EDNS0NSID, Nsid: "??"}}),
			newOpt([]dns.EDNS0{&dns.EDNS0_NSID{Code: dns.EDNS0NSID, Nsid: ""}}),
		},
		{
			[]string{"nsid", "append"},
			newOpt([]dns.EDNS0{&dns.EDNS0_NSID{Code: dns.EDNS0NSID, Nsid: "??"}}),
			newOpt([]dns.EDNS0{&dns.EDNS0_NSID{Code: dns.EDNS0NSID, Nsid: "??"}}),
		},
		{
			[]string{"nsid", "replace"},
			newOpt([]dns.EDNS0{&dns.EDNS0_NSID{Code: dns.EDNS0NSID, Nsid: "??"}}),
			newOpt([]dns.EDNS0{&dns.EDNS0_NSID{Code: dns.EDNS0NSID, Nsid: ""}}),
		},

		//Subnet tests
		{
			[]string{"subnet", "set", "24", "56"}, nil, nil,
		},
		{
			[]string{"subnet", "set", "24", "56"},
			newOpt(nil),
			newOpt([]dns.EDNS0{&dns.EDNS0_SUBNET{
				Code: dns.EDNS0SUBNET, Family: 1, SourceNetmask: 24, SourceScope: 0,
				Address: net.ParseIP("10.240.0.0").To4(),
			}}),
		},
		{
			[]string{"subnet", "append", "24", "56"},
			newOpt(nil),
			newOpt([]dns.EDNS0{&dns.EDNS0_SUBNET{
				Code: dns.EDNS0SUBNET, Family: 1, SourceNetmask: 24, SourceScope: 0,
				Address: net.ParseIP("10.240.0.0").To4(),
			}}),
		},
		{
			[]string{"subnet", "replace", "24", "56"},
			newOpt(nil),
			newOpt(nil),
		},
		{
			[]string{"subnet", "set", "24", "56"},
			newOpt([]dns.EDNS0{&dns.EDNS0_SUBNET{
				Code: dns.EDNS0SUBNET, Family: 1, SourceNetmask: 28, SourceScope: 0,
				Address: net.ParseIP("192.0.0.0").To4(),
			}}),
			newOpt([]dns.EDNS0{&dns.EDNS0_SUBNET{
				Code: dns.EDNS0SUBNET, Family: 1, SourceNetmask: 24, SourceScope: 0,
				Address: net.ParseIP("10.240.0.0").To4(),
			}}),
		},
		{
			[]string{"subnet", "set", "24", "56"},
			newOpt([]dns.EDNS0{&dns.EDNS0_SUBNET{
				Code: dns.EDNS0SUBNET, Family: 1, SourceNetmask: 8, SourceScope: 0,
				Address: net.ParseIP("192.0.0.0").To4(),
			}}),
			newOpt([]dns.EDNS0{&dns.EDNS0_SUBNET{
				Code: dns.EDNS0SUBNET, Family: 1, SourceNetmask: 8, SourceScope: 0,
				Address: net.ParseIP("10.0.0.0").To4(),
			}}),
		},
		{
			[]string{"subnet", "append", "24", "56"},
			newOpt([]dns.EDNS0{&dns.EDNS0_SUBNET{
				Code: dns.EDNS0SUBNET, Family: 1, SourceNetmask: 8, SourceScope: 0,
				Address: net.ParseIP("192.0.0.0").To4(),
			}}),
			newOpt([]dns.EDNS0{&dns.EDNS0_SUBNET{
				Code: dns.EDNS0SUBNET, Family: 1, SourceNetmask: 8, SourceScope: 0,
				Address: net.ParseIP("192.0.0.0").To4(),
			}}),
		},
		{
			[]string{"subnet", "replace", "24", "56"},
			newOpt([]dns.EDNS0{&dns.EDNS0_SUBNET{
				Code: dns.EDNS0SUBNET, Family: 1, SourceNetmask: 28, SourceScope: 0,
				Address: net.ParseIP("192.0.0.0").To4(),
			}}),
			newOpt([]dns.EDNS0{&dns.EDNS0_SUBNET{
				Code: dns.EDNS0SUBNET, Family: 1, SourceNetmask: 24, SourceScope: 0,
				Address: net.ParseIP("10.240.0.0").To4(),
			}}),
		},
		{
			[]string{"subnet", "replace", "24", "56"},
			newOpt([]dns.EDNS0{&dns.EDNS0_SUBNET{
				Code: dns.EDNS0SUBNET, Family: 1, SourceNetmask: 8, SourceScope: 0,
				Address: net.ParseIP("192.0.0.0").To4(),
			}}),
			newOpt([]dns.EDNS0{&dns.EDNS0_SUBNET{
				Code: dns.EDNS0SUBNET, Family: 1, SourceNetmask: 8, SourceScope: 0,
				Address: net.ParseIP("10.0.0.0").To4(),
			}}),
		},

		//Local (predefined) tests
		{
			[]string{"local", "set", "0xfff1", "1234"}, nil, nil,
		},
		{
			[]string{"local", "set", "0xfff1", "1234"},
			newOpt(nil),
			newOpt([]dns.EDNS0{&dns.EDNS0_LOCAL{Code: 0xfff1, Data: []byte("1234")}}),
		},
		{
			[]string{"local", "append", "0xfff1", "1234"},
			newOpt(nil),
			newOpt([]dns.EDNS0{&dns.EDNS0_LOCAL{Code: 0xfff1, Data: []byte("1234")}}),
		},
		{
			[]string{"local", "replace", "0xfff1", "1234"},
			newOpt(nil),
			newOpt(nil),
		},
		{
			[]string{"local", "set", "0xfff1", "1234"},
			newOpt([]dns.EDNS0{&dns.EDNS0_LOCAL{Code: 0xfff1, Data: []byte("ABCD")}}),
			newOpt([]dns.EDNS0{&dns.EDNS0_LOCAL{Code: 0xfff1, Data: []byte("1234")}}),
		},
		{
			[]string{"local", "append", "0xfff1", "1234"},
			newOpt([]dns.EDNS0{&dns.EDNS0_LOCAL{Code: 0xfff1, Data: []byte("ABCD")}}),
			newOpt([]dns.EDNS0{&dns.EDNS0_LOCAL{Code: 0xfff1, Data: []byte("ABCD")}}),
		},
		{
			[]string{"local", "replace", "0xfff1", "1234"},
			newOpt([]dns.EDNS0{&dns.EDNS0_LOCAL{Code: 0xfff1, Data: []byte("ABCD")}}),
			newOpt([]dns.EDNS0{&dns.EDNS0_LOCAL{Code: 0xfff1, Data: []byte("1234")}}),
		},

		//Local (variable) tests
		{
			[]string{"local", "set", "0xfff1", "{qname}"}, nil, nil,
		},
		{
			[]string{"local", "set", "0xfff1", "{qname}"},
			newOpt(nil),
			newOpt([]dns.EDNS0{&dns.EDNS0_LOCAL{Code: 0xfff1, Data: []byte("example.com")}}),
		},
		{
			[]string{"local", "append", "0xfff1", "{qname}"},
			newOpt(nil),
			newOpt([]dns.EDNS0{&dns.EDNS0_LOCAL{Code: 0xfff1, Data: []byte("example.com")}}),
		},
		{
			[]string{"local", "replace", "0xfff1", "{qname}"},
			newOpt(nil),
			newOpt(nil),
		},
		{
			[]string{"local", "set", "0xfff1", "{qname}"},
			newOpt([]dns.EDNS0{&dns.EDNS0_LOCAL{Code: 0xfff1, Data: []byte("ABCD")}}),
			newOpt([]dns.EDNS0{&dns.EDNS0_LOCAL{Code: 0xfff1, Data: []byte("example.com")}}),
		},
		{
			[]string{"local", "append", "0xfff1", "{qname}"},
			newOpt([]dns.EDNS0{&dns.EDNS0_LOCAL{Code: 0xfff1, Data: []byte("ABCD")}}),
			newOpt([]dns.EDNS0{&dns.EDNS0_LOCAL{Code: 0xfff1, Data: []byte("ABCD")}}),
		},
		{
			[]string{"local", "replace", "0xfff1", "{qname}"},
			newOpt([]dns.EDNS0{&dns.EDNS0_LOCAL{Code: 0xfff1, Data: []byte("ABCD")}}),
			newOpt([]dns.EDNS0{&dns.EDNS0_LOCAL{Code: 0xfff1, Data: []byte("example.com")}}),
		},
	}

	rw := Rewrite{Next: plugin.HandlerFunc(msgPrinter)}

	ctx := context.TODO()
	for i, tc := range tests {
		r, e := newEdns0Rule("stop", tc.ruleArgs...)
		if e != nil {
			t.Errorf("Test %d: failed to create rule: %s", i, e)
			continue
		}
		rw.Rules = []Rule{r}

		m := new(dns.Msg)
		m.SetQuestion("example.com", dns.TypeA)
		if tc.reqOPT != nil {
			m.Extra = []dns.RR{tc.reqOPT}
		}

		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		rw.ServeDNS(ctx, rec, m)
		resp := rec.Msg
		respOpt := resp.IsEdns0()

		if tc.expOPT == nil {
			if respOpt != nil {
				t.Errorf("Test %d: unexpected OPT record in response %v", i, respOpt)
			}
			continue
		}
		if respOpt == nil {
			t.Errorf("Test %d: not found OPT record in response", i)
			continue
		}
		if !reflect.DeepEqual(tc.expOPT.Option, respOpt.Option) {
			t.Errorf("Test %d: unexpected options, expected %v, got %v", i, tc.expOPT.Option, respOpt.Option)
			for i, a := range respOpt.Option {
				if s, ok := a.(*dns.EDNS0_SUBNET); ok {
					t.Logf("Exp option %d = %#v", i, *s)
					e := tc.expOPT.Option[i].(*dns.EDNS0_SUBNET)
					t.Logf("Act option %d = %#v", i, *e)
				}
			}
		}
	}
}

func newOpt(opt []dns.EDNS0) *dns.OPT {
	e := &dns.OPT{Hdr: dns.RR_Header{Name: ".", Rrtype: dns.TypeOPT}}
	e.SetUDPSize(4096)
	e.Option = opt
	return e
}
