package forward

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/proxy"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestSetup(t *testing.T) {
	tests := []struct {
		input           string
		shouldErr       bool
		expectedFrom    string
		expectedIgnored []string
		expectedFails   uint32
		expectedOpts    proxy.Options
		expectedErr     string
	}{
		// positive
		{"forward . 127.0.0.1", false, ".", nil, 2, proxy.Options{HCRecursionDesired: true, HCDomain: "."}, ""},
		{"forward . 127.0.0.1 {\nhealth_check 0.5s domain example.org\n}\n", false, ".", nil, 2, proxy.Options{HCRecursionDesired: true, HCDomain: "example.org."}, ""},
		{"forward . 127.0.0.1 {\nexcept miek.nl\n}\n", false, ".", nil, 2, proxy.Options{HCRecursionDesired: true, HCDomain: "."}, ""},
		{"forward . 127.0.0.1 {\nmax_fails 3\n}\n", false, ".", nil, 3, proxy.Options{HCRecursionDesired: true, HCDomain: "."}, ""},
		{"forward . 127.0.0.1 {\nforce_tcp\n}\n", false, ".", nil, 2, proxy.Options{ForceTCP: true, HCRecursionDesired: true, HCDomain: "."}, ""},
		{"forward . 127.0.0.1 {\nprefer_udp\n}\n", false, ".", nil, 2, proxy.Options{PreferUDP: true, HCRecursionDesired: true, HCDomain: "."}, ""},
		{"forward . 127.0.0.1 {\nforce_tcp\nprefer_udp\n}\n", false, ".", nil, 2, proxy.Options{PreferUDP: true, ForceTCP: true, HCRecursionDesired: true, HCDomain: "."}, ""},
		{"forward . 127.0.0.1:53", false, ".", nil, 2, proxy.Options{HCRecursionDesired: true, HCDomain: "."}, ""},
		{"forward . 127.0.0.1:8080", false, ".", nil, 2, proxy.Options{HCRecursionDesired: true, HCDomain: "."}, ""},
		{"forward . [::1]:53", false, ".", nil, 2, proxy.Options{HCRecursionDesired: true, HCDomain: "."}, ""},
		{"forward . [2003::1]:53", false, ".", nil, 2, proxy.Options{HCRecursionDesired: true, HCDomain: "."}, ""},
		{"forward . 127.0.0.1 \n", false, ".", nil, 2, proxy.Options{HCRecursionDesired: true, HCDomain: "."}, ""},
		{"forward 10.9.3.0/18 127.0.0.1", false, "0.9.10.in-addr.arpa.", nil, 2, proxy.Options{HCRecursionDesired: true, HCDomain: "."}, ""},
		{`forward . ::1
		forward com ::2`, false, ".", nil, 2, proxy.Options{HCRecursionDesired: true, HCDomain: "."}, "plugin"},
		// negative
		{"forward . a27.0.0.1", true, "", nil, 0, proxy.Options{HCRecursionDesired: true, HCDomain: "."}, "not an IP"},
		{"forward . 127.0.0.1 {\nblaatl\n}\n", true, "", nil, 0, proxy.Options{HCRecursionDesired: true, HCDomain: "."}, "unknown property"},
		{"forward . 127.0.0.1 {\nhealth_check 0.5s domain\n}\n", true, "", nil, 0, proxy.Options{HCRecursionDesired: true, HCDomain: "."}, "Wrong argument count or unexpected line ending after 'domain'"},
		{"forward . https://127.0.0.1 \n", true, ".", nil, 2, proxy.Options{HCRecursionDesired: true, HCDomain: "."}, "'https' is not supported as a destination protocol in forward: https://127.0.0.1"},
		{"forward xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx 127.0.0.1 \n", true, ".", nil, 2, proxy.Options{HCRecursionDesired: true, HCDomain: "."}, "unable to normalize 'xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx'"},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		fs, err := parseForward(c)

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: expected error but found %s for input %s", i, err, test.input)
		}

		if err != nil {
			if !test.shouldErr {
				t.Fatalf("Test %d: expected no error but found one for input %s, got: %v", i, test.input, err)
			}

			if !strings.Contains(err.Error(), test.expectedErr) {
				t.Errorf("Test %d: expected error to contain: %v, found error: %v, input: %s", i, test.expectedErr, err, test.input)
			}
		}

		if !test.shouldErr {
			f := fs[0]
			if f.from != test.expectedFrom {
				t.Errorf("Test %d: expected: %s, got: %s", i, test.expectedFrom, f.from)
			}
			if test.expectedIgnored != nil {
				if !reflect.DeepEqual(f.ignored, test.expectedIgnored) {
					t.Errorf("Test %d: expected: %q, actual: %q", i, test.expectedIgnored, f.ignored)
				}
			}
			if f.maxfails != test.expectedFails {
				t.Errorf("Test %d: expected: %d, got: %d", i, test.expectedFails, f.maxfails)
			}
			if f.opts != test.expectedOpts {
				t.Errorf("Test %d: expected: %v, got: %v", i, test.expectedOpts, f.opts)
			}
		}
	}
}

