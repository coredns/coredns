package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnsutil"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/upstream"

	"github.com/go-logr/logr"
	"github.com/miekg/dns"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc" // pull this in here, because we want it excluded if plugin.cfg doesn't have k8s
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

const pluginName = "kubernetes"

var log = clog.NewWithPlugin(pluginName)

func init() { plugin.Register(pluginName, setup) }

func setup(c *caddy.Controller) error {
	// Do not call klog.InitFlags(nil) here.  It will cause reload to panic.
	klog.SetLogger(logr.New(&loggerAdapter{log}))

	k, err := kubernetesParse(c)
	if err != nil {
		return plugin.Error(pluginName, err)
	}

	onStart, onShut, err := k.InitKubeCache(context.Background())
	if err != nil {
		return plugin.Error(pluginName, err)
	}
	if onStart != nil {
		c.OnStartup(onStart)
	}
	if onShut != nil {
		c.OnShutdown(onShut)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		k.Next = next
		return k
	})

	// get locally bound addresses
	c.OnStartup(func() error {
		k.localIPs = boundIPs(c)
		return nil
	})

	return nil
}

func kubernetesParse(c *caddy.Controller) (*Kubernetes, error) {
	var (
		k8s *Kubernetes
		err error
	)

	i := 0
	for c.Next() {
		if i > 0 {
			return nil, plugin.ErrOnce
		}
		i++

		k8s, err = ParseStanza(c)
		if err != nil {
			return k8s, err
		}
	}
	return k8s, nil
}

// ParseStanza parses a kubernetes stanza
func ParseStanza(c *caddy.Controller) (*Kubernetes, error) {
	k8s := New([]string{""})
	k8s.autoPathSearch = searchFromResolvConf()

	opts := dnsControlOpts{
		initEndpointsCache: true,
		ignoreEmptyService: false,
	}
	k8s.opts = opts

	k8s.Zones = plugin.OriginsFromArgsOrServerBlock(c.RemainingArgs(), c.ServerBlockKeys)

	k8s.primaryZoneIndex = -1
	for i, z := range k8s.Zones {
		if dnsutil.IsReverse(z) > 0 {
			continue
		}
		k8s.primaryZoneIndex = i
		break
	}

	if k8s.primaryZoneIndex == -1 {
		return nil, errors.New("non-reverse zone name must be used")
	}

	k8s.Upstream = upstream.New()

	k8s.startupTimeout = time.Second * 5
	for c.NextBlock() {
		switch c.Val() {
		case "endpoint_pod_names":
			args := c.RemainingArgs()
			if len(args) > 0 {
				return nil, c.ArgErr()
			}
			k8s.endpointNameMode = true
			continue
		case "pods":
			args := c.RemainingArgs()
			if len(args) == 1 {
				switch args[0] {
				case podModeDisabled, podModeInsecure, podModeVerified:
					k8s.podMode = args[0]
				default:
					return nil, fmt.Errorf("wrong value for pods: %s,  must be one of: disabled, verified, insecure", args[0])
				}
				continue
			}
			return nil, c.ArgErr()
		case "namespaces":
			args := c.RemainingArgs()
			if len(args) > 0 {
				for _, a := range args {
					k8s.Namespaces[a] = struct{}{}
				}
				continue
			}
			return nil, c.ArgErr()
		case "endpoint":
			args := c.RemainingArgs()
			if len(args) > 0 {
				// Multiple endpoints are deprecated but still could be specified,
				// only the first one be used, though
				k8s.APIServerList = args
				if len(args) > 1 {
					log.Warningf("Multiple endpoints have been deprecated, only the first specified endpoint '%s' is used", args[0])
				}
				continue
			}
			return nil, c.ArgErr()
		case "tls": // cert key cacertfile
			args := c.RemainingArgs()
			if len(args) == 3 {
				k8s.APIClientCert, k8s.APIClientKey, k8s.APICertAuth = args[0], args[1], args[2]
				continue
			}
			return nil, c.ArgErr()
		case "labels":
			args := c.RemainingArgs()
			if len(args) > 0 {
				labelSelectorString := strings.Join(args, " ")
				ls, err := meta.ParseToLabelSelector(labelSelectorString)
				if err != nil {
					return nil, fmt.Errorf("unable to parse label selector value: '%v': %v", labelSelectorString, err)
				}
				k8s.opts.labelSelector = ls
				continue
			}
			return nil, c.ArgErr()
		case "namespace_labels":
			args := c.RemainingArgs()
			if len(args) > 0 {
				namespaceLabelSelectorString := strings.Join(args, " ")
				nls, err := meta.ParseToLabelSelector(namespaceLabelSelectorString)
				if err != nil {
					return nil, fmt.Errorf("unable to parse namespace_label selector value: '%v': %v", namespaceLabelSelectorString, err)
				}
				k8s.opts.namespaceLabelSelector = nls
				continue
			}
			return nil, c.ArgErr()
		case "fallthrough":
			k8s.Fall.SetZonesFromArgs(c.RemainingArgs())
		case "ttl":
			args := c.RemainingArgs()
			if len(args) == 0 {
				return nil, c.ArgErr()
			}
			t, err := strconv.Atoi(args[0])
			if err != nil {
				return nil, err
			}
			if t < 0 || t > 3600 {
				return nil, c.Errf("ttl must be in range [0, 3600]: %d", t)
			}
			k8s.ttl = uint32(t)
		case "noendpoints":
			if len(c.RemainingArgs()) != 0 {
				return nil, c.ArgErr()
			}
			k8s.opts.initEndpointsCache = false
		case "ignore":
			args := c.RemainingArgs()
			if len(args) > 0 {
				ignore := args[0]
				if ignore == "empty_service" {
					k8s.opts.ignoreEmptyService = true
					continue
				}
				return nil, fmt.Errorf("unable to parse ignore value: '%v'", ignore)
			}
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
			k8s.ClientConfig = config
		case "multicluster":
			k8s.opts.multiclusterZones = plugin.OriginsFromArgsOrServerBlock(c.RemainingArgs(), []string{})
		case "startup_timeout":
			args := c.RemainingArgs()
			if len(args) == 0 {
				return nil, c.ArgErr()
			} else {
				var err error
				k8s.startupTimeout, err = time.ParseDuration(args[0])
				if err != nil {
					return nil, fmt.Errorf("failed to parse startup_timeout: %v, %s", args[0], err)
				}
			}
		default:
			return nil, c.Errf("unknown property '%s'", c.Val())
		}
	}

	if len(k8s.Namespaces) != 0 && k8s.opts.namespaceLabelSelector != nil {
		return nil, c.Errf("namespaces and namespace_labels cannot both be set")
	}

	for _, multiclusterZone := range k8s.opts.multiclusterZones {
		if !slices.Contains(k8s.Zones, multiclusterZone) {
			fmt.Println(k8s.Zones)
			return nil, c.Errf("is not authoritative for the multicluster zone %s", multiclusterZone)
		}
	}

	return k8s, nil
}

func searchFromResolvConf() []string {
	rc, err := dns.ClientConfigFromFile("/etc/resolv.conf")
	if err != nil {
		return nil
	}
	plugin.Zones(rc.Search).Normalize()
	return rc.Search
}
