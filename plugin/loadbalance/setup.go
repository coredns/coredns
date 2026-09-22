package loadbalance

import (
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"

	"github.com/miekg/dns"
)

var log = clog.NewWithPlugin("loadbalance")
var errOpen = errors.New("weight file open error")

func init() { plugin.Register("loadbalance", setup) }

type lbFuncs struct {
	shuffleFunc    func(*dns.Msg) *dns.Msg
	onStartUpFunc  func() error
	onShutdownFunc func() error
	weighted       *weightedRR // used in unit tests only
	preferSubnets  []*net.IPNet
}

func setup(c *caddy.Controller) error {
	//shuffleFunc, startUpFunc, shutdownFunc, err := parse(c)
	lb, err := parse(c)
	if err != nil {
		return plugin.Error("loadbalance", err)
	}
	if lb.onStartUpFunc != nil {
		c.OnStartup(lb.onStartUpFunc)
	}
	if lb.onShutdownFunc != nil {
		c.OnShutdown(lb.onShutdownFunc)
	}

	shuffle := lb.shuffleFunc
	if len(lb.preferSubnets) > 0 {
		original := shuffle
		shuffle = func(res *dns.Msg) *dns.Msg {
			return reorderPreferredSubnets(original(res), lb.preferSubnets)
		}
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		return LoadBalance{Next: next, shuffle: shuffle}
	})

	return nil
}

func parse(c *caddy.Controller) (*lbFuncs, error) {
	config := dnsserver.GetConfig(c)
	lb := &lbFuncs{}

	for c.Next() {
		args := c.RemainingArgs()
		if len(args) == 0 {
			lb.shuffleFunc = randomShuffle
		} else {
			switch args[0] {
			case ramdomShufflePolicy:
				if len(args) > 1 {
					return nil, c.Errf("unknown property for %s", args[0])
				}
				lb.shuffleFunc = randomShuffle

			case weightedRoundRobinPolicy:
				if len(args) < 2 {
					return nil, c.Err("missing weight file argument")
				}
				if len(args) > 2 {
					return nil, c.Err("unexpected argument(s)")
				}
				weightFileName := args[1]
				if !filepath.IsAbs(weightFileName) && config.Root != "" {
					weightFileName = filepath.Join(config.Root, weightFileName)
				}
				reload := 30 * time.Second
				for c.NextBlock() {
					switch c.Val() {
					case "reload":
						t := c.RemainingArgs()
						if len(t) < 1 {
							return nil, c.Err("reload duration value is missing")
						}
						if len(t) > 1 {
							return nil, c.Err("unexpected argument")
						}
						var err error
						reload, err = time.ParseDuration(t[0])
						if err != nil {
							return nil, c.Errf("invalid reload duration '%s'", t[0])
						}
					default:
						return nil, c.Errf("unknown property '%s'", c.Val())
					}
				}
				*lb = *createWeightedFuncs(weightFileName, reload)
			default:
				return nil, fmt.Errorf("unknown policy: %s", args[0])
			}
		}

		for c.NextBlock() {
			switch c.Val() {
			case "prefer":
				cidrs := c.RemainingArgs()
				for _, cidr := range cidrs {
					_, subnet, err := net.ParseCIDR(cidr)
					if err != nil {
						return nil, c.Errf("invalid CIDR %q: %v", cidr, err)
					}
					lb.preferSubnets = append(lb.preferSubnets, subnet)
				}
			default:
				return nil, c.Errf("unknown property '%s'", c.Val())
			}
		}
	}

	return lb, nil
}
