package test

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadyChecksBothInstancesOfPlugin(t *testing.T) {
	for _, tc := range []struct {
		name          string
		separatePorts bool
		firstReady    [2]int
		secondReady   [2]int
	}{
		{
			name:        "shared endpoint",
			firstReady:  [2]int{503, 503},
			secondReady: [2]int{503, 503},
		},
		{
			name:          "separate endpoints",
			separatePorts: true,
			firstReady:    [2]int{200, 503},
			secondReady:   [2]int{503, 200},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			firstPort := pickPort(t)
			secondPort := firstPort
			if tc.separatePorts {
				for secondPort == firstPort {
					secondPort = pickPort(t)
				}
			}
			testReadyEndpoints(t, [2]int{firstPort, secondPort}, tc.firstReady, tc.secondReady)
		})
	}
}

func testReadyEndpoints(t *testing.T, ports, firstReady, secondReady [2]int) {
	t.Helper()
	corefile := fmt.Sprintf(`first.example.org:0 {
		bind 127.0.0.1
		ready 127.0.0.1:%d {
			monitor continuously
		}
		erratic {
			drop 0
		}
	}
	second.example.org:0 {
		bind 127.0.0.1
		ready 127.0.0.1:%d {
			monitor until-ready
		}
		erratic {
			drop 0
		}
	}`, ports[0], ports[1])

	instance, udp, _, err := CoreDNSServerAndPorts(corefile)
	require.NoError(t, err)
	t.Cleanup(func() {
		for _, err := range instance.ShutdownCallbacks() {
			assert.NoError(t, err)
		}
		assert.NoError(t, instance.Stop())
	})

	client := http.Client{Timeout: time.Second}
	probe := func(statuses [2]int) {
		t.Helper()
		for i, port := range ports {
			response, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/ready", port))
			require.NoError(t, err)
			body, err := io.ReadAll(response.Body)
			response.Body.Close()
			require.NoError(t, err)
			assert.Equal(t, statuses[i], response.StatusCode, "port %d", port)
			wantBody := "erratic"
			if statuses[i] == http.StatusOK {
				wantBody = "OK"
			}
			assert.Equal(t, wantBody, string(body), "port %d", port)
		}
	}
	query := func(name string, count int) {
		t.Helper()
		client := dns.Client{Timeout: time.Second}
		for range count {
			message := new(dns.Msg).SetQuestion(name, dns.TypeA)
			response, _, err := client.Exchange(message, udp)
			require.NoError(t, err)
			require.Equal(t, dns.RcodeSuccess, response.Rcode)
		}
	}

	// Erratic reports ready after three queries and unready after five.
	probe([2]int{503, 503})
	query("first.example.org.", 3)
	probe(firstReady)
	query("second.example.org.", 3)
	probe([2]int{200, 200})
	query("second.example.org.", 2)
	probe([2]int{200, 200})
	query("first.example.org.", 2)
	probe(secondReady)

	invalid := strings.Replace(corefile, "monitor continuously", "monitor invalid", 1)
	_, err = instance.Restart(NewInput(invalid))
	require.ErrorContains(t, err, "monitor type 'invalid' not supported")
	probe(secondReady)
}
