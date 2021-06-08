package forwardcrd

import (
	"context"
	"os"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/dnstap"
	clog "github.com/coredns/coredns/plugin/pkg/log"

	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

const pluginName = "forwardcrd"

var log = clog.NewWithPlugin(pluginName)

func init() {
	plugin.Register(pluginName, setup)
}

func setup(c *caddy.Controller) error {
	klog.SetOutput(os.Stdout)

	k, err := parseForwardCRD(c)
	if err != nil {
		return plugin.Error(pluginName, err)
	}

	err = k.InitKubeCache(context.Background())
	if err != nil {
		return plugin.Error(pluginName, err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		k.Next = next
		return k
	})

	c.OnStartup(func() error {
		go k.APIConn.Run(1)

		timeout := time.After(5 * time.Second)
		ticker := time.NewTicker(100 * time.Millisecond)
		for {
			select {
			case <-ticker.C:
				if k.APIConn.HasSynced() {
					return nil
				}
			case <-timeout:
				return nil
			}
		}
	})

	c.OnStartup(func() error {
		if taph := dnsserver.GetConfig(c).Handler("dnstap"); taph != nil {
			if tapPlugin, ok := taph.(dnstap.Dnstap); ok {
				k.APIConn.(*forwardCRDControl).tapPlugin = &tapPlugin
			}
		}
		return nil
	})

	c.OnShutdown(func() error {
		return k.APIConn.Stop()
	})

	return nil
}

func parseForwardCRD(c *caddy.Controller) (*ForwardCRD, error) {
	var (
		k   *ForwardCRD
		err error
		i   int
	)

	for c.Next() {
		if i > 0 {
			return nil, plugin.ErrOnce
		}
		i++
		k, err = parseStanza(c)
		if err != nil {
			return nil, err
		}
	}

	return k, nil
}

func parseStanza(c *caddy.Controller) (*ForwardCRD, error) {
	k := New()

	args := c.RemainingArgs()
	k.Zones = plugin.OriginsFromArgsOrServerBlock(args, c.ServerBlockKeys)

	for c.NextBlock() {
		switch c.Val() {
		case "endpoint":
			args := c.RemainingArgs()
			if len(args) != 1 {
				return nil, c.ArgErr()
			}
			k.APIServerEndpoint = args[0]
		case "tls":
			args := c.RemainingArgs()
			if len(args) != 3 {
				return nil, c.ArgErr()
			}
			k.APIClientCert, k.APIClientKey, k.APICertAuth = args[0], args[1], args[2]
		case "kubeconfig":
			args := c.RemainingArgs()
			if len(args) != 1 && len(args) != 2 {
				return nil, c.ArgErr()
			}
			overrides := &clientcmd.ConfigOverrides{}
			if len(args) == 2 {
				overrides.CurrentContext = args[1]
			}
			config := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
				&clientcmd.ClientConfigLoadingRules{ExplicitPath: args[0]},
				overrides,
			)
			k.ClientConfig = config
		case "namespace":
			args := c.RemainingArgs()
			if len(args) == 0 {
				k.Namespace = ""
			} else if len(args) == 1 {
				k.Namespace = args[0]
			} else {
				return nil, c.ArgErr()
			}
		default:
			return nil, c.Errf("unknown property '%s'", c.Val())
		}
	}

	return k, nil
}
