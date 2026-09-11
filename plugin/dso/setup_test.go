package dso

import (
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
)

func setupCoreDNSf(tb testing.TB, format string, a ...any) (*caddy.Instance, error) {
	tb.Helper()

	caddy.Quiet = true
	dnsserver.Quiet = true

	input := caddy.CaddyfileInput{
		Filepath:       "Testfile",
		ServerTypeName: "dns",
		Contents:       fmt.Appendf(nil, format, a...),
	}
	inst, err := caddy.Start(input)
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

func TestSetupShared(t *testing.T) {
	ports := assertAllocatePorts(t, 3)
	tcs := []struct {
		name    string
		input   string
		wantErr error
	}{
		{
			"same ports",
			`
a.test.:%[1]d b.test.:%[1]d {
	dso {
		tcp_port %[3]d
	}
}`,
			nil,
		},
		{
			"different ports",
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
			_, err := setupCoreDNSf(t, tc.input, ports[0], ports[1], ports[2])
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("Got %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestSetupDefenition(t *testing.T) {
	ports := assertAllocatePorts(t, 4)
	tcs := []struct {
		name    string
		input   string
		wantErr error
	}{
		{
			"same addresses",
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
			"different addresses",
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
			_, err := setupCoreDNSf(t, tc.input, ports[0], ports[1], ports[2], ports[3])
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("Got %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestSetupSentinel(t *testing.T) {
	ports := assertAllocatePorts(t, 2)
	input := `
a.test.:%[1]d {
	dso {
		tcp_port %[2]d
	}
}

b.test.:%[1]d {
	dso
}`
	_, err := setupCoreDNSf(t, input, ports[0], ports[1])
	if err != nil {
		t.Errorf("Got %v", err)
	}
}
