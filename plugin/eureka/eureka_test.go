package eureka

import (
	"context"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/fall"
	"github.com/coredns/coredns/plugin/test"
	"github.com/miekg/dns"
	"testing"
	"time"
)

type fakeEureka struct {
	clientAPI
}

func (e fakeEureka) fetchAllApplications() (*applications, error) {
	return &applications{
		Application: []*application{
			{
				// 1 instance
				Name: "test1",
				Instance: []*instance{
					{
						Status:     statusUp,
						IpAddr:     "1.2.3.4",
						VipAddress: "vip1",
					},
				},
			},
			{
				// 2 instances
				Name: "test2",
				Instance: []*instance{
					{
						Status:     statusUp,
						IpAddr:     "1.2.3.4",
						VipAddress: "vip2",
					},
					{
						Status:     statusUp,
						IpAddr:     "1.2.3.5",
						VipAddress: "vip2",
					},
				},
			},
			{
				// 1 UP instance and 1 DOWN instance
				Name: "test3",
				Instance: []*instance{
					{
						Status:     statusUp,
						IpAddr:     "1.2.3.4",
						VipAddress: "vip3",
					},
					{
						Status:     statusDown,
						IpAddr:     "1.2.3.5",
						VipAddress: "vip3",
					},
				},
			},
			{
				// test4 and test5 share the same VIP
				Name: "test4",
				Instance: []*instance{
					{
						Status:     statusUp,
						IpAddr:     "1.2.3.4",
						VipAddress: "vip4",
					},
				},
			},
			{
				// test4 and test5 share the same VIP
				Name: "test5",
				Instance: []*instance{
					{
						Status:     statusUp,
						IpAddr:     "1.2.3.5",
						VipAddress: "vip4",
					},
				},
			},
			{
				// test6 have two different VIPs
				Name: "test6",
				Instance: []*instance{
					{
						Status:     statusUp,
						IpAddr:     "1.2.3.4",
						VipAddress: "vip6-1",
					},
					{
						Status:     statusUp,
						IpAddr:     "1.2.3.5",
						VipAddress: "vip6-2",
					},
				},
			},
		},
	}, nil
}

func TestEureka(t *testing.T) {
	testEurekaWithMode(t, modeApp, appTests)
	testEurekaWithMode(t, modeVip, vipTests)
}

func testEurekaWithMode(t *testing.T, mode mode, tests []test.Case) {
	ctx := context.Background()
	e, err := New(ctx, &options{
		refresh: time.Minute,
		ttl:     30,
		mode:    mode,
	}, fakeEureka{})
	if err != nil {
		t.Fatalf("Failed to create Eureka: %v", err)
	}
	e.Zones = []string{"example.com."}
	e.Next = test.NextHandler(dns.RcodeNameError, nil)
	e.Fall = fall.Zero

	err = e.Run(ctx)
	if err != nil {
		t.Fatalf("Failed to initialize Eureka: %v", err)
	}

	for i, tc := range tests {
		r := tc.Msg()
		w := dnstest.NewRecorder(&test.ResponseWriter{})

		code, err := e.ServeDNS(ctx, w, r)
		if err != tc.Error {
			t.Errorf("Test %d expected no error, got %v", i, err)
			return
		}
		if tc.Error != nil {
			continue
		}
		if code != tc.Rcode {
			t.Errorf("Test %d expected code %v, got %v", i, tc.Rcode, code)
			return
		}

		if code == dns.RcodeSuccess {
			resp := w.Msg
			if resp == nil {
				t.Fatalf("Test %d, got nil message and no error for %q", i, r.Question[0].Name)
			}
			if err = test.SortAndCheck(resp, tc); err != nil {
				t.Error(err)
			}
		}

	}
}

var commonTests = []test.Case{
	{
		// Unsupported query type
		Qname: "test.example.com.", Qtype: dns.TypeAAAA, Rcode: dns.RcodeNameError,
	},
	{
		// non-existing record
		Qname: "non-existing.example.com.", Qtype: dns.TypeA, Rcode: dns.RcodeSuccess,
	},
	{
		// non-existing zone
		Qname: "test1.example.net.", Qtype: dns.TypeA, Rcode: dns.RcodeNameError,
	},
}

var appTests = append(commonTests, []test.Case{
	{
		Qname: "test1.example.com.", Qtype: dns.TypeA, Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("test1.example.com.	30	IN	A	1.2.3.4"),
		},
	},
	{
		Qname: "test2.example.com.", Qtype: dns.TypeA, Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("test2.example.com.	30	IN	A	1.2.3.4"),
			test.A("test2.example.com.	30	IN	A	1.2.3.5"),
		},
	},
	{
		Qname: "test3.example.com.", Qtype: dns.TypeA, Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("test3.example.com.	30	IN	A	1.2.3.4"),
		},
	},
}...)

var vipTests = append(commonTests, []test.Case{
	{
		Qname: "vip1.example.com.", Qtype: dns.TypeA, Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("vip1.example.com.	30	IN	A	1.2.3.4"),
		},
	},
	{
		Qname: "vip2.example.com.", Qtype: dns.TypeA, Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("vip2.example.com.	30	IN	A	1.2.3.4"),
			test.A("vip2.example.com.	30	IN	A	1.2.3.5"),
		},
	},
	{
		Qname: "vip3.example.com.", Qtype: dns.TypeA, Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("vip3.example.com.	30	IN	A	1.2.3.4"),
		},
	},
	{
		Qname: "vip4.example.com.", Qtype: dns.TypeA, Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("vip4.example.com.	30	IN	A	1.2.3.4"),
			test.A("vip4.example.com.	30	IN	A	1.2.3.5"),
		},
	},
	{
		Qname: "vip6-1.example.com.", Qtype: dns.TypeA, Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("vip6-1.example.com.	30	IN	A	1.2.3.4"),
		},
	},
	{
		Qname: "vip6-2.example.com.", Qtype: dns.TypeA, Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("vip6-2.example.com.	30	IN	A	1.2.3.5"),
		},
	},
	{
		Qname: "test.example.com.", Qtype: dns.TypeAAAA, Rcode: dns.RcodeNameError,
	},
}...)
