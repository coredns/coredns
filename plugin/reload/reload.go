package reload

import (
	"crypto/md5"
	"log"
	"time"

	"github.com/mholt/caddy"
)

// reload periodically checks if the Corefile has changed, and reloads if so

type reload struct {
	interval time.Duration
	stopped  bool
	quit     chan bool
}

func hook(event caddy.EventName, info interface{}) error {
	if event != caddy.InstanceStartupEvent {
		return nil
	}

	// if reload is removed from the Corefile, then the hook
	// is still registered but setup is never called again
	// so we need a flag to tell us not to reload
	if r.stopped {
		return nil
	}

	// this should be an instance. ok to panic if not
	instance := info.(*caddy.Instance)
	md5sum := md5.Sum(instance.Caddyfile().Body())

	go func() {
		tick := time.NewTicker(r.interval)

		for {
			select {
			case <-tick.C:
				corefile, err := caddy.LoadCaddyfile(instance.Caddyfile().ServerType())
				if err != nil {
					continue
				}
				s := md5.Sum(corefile.Body())
				if s != md5sum {
					// Let not try to restart with the same file, even though it is wrong.
					md5sum = s
					// now lets consider that plugin will not be reload, unless appear in next config file
					// r.stopped will be reset in setup if the plugin appears in config file
					r.stopped = true
					_, err := instance.Restart(corefile)
					if err != nil {
						log.Printf("[ERROR] Corefile changed but reload failed: %s\n", err)
						continue
					}
					// we are done, this hook gets called again with new instance
					return
				}
			case <-r.quit:
				return
			}
		}
	}()

	return nil
}
