package dso

import (
	"errors"
	"net"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/coredns/caddy"
	_ "github.com/coredns/coredns/plugin/bind"
	"github.com/coredns/coredns/plugin/dso/internal/dsomessage"

	"github.com/miekg/dns"
)

// setupCoreDNSWithDSOf is like setupCoreDNSf but ensures DSO servers are paired.
func setupCoreDNSWithDSOf(tb testing.TB, format string, a ...any) (inst *caddy.Instance) {
	tb.Helper()

	inst, err := setupCoreDNSf(tb, format, a...)
	if err != nil {
		tb.Fatalf("Got Start()=%v", err)
	}
	pairServers(tb, inst)
	return inst
}

// pairServers ensures that all DSO servers in given instance are paired to their corresponding upstreams.
func pairServers(tb testing.TB, inst *caddy.Instance) {
	tb.Helper()

	for _, dnsServer := range inst.Servers() {
		dnsIP := dnsServer.Addr().(*net.TCPAddr).IP
		if dnsIP.IsUnspecified() {
			dnsIP = net.IPv4(127, 0, 0, 1)
		}
		dnsPort := dnsServer.Addr().(*net.TCPAddr).Port
		dnsAddr := (&net.TCPAddr{IP: dnsIP, Port: dnsPort}).String()

		inst.StorageMu.RLock()
		dnsConfigs := inst.Storage[instanceDSOKey{}].(*instanceDSO).configs
		inst.StorageMu.RUnlock()

		for cfg := range dnsConfigs {
			m := new(dns.Msg).SetQuestion(cfg.Zone, dns.TypeSOA)
			dns.ExchangeContext(tb.Context(), m, dnsAddr)
		}
	}
}

// assertDSOConn sets up and asserts functional DSO connection.
func assertDSOConn(tb testing.TB, addr string) (dsoConn *testConn) {
	tb.Helper()

	conn, err := net.DialTimeout("tcp", addr, noMessageTimeout)
	if err != nil {
		tb.Fatalf("Got Dial()=%v", err)
	}
	dsoConn = &testConn{conn}
	tb.Cleanup(func() {
		dsoConn.Close()
	})
	dsoConn.assertExchangeKeepAlive(tb, 1, dsomessage.KeepAlive{})
	return dsoConn
}

func TestHandlerPairing(t *testing.T) {
	ports := assertAllocatePorts(t, 2)
	tcs := []struct {
		name        string
		corefile    string
		wantServers int
	}{
		{
			"single",
			`
			test.:%[1]d {
				dso {
					tcp_port %[2]d
				}
			}`,
			1,
		},
		{
			"joint",
			`
			a.test.:%[1]d b.test.:%[1]d {
				dso {
					tcp_port %[2]d
				}
			}`,
			1,
		},
		{
			"split",
			`
			a.test.:%[1]d {
				dso {
					tcp_port %[2]d
				}
			}
			b.test.:%[1]d {
			}`,
			1,
		},
		{
			"bind single",
			`
			a.test.:%[1]d {
				bind 127.0.0.1 ::1
				dso {
					tcp_port %[2]d
				}
			}`,
			2,
		},
		{
			"bind split",
			`
			a.test.:%[1]d {
				bind 127.0.0.1
				dso {
					tcp_port %[2]d
				}
			}
			b.test.:%[1]d {
				bind ::1
			}`,
			1,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			inst := setupCoreDNSWithDSOf(t, tc.corefile, ports[0], ports[1])
			if servers := Servers(inst); len(servers) != tc.wantServers {
				t.Errorf("Got %v, want %d servers", servers, tc.wantServers)
			}
		})
	}
}

func TestHandlerBareConfig(t *testing.T) {
	ports := assertAllocatePorts(t, 2)

	t.Run("bare", func(t *testing.T) {
		corefile := `
		a.test.:%[1]d {
			dso {
				tcp_port %[2]d
			}
		}
		b.test.:%[1]d {
			dso
		}`
		inst, err := setupCoreDNSf(t, corefile, ports[0], ports[1])
		if err != nil {
			t.Fatalf("Got %v", err)
		}
		m := new(dns.Msg).SetQuestion("b.test.", dns.TypeSOA)
		_, err = dns.ExchangeContext(t.Context(), m, "127.0.0.1:"+strconv.Itoa(ports[0]))
		if err != nil {
			t.Fatalf("Got %v", err)
		}
		if len(Servers(inst)) != 1 {
			t.Fatal("Want server")
		}
	})

	t.Run("bare only", func(t *testing.T) {
		corefile := `
		test.:%[1]d {
			dso
		}`
		_, err := setupCoreDNSf(t, corefile, ports[0], ports[1])
		if !errors.Is(err, errBare) {
			t.Fatalf("Got %v, want %v", err, errBare)
		}
	})
}

