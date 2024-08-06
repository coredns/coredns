package tls

import (
	"github.com/coredns/caddy"
)

type renewCert struct {
	quit  chan bool
	renew chan bool
}

// restarting CoreDNS is necessary when a cert is to be renewed
func hook(event caddy.EventName, info interface{}) error {
	if event != caddy.InstanceStartupEvent {
		return nil
	}

	// this should be an instance. ok to panic if not
	instance := info.(*caddy.Instance)

	go func() {
		for {
			select {
			case <-r.renew:
				corefile, err := caddy.LoadCaddyfile(instance.Caddyfile().ServerType())
				if err != nil {
					continue
				}
				_, err = instance.Restart(corefile)
				if err != nil {
					log.Errorf("Error during Restart: %v, \n", err)
				}
				return
			case <-r.quit:
				log.Debug("Received quit signal, stopping certificate renewal")
				return
			}
		}
	}()
	return nil
}
