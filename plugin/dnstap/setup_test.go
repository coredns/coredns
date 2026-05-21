package dnstap

import (
	"os"
	"reflect"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
)

type results struct {
	endpoint            string
	full                bool
	proto               string
	identity            []byte
	version             []byte
	extraFormat         string
	multipleTcpWriteBuf int
	multipleQueue       int
	isListener          bool
	certFile            string
	keyFile             string
	caFile              string
	skipVerify          bool
}

func TestConfig(t *testing.T) {
	hostname, _ := os.Hostname()
	tests := []struct {
		in     string
		fail   bool
		expect []results
	}{
		{"dnstap dnstap.sock full", false, []results{{endpoint: "dnstap.sock", full: true, proto: "unix", identity: []byte(hostname), version: []byte("-"), multipleTcpWriteBuf: 1, multipleQueue: 1}}},
		{"dnstap unix://dnstap.sock", false, []results{{endpoint: "dnstap.sock", full: false, proto: "unix", identity: []byte(hostname), version: []byte("-"), multipleTcpWriteBuf: 1, multipleQueue: 1}}},
		{"dnstap tcp://127.0.0.1:6000", false, []results{{endpoint: "127.0.0.1:6000", full: false, proto: "tcp", identity: []byte(hostname), version: []byte("-"), multipleTcpWriteBuf: 1, multipleQueue: 1}}},
		{"dnstap tcp://[::1]:6000", false, []results{{endpoint: "[::1]:6000", full: false, proto: "tcp", identity: []byte(hostname), version: []byte("-"), multipleTcpWriteBuf: 1, multipleQueue: 1}}},
		{"dnstap tcp://example.com:6000", false, []results{{endpoint: "example.com:6000", full: false, proto: "tcp", identity: []byte(hostname), version: []byte("-"), multipleTcpWriteBuf: 1, multipleQueue: 1}}},
		{"dnstap", true, []results{{endpoint: "fail", full: false, proto: "tcp", identity: []byte(hostname), version: []byte("-"), multipleTcpWriteBuf: 1, multipleQueue: 1}}},
		{"dnstap dnstap.sock full {\nidentity NAME\nversion VER\n}\n", false, []results{{endpoint: "dnstap.sock", full: true, proto: "unix", identity: []byte("NAME"), version: []byte("VER"), multipleTcpWriteBuf: 1, multipleQueue: 1}}},
		{"dnstap dnstap.sock full {\nidentity NAME\nversion VER\nextra EXTRA\n}\n", false, []results{{endpoint: "dnstap.sock", full: true, proto: "unix", identity: []byte("NAME"), version: []byte("VER"), extraFormat: "EXTRA", multipleTcpWriteBuf: 1, multipleQueue: 1}}},
		{"dnstap dnstap.sock {\nidentity NAME\nversion VER\nextra EXTRA\n}\n", false, []results{{endpoint: "dnstap.sock", full: false, proto: "unix", identity: []byte("NAME"), version: []byte("VER"), extraFormat: "EXTRA", multipleTcpWriteBuf: 1, multipleQueue: 1}}},
		{"dnstap {\nidentity NAME\nversion VER\nextra EXTRA\n}\n", true, []results{{endpoint: "fail", full: false, proto: "tcp", identity: []byte("NAME"), version: []byte("VER"), extraFormat: "EXTRA", multipleTcpWriteBuf: 1, multipleQueue: 1}}},
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
			{endpoint: "dnstap.sock", full: true, proto: "unix", identity: []byte("NAME"), version: []byte("VER"), extraFormat: "EXTRA", multipleTcpWriteBuf: 1, multipleQueue: 1},
			{endpoint: "127.0.0.1:6000", full: false, proto: "tcp", identity: []byte("NAME2"), version: []byte("VER2"), extraFormat: "EXTRA2", multipleTcpWriteBuf: 1, multipleQueue: 1},
		}},
		{"dnstap tls://127.0.0.1:6000", false, []results{{endpoint: "127.0.0.1:6000", full: false, proto: "tls", identity: []byte(hostname), version: []byte("-"), multipleTcpWriteBuf: 1, multipleQueue: 1}}},
		{"dnstap dnstap.sock {\nidentity\n}\n", true, []results{{endpoint: "dnstap.sock", full: false, proto: "unix", identity: []byte(hostname), version: []byte("-"), multipleTcpWriteBuf: 1, multipleQueue: 1}}},
		{"dnstap dnstap.sock {\nversion\n}\n", true, []results{{endpoint: "dnstap.sock", full: false, proto: "unix", identity: []byte(hostname), version: []byte("-"), multipleTcpWriteBuf: 1, multipleQueue: 1}}},
		{"dnstap dnstap.sock {\nextra\n}\n", true, []results{{endpoint: "dnstap.sock", full: false, proto: "unix", identity: []byte(hostname), version: []byte("-"), multipleTcpWriteBuf: 1, multipleQueue: 1}}},
		{"dnstap dnstap.sock {\nidentitiy NAME\n}\n", true, []results{{endpoint: "dnstap.sock", full: false, proto: "unix", identity: []byte(hostname), version: []byte("-"), multipleTcpWriteBuf: 1, multipleQueue: 1}}},
		// Limits and parsing for writebuffer (MiB) and queue (x10k)
		{"dnstap dnstap.sock full 1024 2048", false, []results{{endpoint: "dnstap.sock", full: true, proto: "unix", identity: []byte(hostname), version: []byte("-"), multipleTcpWriteBuf: 1024, multipleQueue: 2048}}},
		{"dnstap dnstap.sock full 1025 1", true, []results{{endpoint: "dnstap.sock", full: true, proto: "unix", identity: []byte(hostname), version: []byte("-"), multipleTcpWriteBuf: 1, multipleQueue: 1}}},
		{"dnstap dnstap.sock full 1 4097", true, []results{{endpoint: "dnstap.sock", full: true, proto: "unix", identity: []byte(hostname), version: []byte("-"), multipleTcpWriteBuf: 1, multipleQueue: 1}}},
		{"dnstap dnstap.sock full 0 10", true, []results{{endpoint: "dnstap.sock", full: true, proto: "unix", identity: []byte(hostname), version: []byte("-"), multipleTcpWriteBuf: 1, multipleQueue: 1}}},
		{"dnstap dnstap.sock full 10 0", true, []results{{endpoint: "dnstap.sock", full: true, proto: "unix", identity: []byte(hostname), version: []byte("-"), multipleTcpWriteBuf: 1, multipleQueue: 1}}},
		{"dnstap dnstap.sock full x 10", true, []results{{endpoint: "dnstap.sock", full: true, proto: "unix", identity: []byte(hostname), version: []byte("-"), multipleTcpWriteBuf: 1, multipleQueue: 1}}},
		{"dnstap dnstap.sock full 10 y", true, []results{{endpoint: "dnstap.sock", full: true, proto: "unix", identity: []byte(hostname), version: []byte("-"), multipleTcpWriteBuf: 1, multipleQueue: 1}}},

		// Listener tests
		{"dnstap listen tcp://127.0.0.1:6000", false, []results{{endpoint: "127.0.0.1:6000", full: false, proto: "tcp", identity: []byte(hostname), version: []byte("-"), isListener: true}}},
		{"dnstap listen tcp://127.0.0.1:6000 full", false, []results{{endpoint: "127.0.0.1:6000", full: true, proto: "tcp", identity: []byte(hostname), version: []byte("-"), isListener: true}}},
		{"dnstap listen unix:///tmp/dnstap.sock", false, []results{{endpoint: "/tmp/dnstap.sock", full: false, proto: "unix", identity: []byte(hostname), version: []byte("-"), isListener: true}}},
		{"dnstap listen /tmp/dnstap.sock full", false, []results{{endpoint: "/tmp/dnstap.sock", full: true, proto: "unix", identity: []byte(hostname), version: []byte("-"), isListener: true}}},
		{"dnstap listen tls://127.0.0.1:6000 full {\ntls /path/to/cert.pem /path/to/key.pem\n}\n", false, []results{{endpoint: "127.0.0.1:6000", full: true, proto: "tls", identity: []byte(hostname), version: []byte("-"), isListener: true, certFile: "/path/to/cert.pem", keyFile: "/path/to/key.pem"}}},
		{"dnstap listen tls://127.0.0.1:6000 {\ntls /path/to/cert.pem /path/to/key.pem /path/to/ca.pem\n}\n", false, []results{{endpoint: "127.0.0.1:6000", full: false, proto: "tls", identity: []byte(hostname), version: []byte("-"), isListener: true, certFile: "/path/to/cert.pem", keyFile: "/path/to/key.pem", caFile: "/path/to/ca.pem"}}},
		{"dnstap listen tls://127.0.0.1:6000 {\ntls /path/to/cert.pem /path/to/key.pem\nskipverify\n}\n", false, []results{{endpoint: "127.0.0.1:6000", full: false, proto: "tls", identity: []byte(hostname), version: []byte("-"), isListener: true, certFile: "/path/to/cert.pem", keyFile: "/path/to/key.pem", skipVerify: true}}},
		{"dnstap listen", true, nil}, // Missing endpoint
		{"dnstap listen tcp://127.0.0.1:6000 {\ntls /path/to/cert.pem\n}\n", true, nil}, // Missing key file for TLS

		// Mixed outgoing and listener
		{`dnstap tcp://remote.example.com:6000 full
              dnstap listen tcp://127.0.0.1:6001`, false, []results{
			{endpoint: "remote.example.com:6000", full: true, proto: "tcp", identity: []byte(hostname), version: []byte("-"), isListener: false, multipleTcpWriteBuf: 1, multipleQueue: 1},
			{endpoint: "127.0.0.1:6001", full: false, proto: "tcp", identity: []byte(hostname), version: []byte("-"), isListener: true},
		}},
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
		for j, tap := range taps {
			if tc.expect[j].isListener {
				// Verify listener configuration
				if tap.listener == nil {
					t.Errorf("Test %d: expected listener to be set", i)
					continue
				}
				if x := tap.listener.endpoint; x != tc.expect[j].endpoint {
					t.Errorf("Test %d: expected listener endpoint %s, got %s", i, tc.expect[j].endpoint, x)
				}
				if x := tap.listener.proto; x != tc.expect[j].proto {
					t.Errorf("Test %d: expected listener proto %s, got %s", i, tc.expect[j].proto, x)
				}
				if x := tap.listener.certFile; x != tc.expect[j].certFile {
					t.Errorf("Test %d: expected listener certFile %s, got %s", i, tc.expect[j].certFile, x)
				}
				if x := tap.listener.keyFile; x != tc.expect[j].keyFile {
					t.Errorf("Test %d: expected listener keyFile %s, got %s", i, tc.expect[j].keyFile, x)
				}
				if x := tap.listener.caFile; x != tc.expect[j].caFile {
					t.Errorf("Test %d: expected listener caFile %s, got %s", i, tc.expect[j].caFile, x)
				}
				if x := tap.listener.skipVerify; x != tc.expect[j].skipVerify {
					t.Errorf("Test %d: expected listener skipVerify %t, got %t", i, tc.expect[j].skipVerify, x)
				}
			} else {
				// Verify outgoing connection configuration
				if tap.io == nil {
					t.Errorf("Test %d: expected io to be set", i)
					continue
				}
				if x := tap.io.(*dio).endpoint; x != tc.expect[j].endpoint {
					t.Errorf("Test %d: expected endpoint %s, got %s", i, tc.expect[j].endpoint, x)
				}
				if x := tap.io.(*dio).proto; x != tc.expect[j].proto {
					t.Errorf("Test %d: expected proto %s, got %s", i, tc.expect[j].proto, x)
				}
				if x := tap.MultipleTcpWriteBuf; x != tc.expect[j].multipleTcpWriteBuf {
					t.Errorf("Test %d: expected MultipleTcpWriteBuf %d, got %d", i, tc.expect[j].multipleTcpWriteBuf, x)
				}
				if x := tap.MultipleQueue; x != tc.expect[j].multipleQueue {
					t.Errorf("Test %d: expected MultipleQueue %d, got %d", i, tc.expect[j].multipleQueue, x)
				}
			}
			// Common properties
			if x := tap.IncludeRawMessage; x != tc.expect[j].full {
				t.Errorf("Test %d: expected IncludeRawMessage %t, got %t", i, tc.expect[j].full, x)
			}
			if x := string(tap.Identity); x != string(tc.expect[j].identity) {
				t.Errorf("Test %d: expected identity %s, got %s", i, tc.expect[j].identity, x)
			}
			if x := string(tap.Version); x != string(tc.expect[j].version) {
				t.Errorf("Test %d: expected version %s, got %s", i, tc.expect[j].version, x)
			}
			if x := tap.ExtraFormat; x != tc.expect[j].extraFormat {
				t.Errorf("Test %d: expected extra format %s, got %s", i, tc.expect[j].extraFormat, x)
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
		t.Fatalf("expected first plugin to be Dnstap, got %v", reflect.TypeOf(handlers[0]))
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