func TestHandlerGracefulRestart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("graceful restart is not supported on windows")
	}
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	var wg sync.WaitGroup
	t.Cleanup(wg.Wait)

	ports := assertAllocatePorts(t, 2)
	corefile := `
	test.:%[1]d {
		dso {
			tcp_port %[2]d
			reconnect %v %v
		}
	}`
	inst := setupCoreDNSWithDSOf(t, corefile, ports[0], ports[1], time.Minute, time.Hour)
	conn := assertDSOConn(t, net.JoinHostPort("127.0.0.1", strconv.Itoa(ports[1])))

	wg.Go(func() {
		inst1, err := inst.Restart(inst.Caddyfile())
		if err != nil {
			t.Errorf("Got Restart()=%v", err)
		} else {
			t.Cleanup(func() {
				inst1.Stop()
				inst1.ShutdownCallbacks()
			})
		}
	})

	m := conn.assertReadMsg(t).(*dsomessage.Msg)
	if m.Rcode != dns.RcodeSuccess || len(m.TLV) == 0 || m.TLV[0].Type() != dsomessage.TypeRetryDelay {
		t.Fatalf("Got %v, want RetryDelay", m)
	}
	v := m.TLV[0].(*dsomessage.RetryDelay).RetryDelay
	if retryDelay := time.Duration(v) * time.Millisecond; retryDelay != time.Minute {
		t.Errorf("Got RetryDelay=%v, want %v", retryDelay, time.Minute)
	}
}

func TestHandlerGracefulShutdown(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	var wg sync.WaitGroup
	t.Cleanup(wg.Wait)

	ports := assertAllocatePorts(t, 2)
	corefile := `
	test.:%[1]d {
		dso {
			tcp_port %[2]d
			reconnect %v %v
		}
	}`
	inst := setupCoreDNSWithDSOf(t, corefile, ports[0], ports[1], time.Minute, time.Hour)
	conn := assertDSOConn(t, net.JoinHostPort("127.0.0.1", strconv.Itoa(ports[1])))

	wg.Go(func() {
		inst.ShutdownCallbacks()
		inst.Stop()
	})

	m := conn.assertReadMsg(t).(*dsomessage.Msg)
	if m.Rcode != dns.RcodeSuccess || len(m.TLV) == 0 || m.TLV[0].Type() != dsomessage.TypeRetryDelay {
		t.Fatalf("Got %v, want RetryDelay", m)
	}
	v := m.TLV[0].(*dsomessage.RetryDelay).RetryDelay
	if retryDelay := time.Duration(v) * time.Millisecond; retryDelay != time.Hour {
		t.Errorf("Got RetryDelay=%v, want %v", retryDelay, time.Hour)
	}
}

