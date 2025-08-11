package test

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// pickPort returns a free TCP port on 127.0.0.1 and closes the probe listener.
func pickPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("probe listen failed: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestMultisocket(t *testing.T) {
	tests := []struct {
		name            string
		corefile        string
		expectedServers int
		expectedErr     string
		expectedPort    string
	}{
		{
			name: "no multisocket",
			corefile: `.:5054 {
			}`,
			expectedServers: 1,
			expectedPort:    "5054",
		},
		{
			name: "multisocket 1",
			corefile: `.:5055 {
				multisocket 1
			}`,
			expectedServers: 1,
			expectedPort:    "5055",
		},
		{
			name: "multisocket 2",
			corefile: `.:5056 {
				multisocket 2
			}`,
			expectedServers: 2,
			expectedPort:    "5056",
		},
		{
			name: "multisocket 100",
			corefile: `.:5057 {
				multisocket 100
			}`,
			expectedServers: 100,
			expectedPort:    "5057",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s, err := CoreDNSServer(test.corefile)
			if err != nil {
				t.Fatalf("Could not get CoreDNS serving instance: %s", err)
			}
			defer s.Stop()

			// check number of servers
			if len(s.Servers()) != test.expectedServers {
				t.Fatalf("Expected %d servers, got %d", test.expectedServers, len(s.Servers()))
			}

			// check that ports are the same
			for _, listener := range s.Servers() {
				if listener.Addr().String() != listener.LocalAddr().String() {
					t.Fatalf("Expected tcp address %s to be on the same port as udp address %s",
						listener.LocalAddr().String(), listener.Addr().String())
				}
				_, port, err := net.SplitHostPort(listener.Addr().String())
				if err != nil {
					t.Fatalf("Could not get port from listener addr: %s", err)
				}
				if port != test.expectedPort {
					t.Fatalf("Expected port %s, got %s", test.expectedPort, port)
				}
			}
		})
	}
}

// NOTE: restart uses a different port to avoid transient EADDRINUSE / shutdown races
// when TCP/UDP from the previous instance haven’t fully torn down yet.
func TestMultisocket_Restart(t *testing.T) {
	tests := []struct {
		name             string
		numSocketsBefore int
		numSocketsAfter  int
	}{
		{name: "increase", numSocketsBefore: 1, numSocketsAfter: 2},
		{name: "decrease", numSocketsBefore: 2, numSocketsAfter: 1},
		{name: "no changes", numSocketsBefore: 2, numSocketsAfter: 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			port1 := pickPort(t)
			port2 := pickPort(t) // restart onto a different free port
			coreTmpl := `.:%d {
				multisocket %d
			}`

			srv, err := CoreDNSServer(fmt.Sprintf(coreTmpl, port1, tc.numSocketsBefore))
			if err != nil {
				t.Fatalf("Could not get CoreDNS serving instance: %v", err)
			}
			if got := len(srv.Servers()); got != tc.numSocketsBefore {
				t.Fatalf("Expected %d servers, got %d", tc.numSocketsBefore, got)
			}

			resultCh := make(chan int, 1)
			errCh := make(chan error, 1)
			stopCh := make(chan struct{})

			// Do the restart in a goroutine; return only the server count.
			go func() {
				newSrv, rerr := srv.Restart(NewInput(fmt.Sprintf(coreTmpl, port2, tc.numSocketsAfter)))
				if rerr != nil {
					errCh <- rerr
					return
				}
				resultCh <- len(newSrv.Servers())
				<-stopCh
				newSrv.Stop()
			}()

			select {
			case got := <-resultCh:
				if got != tc.numSocketsAfter {
					close(stopCh) // still stop the new instance
					t.Fatalf("Expected %d servers, got %d", tc.numSocketsAfter, got)
				}
				close(stopCh) // now safe to stop the new instance
			case rerr := <-errCh:
				// Restart failed; stop the original instance.
				srv.Stop()
				t.Fatalf("Restart failed: %v", rerr)
			case <-time.After(30 * time.Second):
				// Timeout; stop the original instance.
				srv.Stop()
				t.Fatalf("Restart timed out after 30s (ports :%d→:%d, %d→%d sockets)",
					port1, port2, tc.numSocketsBefore, tc.numSocketsAfter)
			}
		})
	}
}

// Just check that server with multisocket works
func TestMultisocket_WhoAmI(t *testing.T) {
	corefile := `.:5059 {
		multisocket
		whoami
	}`
	s, udp, tcp, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer s.Stop()

	m := new(dns.Msg)
	m.SetQuestion("whoami.example.org.", dns.TypeA)

	// check udp
	cl := dns.Client{Net: "udp"}
	udpResp, err := dns.Exchange(m, udp)
	if err != nil {
		t.Fatalf("Expected to receive reply, but didn't: %v", err)
	}
	// check tcp
	cl.Net = "tcp"
	tcpResp, _, err := cl.Exchange(m, tcp)
	if err != nil {
		t.Fatalf("Expected to receive reply, but didn't: %v", err)
	}

	for _, resp := range []*dns.Msg{udpResp, tcpResp} {
		if resp.Rcode != dns.RcodeSuccess {
			t.Fatalf("Expected RcodeSuccess, got %v", resp.Rcode)
		}
		if len(resp.Extra) != 2 {
			t.Errorf("Expected 2 RRs in additional section, got %d", len(resp.Extra))
		}
	}
}
