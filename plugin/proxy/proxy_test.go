package proxy

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/dnstest"

	"github.com/mholt/caddy/caddyfile"
	"github.com/miekg/dns"
)

func TestStop(t *testing.T) {
	config := "proxy . %s {\n health_check %s %dms \n}"
	tests := []struct {
		name                    string
		intervalInMilliseconds  int
		numHealthcheckIntervals int
	}{
		{
			"No Healthchecks After Stop - 5ms, 1 intervals",
			5,
			1,
		},
		{
			"No Healthchecks After Stop - 5ms, 2 intervals",
			5,
			2,
		},
		{
			"No Healthchecks After Stop - 5ms, 3 intervals",
			5,
			3,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			// Set up proxy.
			var counter int64
			backend := dnstest.NewServer(dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
				w.WriteMsg(r)
				atomic.AddInt64(&counter, 1)
			}))

			defer backend.Close()

			corefile := fmt.Sprintf(config, backend.Addr, backend.Addr, test.intervalInMilliseconds)

			c := caddyfile.NewDispenser("Testfile", strings.NewReader(corefile))
			upstreams, err := NewStaticUpstreams(&c)
			if err != nil {
				t.Error("Expected no error. Got:", err.Error())
			}

			// Give some time for healthchecks to hit the server.
			time.Sleep(time.Duration(test.intervalInMilliseconds*test.numHealthcheckIntervals) * time.Millisecond)

			for _, upstream := range upstreams {
				if err := upstream.Stop(); err != nil {
					t.Error("Expected no error stopping upstream. Got: ", err.Error())
				}
			}

			counterValueAfterShutdown := atomic.LoadInt64(&counter)

			// Give some time to see if healthchecks are still hitting the server.
			time.Sleep(time.Duration(test.intervalInMilliseconds*test.numHealthcheckIntervals) * time.Millisecond)

			if counterValueAfterShutdown == 0 {
				t.Error("Expected healthchecks to hit test server. Got no healthchecks.")
			}

			// health checks are in a go routine now, so one may well occur after we shutdown,
			// but we only ever expect one more
			counterValueAfterWaiting := atomic.LoadInt64(&counter)
			if counterValueAfterWaiting > (counterValueAfterShutdown + 1) {
				t.Errorf("Expected no more healthchecks after shutdown. Got: %d healthchecks after shutdown", counterValueAfterWaiting-counterValueAfterShutdown)
			}
		})
	}
}