func TestHandlerRestartTransition(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("graceful restart is not supported on windows")
	}

	t.Run("load", func(t *testing.T) {
		dnsPort := assertAllocatePorts(t, 1)[0]
		corefile := `
		test.:%[1]d {
		}`
		inst, err := setupCoreDNSf(t, corefile, dnsPort)
		if err != nil {
			t.Fatalf("Got Start()=%v", err)
		}

		dsoPort := assertAllocatePorts(t, 1)[0]
		corefile = `
		test.:%[1]d {
			dso {
				tcp_port %[2]d
				reconnect -1s -1s
			}
		}`
		inst1, err := inst.Restart(setupCorefilef(corefile, dnsPort, dsoPort))
		if err != nil {
			t.Fatalf("Got Restart()=%v", err)
		}
		t.Cleanup(func() {
			inst1.ShutdownCallbacks()
			inst1.Stop()
		})
		pairServers(t, inst1)
		assertDSOConn(t, net.JoinHostPort("127.0.0.1", strconv.Itoa(dsoPort)))
	})

	t.Run("successful", func(t *testing.T) {
		ports := assertAllocatePorts(t, 2)
		corefile := `
		test.:%[1]d {
			dso {
				tcp_port %[2]d
				reconnect -1s -1s
			}
		}`
		inst := setupCoreDNSWithDSOf(t, corefile, ports[0], ports[1])
		dsoAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(ports[1]))

		tryConnectFunc := func() error {
			conn, err := net.DialTimeout("tcp", dsoAddr, noMessageTimeout)
			if err != nil {
				return err
			}
			conn.Close()
			return nil
		}

		var (
			wg         sync.WaitGroup
			doneC      = make(chan struct{})
			restartedC = make(chan struct{})
		)
		wg.Go(func() {
			for {
				select {
				case <-doneC:
					return
				default:
				}
				if err := tryConnectFunc(); err != nil {
					t.Errorf("Got Dial()=%v", err)
					return
				}
			}
		})
		wg.Go(func() {
			<-restartedC
			if err := tryConnectFunc(); err != nil {
				t.Errorf("Got Dial()=%v", err)
			}
		})
		defer wg.Wait()
		defer close(doneC)

		corefile = `
		b.test.:%[1]d {
			dso {
				tcp_port %[2]d
				reconnect -1s -1s
			}
		}`
		inst1, err := inst.Restart(setupCorefilef(corefile, ports[0], ports[1]))
		close(restartedC)
		if err != nil {
			t.Fatalf("Got Restart()=%v", err)
		}
		t.Cleanup(func() {
			inst1.ShutdownCallbacks()
			inst1.Stop()
		})
		pairServers(t, inst1)
	})

	t.Run("failed", func(t *testing.T) {
		ports := assertAllocatePorts(t, 2)
		corefile := `
		test.:%[1]d {
			dso {
				tcp_port %[2]d
				reconnect -1s -1s
			}
		}`
		inst := setupCoreDNSWithDSOf(t, corefile, ports[0], ports[1])
		conn := assertDSOConn(t, net.JoinHostPort("127.0.0.1", strconv.Itoa(ports[1])))

		corefile = `
		test.:%[1]d {
			foobar
		}`
		_, err := inst.Restart(setupCorefilef(corefile, ports[0]))
		if err == nil {
			t.Fatal("Want Restart() error")
		}
		conn.assertExchangeKeepAlive(t, 42, dsomessage.KeepAlive{})
	})

	t.Run("unload", func(t *testing.T) {
		ports := assertAllocatePorts(t, 2)
		corefile := `
		test.:%[1]d {
			dso {
				tcp_port %[2]d
				reconnect -1s -1s
			}
		}`
		inst := setupCoreDNSWithDSOf(t, corefile, ports[0], ports[1])

		inst1, err := inst.Restart(setupCorefilef("test.:%d \n{\n}", ports[0]))
		if err != nil {
			t.Fatalf("Got Restart()=%v", err)
		}
		t.Cleanup(func() {
			inst1.Stop()
			inst1.ShutdownCallbacks()
		})

		dsoAddr := net.JoinHostPort("", strconv.Itoa(ports[1]))
		ln, err := net.Listen("tcp", dsoAddr)
		if ln != nil {
			ln.Close()
		}
		if err != nil {
			t.Errorf("Got Listen()=%v", err)
		}
		if servers := Servers(inst1); len(servers) > 0 {
			t.Errorf("Got %v, want no servers", servers)
		}
	})
}

func TestHandlerRestartPortSwap(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("graceful restart is not supported on windows")
	}

	t.Run("dns", func(t *testing.T) {
		ports := assertAllocatePorts(t, 2)
		corefile := `
		test.:%[1]d {
			dso {
				tcp_port %[2]d
				reconnect -1s -1s
			}
		}`
		inst := setupCoreDNSWithDSOf(t, corefile, ports[0], ports[1])

		inst1, err := inst.Restart(setupCorefilef(corefile, ports[1], ports[0]))
		if err != nil {
			t.Fatalf("Got Restart()=%v", err)
		}
		t.Cleanup(func() {
			inst1.ShutdownCallbacks()
			inst1.Stop()
		})

		pairServers(t, inst1)
		assertDSOConn(t, net.JoinHostPort("127.0.0.1", strconv.Itoa(ports[0])))
	})

	t.Run("dso", func(t *testing.T) {
		ports := assertAllocatePorts(t, 4)
		corefile := `
		test.:%[1]d {
			dso {
				tcp_port %[2]d
				reconnect -1s -1s
			}
		}

		test.:%[3]d {
			dso {
				tcp_port %[4]d
				reconnect -1s -1s
			}
		}`
		inst := setupCoreDNSWithDSOf(t, corefile, ports[0], ports[1], ports[2], ports[3])

		inst1, err := inst.Restart(setupCorefilef(corefile, ports[0], ports[3], ports[2], ports[1]))
		if err != nil {
			t.Fatalf("Got Restart()=%v", err)
		}
		t.Cleanup(func() {
			inst1.ShutdownCallbacks()
			inst1.Stop()
		})

		pairServers(t, inst1)
		assertDSOConn(t, net.JoinHostPort("127.0.0.1", strconv.Itoa(ports[1])))
		assertDSOConn(t, net.JoinHostPort("127.0.0.1", strconv.Itoa(ports[3])))
	})
}
