package reload

import (
	"fmt"
	"math/rand"
	"slices"
	"sync"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
)

var log = clog.NewWithPlugin("reload")

func init() {
	plugin.Register("reload", setup)
	// A successful reload can remove the directive. Prepare version tracking for
	// that instance before startup so its shutdown can retire the final hash.
	caddy.RegisterParsingCallback("dns", "reload", func(ctx caddy.Context) error {
		reloadForContext(ctx)
		return nil
	})
}

// The event hook is process-global, but reload state belongs to each Caddy instance.
type reloadStorageKey struct{}

func reloadForContext(ctx caddy.Context) *reload {
	instance, hash := dnsserver.GetInstanceConfigHash(ctx)
	instance.StorageMu.Lock()
	defer instance.StorageMu.Unlock()
	if state, ok := instance.Storage[reloadStorageKey{}].(*reload); ok {
		return state
	}

	state := newReload()
	state.configHash = hash
	instance.Storage[reloadStorageKey{}] = state
	// Stop this instance's watcher on both reload and final shutdown.
	instance.OnShutdown = append(instance.OnShutdown, state.shutdown)
	return state
}

var eventHookMu sync.Mutex

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
	state := reloadForContext(c.Context())
	state.setInterval(i)
	state.setUsage(used)
	registerEventHook()
	return nil
}

// Caddy removes all event hooks before a SIGUSR1 restart. Register the hook
// whenever the directive is set up so that it is restored for that restart,
// while avoiding duplicate registration during ordinary reloads.
func registerEventHook() {
	eventHookMu.Lock()
	defer eventHookMu.Unlock()

	if slices.Contains(caddy.ListPlugins()["event_hooks"], "reload") {
		return
	}
	caddy.RegisterEventHook("reload", hook)
}

const (
	minJitter       = 1 * time.Second
	minInterval     = 2 * time.Second
	defaultInterval = 30 * time.Second
	defaultJitter   = 15 * time.Second
)
