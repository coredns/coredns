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
	sync.Mutex
	dur  time.Duration
	u    int
	s    [64]byte
	mtx  sync.RWMutex
	quit chan bool
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

	// this should be an instance. ok to panic if not
	instance := info.(*caddy.Instance)

	go func() {
		log.Debug("Starting reload handler (waiting for previous handler to finish) ...")
		r.Lock()
		defer r.Unlock()
		log.Debug("Reload handler running")

		if r.s == [64]byte{} {
			log.Info("Running for the first time")
		} else {
			log.Infof("Running configuration SHA512 = %x\n", r.s)
		}

		tick := time.NewTicker(r.interval())
		defer tick.Stop()

		for {
			select {
			case <-tick.C:
				corefile, err := caddy.LoadCaddyfile(instance.Caddyfile().ServerType())
				if err != nil {
					log.Warningf("Corefile load failed: %s", err)
					continue
				}
				parsedCorefile, err := parse(corefile)
				if err != nil {
					log.Warningf("Corefile parse failed: %s", err)
					continue
				}
				s := sha512.Sum512(parsedCorefile)
				if s != r.s {
					reloadInfo.Delete(prometheus.Labels{"hash": "sha512", "value": hex.EncodeToString(r.s[:])})
					// Let not try to restart with the same file, even though it is wrong.
					r.s = s
					// now lets consider that plugin will not be reload, unless appear in next config file
					// change status of usage will be reset in setup if the plugin appears in config file
					r.setUsage(maybeUsed)
					_, err := instance.Restart(corefile)
					reloadInfo.WithLabelValues("sha512", hex.EncodeToString(r.s[:])).Set(1)
					if err != nil {
						log.Errorf("Corefile changed but reload failed: %s", err)
						failedCount.Add(1)
						continue
					}
					// we are done, if the plugin was not set used, then it is not.
					if r.usage() == maybeUsed {
						r.setUsage(unused)
					}
					log.Debug("Reload handler exiting (due to successful reload)")
					return
				}
			case <-r.quit:
				log.Debug("Reload handler exiting (forced)")
				return
			}
		}
	}()

	return nil
}
