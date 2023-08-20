package dnstap

import (
	"os"
	"reflect"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
)

type results struct {
	endpoint    string
	full        bool
	proto       string
	identity    []byte
	version     []byte
	extraFormat string
}

func TestConfig(t *testing.T) {
	hostname, _ := os.Hostname()
	tests := []struct {
		in     string
		fail   bool
		expect []results
	}{
		{"dnstap dnstap.sock full", false, []results{{"dnstap.sock", true, "unix", []byte(hostname), []byte("-"), ""}}},
		{"dnstap unix://dnstap.sock", false, []results{{"dnstap.sock", false, "unix", []byte(hostname), []byte("-"), ""}}},
		{"dnstap tcp://127.0.0.1:6000", false, []results{{"127.0.0.1:6000", false, "tcp", []byte(hostname), []byte("-"), ""}}},
		{"dnstap tcp://[::1]:6000", false, []results{{"[::1]:6000", false, "tcp", []byte(hostname), []byte("-"), ""}}},
		{"dnstap tcp://example.com:6000", false, []results{{"example.com:6000", false, "tcp", []byte(hostname), []byte("-"), ""}}},
		{"dnstap", true, []results{{"fail", false, "tcp", []byte(hostname), []byte("-"), ""}}},
		{"dnstap dnstap.sock full {\nidentity NAME\nversion VER\n}\n", false, []results{{"dnstap.sock", true, "unix", []byte("NAME"), []byte("VER"), ""}}},
		{"dnstap dnstap.sock full {\nidentity NAME\nversion VER\nextra EXTRA\n}\n", false, []results{{"dnstap.sock", true, "unix", []byte("NAME"), []byte("VER"), "EXTRA"}}},
		{"dnstap dnstap.sock {\nidentity NAME\nversion VER\nextra EXTRA\n}\n", false, []results{{"dnstap.sock", false, "unix", []byte("NAME"), []byte("VER"), "EXTRA"}}},
		{"dnstap {\nidentity NAME\nversion VER\nextra EXTRA\n}\n", true, []results{{"fail", false, "tcp", []byte("NAME"), []byte("VER"), "EXTRA"}}},
		{`dnstap dnstap.sock full {
                identity NAME
                version VER
                extra EXTRA
              }
              dnstap tcp://127.0.0.1:6000 {
                identity NAME2
                version VER2
                extra EXTRA2
              }`, false, []results{
			{"dnstap.sock", true, "unix", []byte("NAME"), []byte("VER"), "EXTRA"},
			{"127.0.0.1:6000", false, "tcp", []byte("NAME2"), []byte("VER2"), "EXTRA2"},
		}},
		{"dnstap tls://127.0.0.1:6000", false, []results{{"127.0.0.1:6000", false, "tls", []byte(hostname), []byte("-"), ""}}},
		{"dnstap dnstap.sock {\nidentity\n}\n", true, []results{{"dnstap.sock", false, "unix", []byte(hostname), []byte("-"), ""}}},
		{"dnstap dnstap.sock {\nversion\n}\n", true, []results{{"dnstap.sock", false, "unix", []byte(hostname), []byte("-"), ""}}},
		{"dnstap dnstap.sock {\nextra\n}\n", true, []results{{"dnstap.sock", false, "unix", []byte(hostname), []byte("-"), ""}}},
	}
	for i, tc := range tests {
		c := caddy.NewTestController("dns", tc.in)
		taps, err := parseConfig(c)
		if tc.fail && err == nil {
			t.Fatalf("Test %d: expected test to fail: %s: %s", i, tc.in, err)
		}
		if tc.fail {
			continue
		}

		if err != nil {
			t.Fatalf("Test %d: expected no error, got %s", i, err)
		}
		for i, tap := range taps {
			if x := tap.io.(*dio).endpoint; x != tc.expect[i].endpoint {
				t.Errorf("Test %d: expected endpoint %s, got %s", i, tc.expect[i].endpoint, x)
			}
			if x := tap.io.(*dio).proto; x != tc.expect[i].proto {
				t.Errorf("Test %d: expected proto %s, got %s", i, tc.expect[i].proto, x)
			}
			if x := tap.IncludeRawMessage; x != tc.expect[i].full {
				t.Errorf("Test %d: expected IncludeRawMessage %t, got %t", i, tc.expect[i].full, x)
			}
			if x := string(tap.Identity); x != string(tc.expect[i].identity) {
				t.Errorf("Test %d: expected identity %s, got %s", i, tc.expect[i].identity, x)
			}
			if x := string(tap.Version); x != string(tc.expect[i].version) {
				t.Errorf("Test %d: expected version %s, got %s", i, tc.expect[i].version, x)
			}
			if x := tap.ExtraFormat; x != tc.expect[i].extraFormat {
				t.Errorf("Test %d: expected extra format %s, got %s", i, tc.expect[i].extraFormat, x)
			}
		}
	}
}

func TestMultiDnstap(t *testing.T) {
	input := `
      dnstap dnstap1.sock
      dnstap dnstap2.sock
      dnstap dnstap3.sock
    `

	c := caddy.NewTestController("dns", input)
	setup(c)
	dnsserver.NewServer("", []*dnsserver.Config{dnsserver.GetConfig(c)})

	handlers := dnsserver.GetConfig(c).Handlers()
	d1, ok := handlers[0].(*Dnstap)
	if !ok {
		t.Fatalf("expected first plugin to be Dnstap, got %v", reflect.TypeOf(d1.Next))
	}

	if d1.io.(*dio).endpoint != "dnstap1.sock" {
		t.Errorf("expected first dnstap to \"dnstap1.sock\", got %q", d1.io.(*dio).endpoint)
	}
	if d1.Next == nil {
		t.Fatal("expected first dnstap to point to next dnstap instance")
	}

	d2, ok := d1.Next.(*Dnstap)
	if !ok {
		t.Fatalf("expected second plugin to be Dnstap, got %v", reflect.TypeOf(d1.Next))
	}
	if d2.io.(*dio).endpoint != "dnstap2.sock" {
		t.Errorf("expected second dnstap to \"dnstap2.sock\", got %q", d2.io.(*dio).endpoint)
	}
	if d2.Next == nil {
		t.Fatal("expected second dnstap to point to third dnstap instance")
	}

	d3, ok := d2.Next.(*Dnstap)
	if !ok {
		t.Fatalf("expected third plugin to be Dnstap, got %v", reflect.TypeOf(d2.Next))
	}
	if d3.io.(*dio).endpoint != "dnstap3.sock" {
		t.Errorf("expected third dnstap to \"dnstap3.sock\", got %q", d3.io.(*dio).endpoint)
	}
	if d3.Next != nil {
		t.Error("expected third plugin to be last, but Next is not nil")
	}
}