func TestSetupTLS(t *testing.T) {
	tests := []struct {
		input              string
		shouldErr          bool
		expectedServerName string
		expectedErr        string
	}{
		// positive
		{`forward . tls://127.0.0.1 {
				tls_servername dns
			}`, false, "dns", ""},
		{`forward . 127.0.0.1 {
				tls_servername dns
			}`, false, "", ""},
		{`forward . 127.0.0.1 {
				tls
			}`, false, "", ""},
		{`forward . tls://127.0.0.1`, false, "", ""},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		fs, err := parseForward(c)
		f := fs[0]

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: expected error but found %s for input %s", i, err, test.input)
		}

		if err != nil {
			if !test.shouldErr {
				t.Errorf("Test %d: expected no error but found one for input %s, got: %v", i, test.input, err)
			}

			if !strings.Contains(err.Error(), test.expectedErr) {
				t.Errorf("Test %d: expected error to contain: %v, found error: %v, input: %s", i, test.expectedErr, err, test.input)
			}
		}

		if !test.shouldErr && test.expectedServerName != "" && test.expectedServerName != f.tlsConfig.ServerName {
			t.Errorf("Test %d: expected: %q, actual: %q", i, test.expectedServerName, f.tlsConfig.ServerName)
		}

		if !test.shouldErr && test.expectedServerName != "" && test.expectedServerName != f.proxies[0].GetHealthchecker().GetTLSConfig().ServerName {
			t.Errorf("Test %d: expected: %q, actual: %q", i, test.expectedServerName, f.proxies[0].GetHealthchecker().GetTLSConfig().ServerName)
		}
	}
}

func TestSetupResolvconf(t *testing.T) {
	const resolv = "resolv.conf"
	if err := os.WriteFile(resolv,
		[]byte(`nameserver 10.10.255.252
nameserver 10.10.255.253`), 0666); err != nil {
		t.Fatalf("Failed to write resolv.conf file: %s", err)
	}
	defer os.Remove(resolv)

	const resolvIPV6 = "resolv-ipv6.conf"
	if err := os.WriteFile(resolvIPV6,
		[]byte(`nameserver 0388:d254:7aec:6892:9f7f:e93b:5806:1b0f%en0`), 0666); err != nil {
		t.Fatalf("Failed to write %v file: %s", resolvIPV6, err)
	}
	defer os.Remove(resolvIPV6)

	tests := []struct {
		input         string
		shouldErr     bool
		expectedErr   string
		expectedNames []string
	}{
		// pass
		{`forward . ` + resolv, false, "", []string{"10.10.255.252:53", "10.10.255.253:53"}},
		// fail
		{`forward . /dev/null`, true, "no nameservers", nil},
		// IPV6 with local zone
		{`forward . ` + resolvIPV6, false, "", []string{"[0388:d254:7aec:6892:9f7f:e93b:5806:1b0f]:53"}},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		fs, err := parseForward(c)

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: expected error but found %s for input %s", i, err, test.input)
			continue
		}

		if err != nil {
			if !test.shouldErr {
				t.Errorf("Test %d: expected no error but found one for input %s, got: %v", i, test.input, err)
			}

			if !strings.Contains(err.Error(), test.expectedErr) {
				t.Errorf("Test %d: expected error to contain: %v, found error: %v, input: %s", i, test.expectedErr, err, test.input)
			}
		}

		if test.shouldErr {
			continue
		}

		f := fs[0]
		for j, n := range test.expectedNames {
			addr := f.proxies[j].Addr()
			if n != addr {
				t.Errorf("Test %d, expected %q, got %q", j, n, addr)
			}
		}

		for _, p := range f.proxies {
			p.Healthcheck() // this should almost always err, we don't care it shouldn't crash
		}
	}
}

