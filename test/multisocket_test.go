package test

import (
	"fmt"
	"net"
	"testing"

	"github.com/miekg/dns"
)

// These tests need a fixed port, because :0 selects a random port for each socket, but we need all sockets to be on
// the same port.

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
			defer s.Stop()
			if err != nil {
				t.Fatalf("Could not get CoreDNS serving instance: %s", err)
			}
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

func TestMultisocket_Restart(t *testing.T) {
	tests := []struct {
		name             string
		numSocketsBefore int
		numSocketsAfter  int
	}{
		{
			name:             "increase",
			numSocketsBefore: 1,
			numSocketsAfter:  2,
		},
		{
			name:             "decrease",
			numSocketsBefore: 2,
			numSocketsAfter:  1,
		},
		{
			name:             "no changes",
			numSocketsBefore: 2,
			numSocketsAfter:  2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			corefile := `.:5058 {
				multisocket %d
			}`
			srv, err := CoreDNSServer(fmt.Sprintf(corefile, test.numSocketsBefore))
			if err != nil {
				t.Fatalf("Could not get CoreDNS serving instance: %s", err)
			}
			if test.numSocketsBefore != len(srv.Servers()) {
				t.Fatalf("Expected %d servers, got %d", test.numSocketsBefore, len(srv.Servers()))
			}

			newSrv, err := srv.Restart(NewInput(fmt.Sprintf(corefile, test.numSocketsAfter)))
			if err != nil {
				t.Fatalf("Could not get CoreDNS serving instance: %s", err)
			}
			if test.numSocketsAfter != len(newSrv.Servers()) {
				t.Fatalf("Expected %d servers, got %d", test.numSocketsAfter, len(newSrv.Servers()))
			}
			newSrv.Stop()
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
