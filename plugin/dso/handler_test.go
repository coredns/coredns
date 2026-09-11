package dso

import (
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/coredns/caddy"
	_ "github.com/coredns/coredns/plugin/bind"
	"github.com/coredns/coredns/plugin/dso/internal/dsomessage"
	_ "github.com/coredns/coredns/plugin/multisocket"

	"github.com/miekg/dns"
)

func setupCoreDNSWithDSO(tb testing.TB, restartInterval, shutdownInterval time.Duration) (inst *caddy.Instance, dsoConn *testConn) {
	tb.Helper()

	ports := assertAllocatePorts(tb, 2)
	input := `
a.test.:%[1]d {
	dso {
		tcp_port %[2]d
		reconnect %v %v
	}
}`
	inst, err := setupCoreDNSf(tb, input, ports[0], ports[1], restartInterval, shutdownInterval)
	if err != nil {
		tb.Fatalf("Got %v, want DNS server", err)
	}

	// Plain DNS for pairing.
	dnsM := new(dns.Msg).SetQuestion("a.test.", dns.TypeA)
	_, err = dns.ExchangeContext(tb.Context(), dnsM, net.JoinHostPort("127.0.0.1", strconv.Itoa(ports[0])))
	if err != nil {
		tb.Fatalf("Got %v, want DNS answer", err)
	}

	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(ports[1])))
	if err != nil {
		tb.Fatalf("Got %v, want DSO connection", err)
	}
	dsoConn = &testConn{conn}
	tb.Cleanup(func() {
		dsoConn.Close()
	})
	dsoConn.assertExchangeKeepAlive(tb, 1, dsomessage.KeepAlive{})
	return inst, dsoConn
}

func TestHandlerPairing(t *testing.T) {
	ports := assertAllocatePorts(t, 2)
	tcs := []struct {
		name        string
		input       string
		zones       []string
		addrs       []string
		wantServers int
	}{
		{
			"single",
			`
a.test.:%[1]d {
	dso {
		tcp_port %[2]d
	}
}`,
			[]string{"a.test."},
			[]string{"127.0.0.1"},
			1,
		},
		{
			"joint a",
			`
a.test.:%[1]d b.test.:%[1]d {
	dso {
		tcp_port %[2]d
	}
}`,
			[]string{"a.test."},
			[]string{"127.0.0.1"},
			1,
		},
		{
			"joint b",
			`
		a.test.:%[1]d b.test.:%[1]d {
			dso {
				tcp_port %[2]d
			}
		}`,
			[]string{"b.test."},
			[]string{"127.0.0.1"},
			1,
		},
		{
			"split a",
			`
		a.test.:%[1]d {
			dso {
				tcp_port %[2]d
			}
		}

		b.test.:%[1]d {
		}`,
			[]string{"a.test."},
			[]string{"127.0.0.1"},
			1,
		},
		{
			"split b",
			`
		a.test.:%[1]d {
			dso {
				tcp_port %[2]d
			}
		}

		b.test.:%[1]d {
		}`,
			[]string{"b.test."},
			[]string{"127.0.0.1"},
			0,
		},
		{
			"split sentinel",
			`
		a.test.:%[1]d {
			dso {
				tcp_port %[2]d
			}
		}

		b.test.:%[1]d {
			dso
		}`,
			[]string{"b.test."},
			[]string{"127.0.0.1"},
			1,
		},
		{
			"bind 127.0.0.1",
			`
		a.test.:%[1]d {
			bind 127.0.0.1 ::1
			dso {
				tcp_port %[2]d
			}
		}`,
			[]string{"a.test."},
			[]string{"127.0.0.1"},
			1,
		},
		{
			"bind ::1",
			`
		a.test.:%[1]d {
			bind 127.0.0.1 ::1
			dso {
				tcp_port %[2]d
			}
		}`,
			[]string{"a.test."},
			[]string{"::1"},
			1,
		},
		{
			"bind all",
			`
		a.test.:%[1]d {
			bind 127.0.0.1 ::1
			dso {
				tcp_port %[2]d
			}
		}`,
			[]string{"a.test."},
			[]string{"127.0.0.1", "::1"},
			2,
		},
		{
			"multisocket",
			`
		a.test.:%[1]d {
			multisocket 5
			dso {
				tcp_port %[2]d
			}
		}`,
			[]string{"a.test."},
			[]string{"127.0.0.1", "127.0.0.1", "127.0.0.1", "127.0.0.1", "127.0.0.1"},
			1,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			inst, err := setupCoreDNSf(t, tc.input, ports[0], ports[1])
			if err != nil {
				t.Fatalf("Got %v", err)
			}

			for _, z := range tc.zones {
				m := new(dns.Msg).SetQuestion(z, dns.TypeA)
				for _, a := range tc.addrs {
					dns.ExchangeContext(t.Context(), m, net.JoinHostPort(a, strconv.Itoa(ports[0])))
				}
			}
			if servers := Servers(inst); len(servers) != tc.wantServers {
				t.Errorf("Got %v, want %d servers", servers, tc.wantServers)
			}
		})
	}
}

func TestHandlerRestart(t *testing.T) {
	inst, conn := setupCoreDNSWithDSO(t, time.Minute, time.Hour)

	var wg sync.WaitGroup
	wg.Go(func() { inst.Restart(inst.Caddyfile()) })
	defer wg.Wait()
	defer conn.Close()

	m := conn.assertReadMsg(t).(*dsomessage.Msg)
	if m.Rcode != dns.RcodeSuccess || len(m.TLV) == 0 || m.TLV[0].Type() != dsomessage.TypeRetryDelay {
		t.Fatalf("Got %v, want RetryDelay", m)
	}
	v := m.TLV[0].(*dsomessage.RetryDelay).RetryDelay
	if retryDelay := time.Duration(v) * time.Millisecond; retryDelay != time.Minute {
		t.Errorf("Got RetryDelay=%v, want %v", retryDelay, time.Minute)
	}
}

func TestHandlerShutdown(t *testing.T) {
	inst, conn := setupCoreDNSWithDSO(t, time.Minute, time.Hour)

	var wg sync.WaitGroup
	wg.Go(func() { inst.ShutdownCallbacks() })
	defer wg.Wait()
	defer conn.Close()

	m := conn.assertReadMsg(t).(*dsomessage.Msg)
	if m.Rcode != dns.RcodeSuccess || len(m.TLV) == 0 || m.TLV[0].Type() != dsomessage.TypeRetryDelay {
		t.Fatalf("Got %v, want RetryDelay", m)
	}
	v := m.TLV[0].(*dsomessage.RetryDelay).RetryDelay
	if retryDelay := time.Duration(v) * time.Millisecond; retryDelay != time.Hour {
		t.Errorf("Got RetryDelay=%v, want %v", retryDelay, time.Hour)
	}
}
