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

func setup(c *caddy.Controller) error {
	log.Debug("setup called")
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

	if r := c.Get("reload"); r != nil {
		r.(*reload).interval = i
	} else {
		r := &reload{interval: i, quit: make(chan bool, 1)}
		c.Set("reload", r)
		c.OnShutdown(func() error {
			select {
			case r.quit <- true:
			default:
			}
			return nil
		})
		caddy.RegisterOrUpdateEventHook("reload", hook)
	}
	return nil
}

const (
	minJitter       = 1 * time.Second
	minInterval     = 2 * time.Second
	defaultInterval = 30 * time.Second
	defaultJitter   = 15 * time.Second
)
