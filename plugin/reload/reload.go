// Package reload periodically checks if the Corefile has changed, and reloads if so.
package reload

import (
	"bytes"
	"crypto/sha512"
	"encoding/json"
	"sync"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/caddy/caddyfile"
)

const (
	unused    = 0
	maybeUsed = 1
	used      = 2
)

type reload struct {
	configHash   [sha512.Size]byte
	dur          time.Duration
	u            int
	mtx          sync.RWMutex
	quit         chan struct{}
	shutdownOnce sync.Once
}

func newReload() *reload {
	return &reload{dur: defaultInterval, quit: make(chan struct{})}
}

func (r *reload) shutdown() error {
	r.shutdownOnce.Do(func() {
		close(r.quit)
		r.removeVersion()
	})
	return nil
}

func reloadForInstance(instance *caddy.Instance) *reload {
	if instance == nil {
		return nil
	}
	instance.StorageMu.RLock()
	defer instance.StorageMu.RUnlock()
	state, _ := instance.Storage[reloadStorageKey{}].(*reload)
	return state
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

func hook(event caddy.EventName, info any) error {
	if event != caddy.InstanceStartupEvent {
		return nil
	}

	// this should be an instance. ok to panic if not
	instance := info.(*caddy.Instance)
	r := reloadForInstance(instance)
	if r == nil || r.usage() == unused {
		return nil
	}
	lastAttemptedHash := r.configHash
	log.Infof("Running configuration SHA512 = %x\n", r.configHash)
	r.publishVersion()
	quit := r.quit
	interval := r.interval()

	go func() {
		tick := time.NewTicker(interval)
		defer tick.Stop()

		for {
			select {
			case <-tick.C:
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
				if s != lastAttemptedHash {
					// Let not try to restart with the same file, even though it is wrong.
					lastAttemptedHash = s
					// now lets consider that plugin will not be reload, unless appear in next config file
					// change status of usage will be reset in setup if the plugin appears in config file
					r.setUsage(maybeUsed)
					// If shutdown is in progress, avoid attempting a restart.
					if shutdownRequested(quit) {
						return
					}
					nextInstance, err := instance.Restart(corefile)
					if err != nil {
						log.Errorf("Corefile changed but reload failed: %s", err)
						failedCount.Add(1)
						continue
					}
					// The startup hook publishes versions for instances with a watcher.
					if next := reloadForInstance(nextInstance); next != nil && next.usage() == unused {
						next.publishVersion()
					}
					// we are done, if the plugin was not set used, then it is not.
					if r.usage() == maybeUsed {
						r.setUsage(unused)
					}
					return
				}
			case <-quit:
				return
			}
		}
	}()

	return nil
}

// shutdownRequested reports whether a shutdown has been requested via quit channel.
// helps with unit testing of the shutdown gate logic.
func shutdownRequested(quit <-chan struct{}) bool {
	select {
	case <-quit:
		return true
	default:
		return false
	}
}