func TestSetupMaxConcurrent(t *testing.T) {
	tests := []struct {
		input       string
		shouldErr   bool
		expectedVal int64
		expectedErr string
	}{
		// positive
		{"forward . 127.0.0.1 {\nmax_concurrent 1000\n}\n", false, 1000, ""},
		// negative
		{"forward . 127.0.0.1 {\nmax_concurrent many\n}\n", true, 0, "invalid"},
		{"forward . 127.0.0.1 {\nmax_concurrent -4\n}\n", true, 0, "negative"},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		fs, err := parseForward(c)

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: expected error but found %s for input %s", i, err, test.input)
		}

		if err != nil {
			if !test.shouldErr {
				t.Errorf("Test %d: expected no error but found one for input %s, got: %v", i, test.input, err)
			}

			if !strings.Contains(err.Error(), test.expectedErr) {
				t.Errorf("Test %d: expected error to contain: %v, found error: %v, input: %s", i, test.expectedErr, err, test.input)
			}
		}

		if test.shouldErr {
			continue
		}
		f := fs[0]
		if f.maxConcurrent != test.expectedVal {
			t.Errorf("Test %d: expected: %d, got: %d", i, test.expectedVal, f.maxConcurrent)
		}
	}
}

func TestSetupHealthCheck(t *testing.T) {
	tests := []struct {
		input          string
		shouldErr      bool
		expectedRecVal bool
		expectedDomain string
		expectedErr    string
	}{
		// positive
		{"forward . 127.0.0.1\n", false, true, ".", ""},
		{"forward . 127.0.0.1 {\nhealth_check 0.5s\n}\n", false, true, ".", ""},
		{"forward . 127.0.0.1 {\nhealth_check 0.5s no_rec\n}\n", false, false, ".", ""},
		{"forward . 127.0.0.1 {\nhealth_check 0.5s no_rec domain example.org\n}\n", false, false, "example.org.", ""},
		{"forward . 127.0.0.1 {\nhealth_check 0.5s domain example.org\n}\n", false, true, "example.org.", ""},
		{"forward . 127.0.0.1 {\nhealth_check 0.5s domain .\n}\n", false, true, ".", ""},
		{"forward . 127.0.0.1 {\nhealth_check 0.5s domain example.org.\n}\n", false, true, "example.org.", ""},
		// negative
		{"forward . 127.0.0.1 {\nhealth_check no_rec\n}\n", true, true, ".", "time: invalid duration"},
		{"forward . 127.0.0.1 {\nhealth_check domain example.org\n}\n", true, true, "example.org", "time: invalid duration"},
		{"forward . 127.0.0.1 {\nhealth_check 0.5s rec\n}\n", true, true, ".", "health_check: unknown option rec"},
		{"forward . 127.0.0.1 {\nhealth_check 0.5s domain\n}\n", true, true, ".", "Wrong argument count or unexpected line ending after 'domain'"},
		{"forward . 127.0.0.1 {\nhealth_check 0.5s domain example..org\n}\n", true, true, ".", "health_check: invalid domain name"},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		fs, err := parseForward(c)

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: expected error but found %s for input %s", i, err, test.input)
		}

		if err != nil {
			if !test.shouldErr {
				t.Errorf("Test %d: expected no error but found one for input %s, got: %v", i, test.input, err)
			}
			if !strings.Contains(err.Error(), test.expectedErr) {
				t.Errorf("Test %d: expected error to contain: %v, found error: %v, input: %s", i, test.expectedErr, err, test.input)
			}
		}

		if test.shouldErr {
			continue
		}

		f := fs[0]
		if f.opts.HCRecursionDesired != test.expectedRecVal || f.proxies[0].GetHealthchecker().GetRecursionDesired() != test.expectedRecVal ||
			f.opts.HCDomain != test.expectedDomain || f.proxies[0].GetHealthchecker().GetDomain() != test.expectedDomain || !dns.IsFqdn(f.proxies[0].GetHealthchecker().GetDomain()) {
			t.Errorf("Test %d: expectedRec: %v, got: %v. expectedDomain: %s, got: %s. ", i, test.expectedRecVal, f.opts.HCRecursionDesired, test.expectedDomain, f.opts.HCDomain)
		}
	}
}

