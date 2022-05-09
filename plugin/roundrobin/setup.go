package roundrobin

import (
	"fmt"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/roundrobin/internal/strategy"
)

const (
	pluginName = "roundrobin"
)

var log = clog.NewWithPlugin(pluginName)

func init() { plugin.Register(pluginName, setup) }

func setup(c *caddy.Controller) error {
	strategy, err := parse(c)
	if err != nil {
		return plugin.Error(pluginName, err)
	}
	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		return New(next, strategy)
	})
	return nil
}

func parse(c *caddy.Controller) (strategy.Shuffler, error) {
	for c.Next() {
		args := c.RemainingArgs()
		if len(args) == 0 {
			return strategy.NewStateful(), nil
		}
		switch args[0] {
		case strategyStateless:
			return strategy.NewStateless(), nil
		case strategyWeight:
			return nil, fmt.Errorf("not implemented %s", args[0])
		case strategyRandom:
			return strategy.NewRandom(), nil
		case strategyStateful:
			return strategy.NewStateful(), nil
		}
	}
	return nil, fmt.Errorf("unknown roundrobin type")
}
