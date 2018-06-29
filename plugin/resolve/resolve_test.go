package resolve

import (
	"fmt"
	"net"
	"testing"

	"github.com/miekg/dns"
)

func TestName(t *testing.T) {
	t.Run("Name is 'resolve'", func(t *testing.T) {
		c := Resolve{}
		want := "resolve"
		if got := c.Name(); got != want {
			t.Errorf("Resolve.Name() = %v; want %v", got, want)
		}
	})
}

func TestAddTarget(t *testing.T) {
	type args struct {
		clientRR []dns.RR
		targetRR []dns.RR
	}
	tests := []struct {
		name     string
		args     args
		testFunc func([]dns.RR) error
	}{
		{
			name: "basic test",
			args: args{
				clientRR: []dns.RR{&dns.CNAME{Hdr: dns.RR_Header{Name: "cname", Rrtype: dns.TypeCNAME}, Target: "target."}},
				targetRR: []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: "target", Rrtype: dns.TypeA}, A: net.ParseIP("1.2.3.4")}},
			},
			testFunc: func(rr []dns.RR) error {
				if len(rr) != 2 {
					return fmt.Errorf("Expected 2 answers; got %v", len(rr))
				}
				if rr[0].Header().Rrtype != dns.TypeCNAME {
					t.Errorf("Expected 1st answer to be type %v; got %v", dns.TypeCNAME, rr[0].Header().Rrtype)
				}
				if rr[1].Header().Rrtype != dns.TypeA {
					t.Errorf("Expected 2nd answer to be type %v; got %v", dns.TypeA, rr[0].Header().Rrtype)
				}
				return nil
			},
		},
		{
			name: "do not add duplicate A",
			args: args{
				clientRR: []dns.RR{
					&dns.CNAME{Hdr: dns.RR_Header{Name: "cname", Rrtype: dns.TypeCNAME}, Target: "target."},
					&dns.A{Hdr: dns.RR_Header{Name: "target", Rrtype: dns.TypeA}, A: net.ParseIP("1.2.3.4")},
				},
				targetRR: []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: "target", Rrtype: dns.TypeA}, A: net.ParseIP("1.2.3.4")}},
			},
			testFunc: func(rr []dns.RR) error {
				if len(rr) != 2 {
					return fmt.Errorf("Expected 2 answers; got %v", len(rr))
				}
				if rr[0].Header().Rrtype != dns.TypeCNAME {
					t.Errorf("Expected 1st answer to be type %v; got %v", dns.TypeCNAME, rr[0].Header().Rrtype)
				}
				if rr[1].Header().Rrtype != dns.TypeA {
					t.Errorf("Expected 2nd answer to be type %v; got %v", dns.TypeA, rr[0].Header().Rrtype)
				}
				return nil
			},
		},
		{
			name: "do not add duplicate AAAA",
			args: args{
				clientRR: []dns.RR{
					&dns.CNAME{Hdr: dns.RR_Header{Name: "cname", Rrtype: dns.TypeCNAME}, Target: "target."},
					&dns.AAAA{Hdr: dns.RR_Header{Name: "target", Rrtype: dns.TypeAAAA}, AAAA: net.ParseIP("::1:2:3:4")},
				},
				targetRR: []dns.RR{&dns.AAAA{Hdr: dns.RR_Header{Name: "target", Rrtype: dns.TypeAAAA}, AAAA: net.ParseIP("::1:2:3:4")}},
			},
			testFunc: func(rr []dns.RR) error {
				if len(rr) != 2 {
					return fmt.Errorf("Expected 2 answers; got %v", len(rr))
				}
				if rr[0].Header().Rrtype != dns.TypeCNAME {
					t.Errorf("Expected 1st answer to be type %v; got %v", dns.TypeCNAME, rr[0].Header().Rrtype)
				}
				if rr[1].Header().Rrtype != dns.TypeAAAA {
					t.Errorf("Expected 2nd answer to be type %v; got %v", dns.TypeAAAA, rr[0].Header().Rrtype)
				}
				return nil
			},
		},
		{
			name: "do not add duplicate SRV",
			args: args{
				clientRR: []dns.RR{
					&dns.CNAME{Hdr: dns.RR_Header{Name: "cname", Rrtype: dns.TypeCNAME}, Target: "target."},
					&dns.SRV{Hdr: dns.RR_Header{Name: "target", Rrtype: dns.TypeSRV}, Target: "srv."},
				},
				targetRR: []dns.RR{&dns.SRV{Hdr: dns.RR_Header{Name: "target", Rrtype: dns.TypeSRV}, Target: "srv."}},
			},
			testFunc: func(rr []dns.RR) error {
				if len(rr) != 2 {
					return fmt.Errorf("Expected 2 answers; got %v", len(rr))
				}
				if rr[0].Header().Rrtype != dns.TypeCNAME {
					t.Errorf("Expected 1st answer to be type %v; got %v", dns.TypeCNAME, rr[0].Header().Rrtype)
				}
				if rr[1].Header().Rrtype != dns.TypeSRV {
					t.Errorf("Expected 2nd answer to be type %v; got %v", dns.TypeSRV, rr[0].Header().Rrtype)
				}
				return nil
			},
		},
		{
			name: "do not add duplicate other types",
			args: args{
				clientRR: []dns.RR{
					&dns.CNAME{Hdr: dns.RR_Header{Name: "cname", Rrtype: dns.TypeCNAME}, Target: "target."},
					&dns.TXT{Hdr: dns.RR_Header{Name: "target", Rrtype: dns.TypeTXT}, Txt: []string{"txt"}},
				},
				targetRR: []dns.RR{&dns.TXT{Hdr: dns.RR_Header{Name: "target", Rrtype: dns.TypeTXT}, Txt: []string{"txt"}}},
			},
			testFunc: func(rr []dns.RR) error {
				if len(rr) != 2 {
					return fmt.Errorf("Expected 2 answers; got %v", len(rr))
				}
				if rr[0].Header().Rrtype != dns.TypeCNAME {
					t.Errorf("Expected 1st answer to be type %v; got %v", dns.TypeCNAME, rr[0].Header().Rrtype)
				}
				if rr[1].Header().Rrtype != dns.TypeTXT {
					t.Errorf("Expected 2nd answer to be type %v; got %v", dns.TypeTXT, rr[0].Header().Rrtype)
				}
				return nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.args.clientRR = addTarget(tt.args.clientRR, tt.args.targetRR)
			err := tt.testFunc(tt.args.clientRR)
			if err != nil {
				t.Fatal(err.Error())
			}
		})
	}
}