func TestMultiForward(t *testing.T) {
	input := `
      forward 1st.example.org 10.0.0.1
      forward 2nd.example.org 10.0.0.2
      forward 3rd.example.org 10.0.0.3
    `

	c := caddy.NewTestController("dns", input)
	setup(c)
	dnsserver.NewServer("", []*dnsserver.Config{dnsserver.GetConfig(c)})

	handlers := dnsserver.GetConfig(c).Handlers()
	f1, ok := handlers[0].(*Forward)
	if !ok {
		t.Fatalf("expected first plugin to be Forward, got %v", reflect.TypeOf(f1.Next))
	}

	if f1.from != "1st.example.org." {
		t.Errorf("expected first forward from \"1st.example.org.\", got %q", f1.from)
	}
	if f1.Next == nil {
		t.Fatal("expected first forward to point to next forward instance, not nil")
	}

	f2, ok := f1.Next.(*Forward)
	if !ok {
		t.Fatalf("expected second plugin to be Forward, got %v", reflect.TypeOf(f1.Next))
	}
	if f2.from != "2nd.example.org." {
		t.Errorf("expected second forward from \"2nd.example.org.\", got %q", f2.from)
	}
	if f2.Next == nil {
		t.Fatal("expected second forward to point to third forward instance, got nil")
	}

	f3, ok := f2.Next.(*Forward)
	if !ok {
		t.Fatalf("expected third plugin to be Forward, got %v", reflect.TypeOf(f2.Next))
	}
	if f3.from != "3rd.example.org." {
		t.Errorf("expected third forward from \"3rd.example.org.\", got %q", f3.from)
	}
	if f3.Next != nil {
		t.Error("expected third plugin to be last, but Next is not nil")
	}
}
func TestNextAlternate(t *testing.T) {
	testsValid := []struct {
		input    string
		expected []int
	}{
		{"forward . 127.0.0.1 {\nnext NXDOMAIN\n}\n", []int{dns.RcodeNameError}},
		{"forward . 127.0.0.1 {\nnext SERVFAIL\n}\n", []int{dns.RcodeServerFailure}},
		{"forward . 127.0.0.1 {\nnext NXDOMAIN SERVFAIL\n}\n", []int{dns.RcodeNameError, dns.RcodeServerFailure}},
		{"forward . 127.0.0.1 {\nnext NXDOMAIN SERVFAIL REFUSED\n}\n", []int{dns.RcodeNameError, dns.RcodeServerFailure, dns.RcodeRefused}},
	}
	for i, test := range testsValid {
		c := caddy.NewTestController("dns", test.input)
		f, err := parseForward(c)
		forward := f[0]
		if err != nil {
			t.Errorf("Test %d: %v", i, err)
		}
		if len(forward.nextAlternateRcodes) != len(test.expected) {
			t.Errorf("Test %d: expected %d next rcodes, got %d", i, len(test.expected), len(forward.nextAlternateRcodes))
		}
		for j, rcode := range forward.nextAlternateRcodes {
			if rcode != test.expected[j] {
				t.Errorf("Test %d: expected next rcode %d, got %d", i, test.expected[j], rcode)
			}
		}
	}

	testsInvalid := []string{
		"forward . 127.0.0.1 {\nnext\n}\n",
		"forward . 127.0.0.1 {\nnext INVALID\n}\n",
		"forward . 127.0.0.1 {\nnext NXDOMAIN INVALID\n}\n",
	}
	for i, test := range testsInvalid {
		c := caddy.NewTestController("dns", test)
		_, err := parseForward(c)
		if err == nil {
			t.Errorf("Test %d: expected error, got nil", i)
		}
	}
}

