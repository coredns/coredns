// Package reload periodically checks if the Corefile has changed, and reloads if so.
package reload

import (
	"bytes"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/caddy/caddyfile"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	unused    = 0
	maybeUsed = 1
	used      = 2
)

type reload struct {
	dur    time.Duration
	u      int
	mtx    sync.RWMutex
	quit   chan struct{} // Quit channel for stopping the goroutine
	tick *time.Ticker  // Ticker to manage the periodic check
}

func (r *reload) setUsage(u int) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.u = u
}

func (r *reload) usage() int {
	r.mtx.RLock()
	defer r.mtx.RUnlock()
	return r.u
}

func (r *reload) setInterval(i time.Duration) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.dur = i
}

func (r *reload) interval() time.Duration {
	r.mtx.RLock()
	defer r.mtx.RUnlock()
	return r.dur
}

func parse(corefile caddy.Input) ([]byte, error) {
	serverBlocks, err := caddyfile.Parse(corefile.Path(), bytes.NewReader(corefile.Body()), nil)
	if err != nil {
		return nil, err
	}
	return json.Marshal(serverBlocks)
}

func hook(event caddy.EventName, info interface{}) error {
	if event != caddy.InstanceStartupEvent {
		return nil
	}
	// if reload is removed from the Corefile, then the hook
	// is still registered but setup is never called again
	// so we need a flag to tell us not to reload
	if r.usage() == unused {
		return nil
	}

	// Stop any previous goroutine if it exists
	if r.quit != nil {
		close(r.quit) // Close the quit channel to stop the old goroutine
	}

	// Create a new quit channel and ticker for the new goroutine
	r.quit = make(chan struct{})
	r.tick = time.NewTicker(r.interval())

	// this should be an instance. ok to panic if not
	instance := info.(*caddy.Instance)
	parsedCorefile, err := parse(instance.Caddyfile())
	if err != nil {
		return err
	}

	sha512sum := sha512.Sum512(parsedCorefile)
	log.Infof("Running configuration SHA512 = %x\n", sha512sum)

	// Start a new goroutine to periodically check for config changes
	go func() {
		defer r.tick.Stop() // Ensure the ticker stops when the goroutine exits

		for {
			select {
			case <-r.tick.C:
				corefile, err := caddy.LoadCaddyfile(instance.Caddyfile().ServerType())
				if err != nil {
					continue
				}
				parsedCorefile, err := parse(corefile)
				if err != nil {
					log.Warningf("Corefile parse failed: %s", err)
					continue
				}
				s := sha512.Sum512(parsedCorefile)
				if s != sha512sum {
					// Configuration has changed, trigger a reload
					reloadInfo.Delete(prometheus.Labels{"hash": "sha512", "value": hex.EncodeToString(sha512sum[:])})
					// Let not try to restart with the same file, even though it is wrong.
					sha512sum = s
					// now lets consider that plugin will not be reload, unless appear in next config file
					// change status of usage will be reset in setup if the plugin appears in config file
					r.setUsage(maybeUsed)
					_, err := instance.Restart(corefile)
					reloadInfo.WithLabelValues("sha512", hex.EncodeToString(sha512sum[:])).Set(1)
					if err != nil {
						log.Errorf("Corefile changed but reload failed: %s", err)
						failedCount.Add(1)
						continue
					}
					// we are done, if the plugin was not set used, then it is not.
					if r.usage() == maybeUsed {
						r.setUsage(unused)
					}
					return
				}
			case <-r.quit:
				return // Quit signal received, stop the goroutine
			}
		}
	}()

	return nil
}
