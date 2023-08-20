// Package reload periodically checks if the Corefile has changed, and reloads if so.
package reload

import (
	"encoding/hex"
	"time"

	"github.com/coredns/caddy"

	"github.com/prometheus/client_golang/prometheus"
)

type reload struct {
	interval time.Duration
	quit     chan bool
}

func hook(event caddy.EventName, info interface{}) error {
	if event != caddy.InstanceStartupEvent {
		return nil
	}

	// this should be an instance. ok to panic if not
	instance := info.(*caddy.Instance)

	// fetch reload data from instance storage (if reload is used)
	var r *reload
	if v, ok := instance.Storage["reload"]; ok {
		log.Debug("Reload plugin used")
		// the following cast should always work
		r = v.(*reload)
	} else {
		log.Debug("Reload plugin not used")
		return nil
	}

	// start reload handler
	go func() {
		log.Infof("Running configuration SHA512 = %x\n", instance.ConfigDigest)

		tick := time.NewTicker(r.interval)
		defer tick.Stop()

		for {
			select {
			case <-tick.C:
				corefile, err := caddy.LoadCaddyfile(instance.Caddyfile().ServerType())
				if err != nil {
					log.Warningf("Corefile load failed: %s", err)
					continue
				}
				configDigest, err := caddy.ConfigDigest(corefile)
				if err != nil {
					log.Warningf("Corefile parse failed: %s", err)
					continue
				}
				if configDigest != instance.ConfigDigest {
					reloadInfo.Delete(prometheus.Labels{"hash": "sha512", "value": hex.EncodeToString(instance.ConfigDigest[:])})
					_, err := instance.Restart(corefile)
					reloadInfo.WithLabelValues("sha512", hex.EncodeToString(configDigest[:])).Set(1)
					if err != nil {
						log.Errorf("Corefile changed but reload failed: %s", err)
						failedCount.Add(1)
						continue
					}
					log.Debug("Reload handler exiting (due to successful reload)")
					return
				}
			case <-r.quit:
				log.Debug("Reload handler exiting (due to shutdown)")
				return
			}
		}
	}()

	return nil
}
