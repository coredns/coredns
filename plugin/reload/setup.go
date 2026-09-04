package reload

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
)

var log = clog.NewWithPlugin("reload")

func init() { plugin.Register("reload", setup) }

// The event hook is process-global, but reload state belongs to each Caddy instance.
type reloadStorageKey struct{}

func reloadForController(c *caddy.Controller) *reload {
	if state, ok := c.Get(reloadStorageKey{}).(*reload); ok {
		return state
	}

	state := newReload()
	c.Set(reloadStorageKey{}, state)
	// Stop this instance's watcher on both reload and final shutdown.
	c.OnShutdown(state.shutdown)
	return state
}

var once sync.Once

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

	if j != 0 && j < minJitter {
		return plugin.Error("reload", fmt.Errorf("jitter value must be 0 or greater or equal to %v", minJitter))
	}

	if j > 0 && j > i/2 {
		j = i / 2
	}

	if j > 0 {
		jitter := time.Duration(rand.Int63n(j.Nanoseconds()) - (j.Nanoseconds() / 2)) // #nosec G404 -- non-cryptographic jitter.
		i = i + jitter
	}

	// prepare info for next onInstanceStartup event
	state := reloadForController(c)
	state.setInterval(i)
	state.setUsage(used)
	once.Do(func() {
		caddy.RegisterEventHook("reload", hook)
	})
	return nil
}

const (
	minJitter       = 1 * time.Second
	minInterval     = 2 * time.Second
	defaultInterval = 30 * time.Second
	defaultJitter   = 15 * time.Second
)
