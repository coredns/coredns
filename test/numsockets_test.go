package test

import (
	"net"
	"testing"
)

func TestNumsockets(t *testing.T) {
	// this test needs a fixed port, because :0 selects a random port for each socket, but we need all sockets to be on
	// the same port.

	tests := []struct {
		name            string
		corefile        string
		expectedServers int
		expectedErr     string
		expectedPort    string
	}{
		{
			name: "no numsockets",
			corefile: `.:5054 {
			}`,
			expectedServers: 1,
			expectedPort:    "5054",
		},
		{
			name: "numsockets 1",
			corefile: `.:5055 {
				numsockets 1
			}`,
			expectedServers: 1,
			expectedPort:    "5055",
		},
		{
			name: "numsockets 2",
			corefile: `.:5056 {
				numsockets 2
			}`,
			expectedServers: 2,
			expectedPort:    "5056",
		},
		{
			name: "numsockets 100",
			corefile: `.:5057 {
				numsockets 100
			}`,
			expectedServers: 100,
			expectedPort:    "5057",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, err := CoreDNSServer(test.corefile)
			if err != nil {
				t.Fatalf("Could not get CoreDNS serving instance: %s", err)
			}
			// check number of servers
			if len(c.Servers()) != test.expectedServers {
				t.Fatalf("Expected %d servers, got %d", test.expectedServers, len(c.Servers()))
			}

			// check that ports are the same
			for _, listener := range c.Servers() {
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
			c.Stop()
		})
	}
}