func TestFailfastAllUnhealthyUpstreams(t *testing.T) {
	tests := []struct {
		input          string
		expectedRecVal bool
		expectedErr    string
	}{
		// positive
		{"forward . 127.0.0.1\n", false, ""},
		{"forward . 127.0.0.1 {\nfailfast_all_unhealthy_upstreams\n}\n", true, ""},
		// negative
		{"forward . 127.0.0.1 {\nfailfast_all_unhealthy_upstreams false\n}\n", false, "Wrong argument count"},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		fs, err := parseForward(c)

		if err != nil {
			if test.expectedErr == "" {
				t.Errorf("Test %d: expected no error but found one for input %s, got: %v", i, test.input, err)
			}
			if !strings.Contains(err.Error(), test.expectedErr) {
				t.Errorf("Test %d: expected error to contain: %v, found error: %v, input: %s", i, test.expectedErr, err, test.input)
			}
		} else {
			if test.expectedErr != "" {
				t.Errorf("Test %d: expected error but found no error for input %s", i, test.input)
			}
		}

		if test.expectedErr != "" {
			continue
		}

		f := fs[0]
		if f.failfastUnhealthyUpstreams != test.expectedRecVal {
			t.Errorf("Test %d: Expected Rec:%v, got:%v", i, test.expectedRecVal, f.failfastUnhealthyUpstreams)
		}
	}
}

func TestFailover(t *testing.T) {

	server_fail_s := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetRcode(r, dns.RcodeServerFailure)
		w.WriteMsg(ret)
	})
	defer server_fail_s.Close()

	server_refused_s := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetRcode(r, dns.RcodeRefused)
		w.WriteMsg(ret)
	})
	defer server_refused_s.Close()

	s := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		ret.Answer = append(ret.Answer, test.A("example.org. IN A 127.0.0.1"))
		w.WriteMsg(ret)
	})
	defer s.Close()

	tests := []struct {
		input     string
		hasRecord bool
		failMsg   string
	}{
		{fmt.Sprintf(
			`forward . %s %s %s {
				policy sequential
				failover ServFail Refused
				}`, server_fail_s.Addr, server_refused_s.Addr, s.Addr), true, "If failover is set, records should be returned as long as one of the upstreams is work"},
		{fmt.Sprintf(
			`forward . %s %s %s {
				policy sequential
				}`, server_fail_s.Addr, server_refused_s.Addr, s.Addr), false, "If failover is not set and the first upstream is not work, no records should be returned"},
		{fmt.Sprintf(
			`forward . %s %s %s {
				policy sequential
				}`, s.Addr, server_fail_s.Addr, server_refused_s.Addr), true, "Although failover is not set, as long as the first upstream is work, there should be has a record return"},
	}

	for _, testCase := range tests {
		c := caddy.NewTestController("dns", testCase.input)
		fs, err := parseForward(c)

		f := fs[0]
		if err != nil {
			t.Errorf("Failed to create forwarder: %s", err)
		}
		f.OnStartup()
		defer f.OnShutdown()

		m := new(dns.Msg)
		m.SetQuestion("example.org.", dns.TypeA)
		rec := dnstest.NewRecorder(&test.ResponseWriter{})

		if _, err := f.ServeDNS(context.TODO(), rec, m); err != nil {
			t.Fatal("Expected to receive reply, but didn't")
		}

		if (len(rec.Msg.Answer) > 0) != testCase.hasRecord {
			t.Errorf(" %s: \n %s", testCase.failMsg, testCase.input)
		}
	}
}
