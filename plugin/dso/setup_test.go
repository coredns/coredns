package dso

import (
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
)

func setupCorefilef(format string, a ...any) caddy.CaddyfileInput {
	return caddy.CaddyfileInput{
		Filepath:       "Testfile",
		ServerTypeName: "dns",
		Contents:       fmt.Appendf(nil, format, a...),
	}
}

func setupCoreDNSf(tb testing.TB, format string, a ...any) (*caddy.Instance, error) {
	tb.Helper()

	caddy.Quiet = true
	dnsserver.Quiet = true

	inst, err := caddy.Start(setupCorefilef(format, a...))
	tb.Cleanup(func() {
		inst.Stop()
		inst.ShutdownCallbacks()
	})
	return inst, err
}

// assertAllocatePorts allocates given number of ports.
func assertAllocatePorts(tb testing.TB, num int) (ports []int) {
	tb.Helper()

	var lns []net.Listener
	for range num {
		var ok bool
		for range 10 {
			ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", "0"))
			if err != nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			lns = append(lns, ln)
			ports = append(ports, ln.Addr().(*net.TCPAddr).Port)
			ok = true
			break
		}
		if !ok {
			break
		}
	}
	for _, ln := range lns {
		ln.Close()
	}
	if len(ports) != num {
		tb.Fatal("Failed to allocate ports")
	}
	return ports
}

func TestSetupJoint(t *testing.T) {
	ports := assertAllocatePorts(t, 3)
	tcs := []struct {
		name     string
		corefile string
		wantErr  error
	}{
		{
			"same",
			`
			a.test.:%[1]d b.test.:%[1]d {
				dso {
					tcp_port %[3]d
				}
			}`,
			nil,
		},
		{
			"separate",
			`
			a.test.:%[1]d b.test.:%[2]d {
				dso {
					tcp_port %[3]d
				}
			}`,
			errShared,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			_, err := setupCoreDNSf(t, tc.corefile, ports[0], ports[1], ports[2])
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("Got %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestSetupSplit(t *testing.T) {
	ports := assertAllocatePorts(t, 4)
	tcs := []struct {
		name     string
		corefile string
		wantErr  error
	}{
		{
			"same",
			`
			a.test.:%[1]d {
				dso {
					tcp_port %[3]d
				}
			}
			b.test.:%[1]d {
				dso {
					tcp_port %[4]d
				}
			}`,
			errRedefined,
		},
		{
			"separate",
			`
			a.test.:%[1]d {
				dso {
					tcp_port %[3]d
				}
			}
			b.test.:%[2]d {
				dso {
					tcp_port %[4]d
				}
			}`,
			nil,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			_, err := setupCoreDNSf(t, tc.corefile, ports[0], ports[1], ports[2], ports[3])
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("Got %v, want %v", err, tc.wantErr)
			}
		})
	}

	t.Run("sentinel", func(t *testing.T) {
		ports := assertAllocatePorts(t, 2)
		corefile := `
		a.test.:%[1]d {
			dso {
				tcp_port %[2]d
			}
		}

		b.test.:%[1]d {
			dso
		}`
		_, err := setupCoreDNSf(t, corefile, ports[0], ports[1])
		if err != nil {
			t.Errorf("Got %v", err)
		}
	})
}

func TestSetupPortConflict(t *testing.T) {
	ports := assertAllocatePorts(t, 3)
	tcs := []struct {
		name     string
		corefile string
	}{
		{
			"dso",
			`
			a.test.:%[1]d {
				dso {
					tcp_port %[3]d
				}
			}
			b.test.:%[2]d {
				dso {
					tcp_port %[3]d
				}
			}`,
		},
		{
			"dns",
			`
			a.test.:%[1]d {
				dso {
					tcp_port %[2]d
				}
			}
			b.test.:%[2]d {
				dso {
					tcp_port %[3]d
				}
			}`,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			_, err := setupCoreDNSf(t, tc.corefile, ports[0], ports[1], ports[2])
			if !errors.Is(err, errAddress) {
				t.Errorf("Got %v, want %v", err, errAddress)
			}
		})
	}
}
