package health

import (
	"net/http"
	"sync"
	"time"

)

// overloaded queries the health end point and updates a metrics showing how long it took.
func (h *health) overloaded() {
	timeout := time.Duration(5 * time.Second)
	client := http.Client{
		Timeout: timeout,
	}
	url := "http://" + h.Addr
	tick := time.NewTicker(1 * time.Second)

	for {
		select {
		case <-tick.C:
			start := time.Now()
			resp, err := client.Get(url)
			if err != nil {
				h.metric.Duration.Observe(timeout.Seconds())
				continue
			}
			resp.Body.Close()
			h.metric.Duration.Observe(time.Since(start).Seconds())

		case <-h.stop:
			tick.Stop()
			return
		}
	}
}

var once sync.Once
