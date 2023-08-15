package reload

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
)

var log = clog.NewWithPlugin("reload")

func init() { plugin.Register("reload", setup) }

// the info reload is global to all application, whatever number of reloads.
// it is used to transmit data between Setup and start of the hook called 'onInstanceStartup'
// WARNING: this data may be unsync after an invalid attempt of reload Corefile.
var (
	r = reload{dur: defaultInterval, u: unused}
)

func setup(c *caddy.Controller) error {
	c.Next() // 'reload'
	args := c.RemainingArgs()

	if len(args) > 2 {
		return plugin.Error("reload", c.ArgErr())
	}

	i := defaultInterval
	if len(args) > 0 {
		d, err := time.ParseDuration(args[0])
		if err != nil {
			return plugin.Error("reload", err)
		}
		i = d
	}
	if i < minInterval {
		return plugin.Error("reload", fmt.Errorf("interval value must be greater or equal to %v", minInterval))
	}

	j := defaultJitter
	if len(args) > 1 {
		d, err := time.ParseDuration(args[1])
		if err != nil {
			return plugin.Error("reload", err)
		}
		j = d
	}
	if j < minJitter {
		return plugin.Error("reload", fmt.Errorf("jitter value must be greater or equal to %v", minJitter))
	}

	if j > i/2 {
		j = i / 2
	}

	jitter := time.Duration(rand.Int63n(j.Nanoseconds()) - (j.Nanoseconds() / 2))
	i = i + jitter

	// prepare info for next onInstanceStartup event
	r.setInterval(i)
	r.setUsage(used)
	registerEventHook("reload", hook)
	return nil
}

const (
	minJitter       = 1 * time.Second
	minInterval     = 2 * time.Second
	defaultInterval = 30 * time.Second
	defaultJitter   = 15 * time.Second
)

// TODO: it would be nicer if github.com/coredns/caddy would expose some method RegisterEventHookIfNotRegistered()
func registerEventHook(name string, hook caddy.EventHook) (changed bool) {
	defer func() {
		if r := recover(); r != nil {
			if s, ok := r.(string); !ok || s != "hook named "+name+" already registered" {
				panic(r)
			}
		}
	}()
	caddy.RegisterEventHook(name, hook)
	changed = true
	return
}
