package kubernetes

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/middleware"
	"github.com/coredns/coredns/middleware/pkg/dnsutil"
	"github.com/coredns/coredns/middleware/proxy"

	"github.com/mholt/caddy"
	unversionedapi "k8s.io/client-go/1.5/pkg/api/unversioned"
)

func init() {
	caddy.RegisterPlugin("kubernetes", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func setup(c *caddy.Controller) error {
	kubernetes, err := kubernetesParse(c)
	if err != nil {
		return middleware.Error("kubernetes", err)
	}

	err = kubernetes.InitKubeCache()
	if err != nil {
		return middleware.Error("kubernetes", err)
	}

	// Register KubeCache start and stop functions with Caddy
	c.OnStartup(func() error {
		go kubernetes.APIConn.Run()
		return nil
	})

	c.OnShutdown(func() error {
		return kubernetes.APIConn.Stop()
	})

	dnsserver.GetConfig(c).AddMiddleware(func(next middleware.Handler) middleware.Handler {
		kubernetes.Next = next
		return kubernetes
	})

	return nil
}

func kubernetesParse(c *caddy.Controller) (*Kubernetes, error) {
	k8s := &Kubernetes{
		ResyncPeriod:   defaultResyncPeriod,
		interfaceAddrs: &interfaceAddrs{},
		PodMode:        PodModeDisabled,
	}

	for c.Next() {
		if c.Val() == "kubernetes" {
			zones := c.RemainingArgs()

			if len(zones) == 0 {
				k8s.Zones = make([]string, len(c.ServerBlockKeys))
				copy(k8s.Zones, c.ServerBlockKeys)
			}

			k8s.Zones = NormalizeZoneList(zones)
			middleware.Zones(k8s.Zones).Normalize()

			if k8s.Zones == nil || len(k8s.Zones) < 1 {
				return nil, errors.New("zone name must be provided for kubernetes middleware")
			}

			k8s.primaryZone = -1
			for i, z := range k8s.Zones {
				if strings.HasSuffix(z, "in-addr.arpa.") || strings.HasSuffix(z, "ip6.arpa.") {
					continue
				}
				k8s.primaryZone = i
				break
			}

			if k8s.primaryZone == -1 {
				return nil, errors.New("non-reverse zone name must be given for Kubernetes")
			}

			for c.NextBlock() {
				switch c.Val() {
				case "cidrs":
					args := c.RemainingArgs()
					if len(args) > 0 {
						for _, cidrStr := range args {
							_, cidr, err := net.ParseCIDR(cidrStr)
							if err != nil {
								return nil, fmt.Errorf("invalid cidr: %s", cidrStr)
							}
							k8s.ReverseCidrs = append(k8s.ReverseCidrs, *cidr)

						}
						continue
					}
					return nil, c.ArgErr()
				case "pods":
					args := c.RemainingArgs()
					if len(args) == 1 {
						switch args[0] {
						case PodModeDisabled, PodModeInsecure, PodModeVerified:
							k8s.PodMode = args[0]
						default:
							return nil, fmt.Errorf("wrong value for pods: %s,  must be one of: disabled, verified, insecure", args[0])
						}
						continue
					}
					return nil, c.ArgErr()
				case "namespaces":
					args := c.RemainingArgs()
					if len(args) > 0 {
						k8s.Namespaces = append(k8s.Namespaces, args...)
						continue
					}
					return nil, c.ArgErr()
				case "endpoint":
					args := c.RemainingArgs()
					if len(args) > 0 {
						k8s.APIEndpoint = args[0]
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
				case "resyncperiod":
					args := c.RemainingArgs()
					if len(args) > 0 {
						rp, err := time.ParseDuration(args[0])
						if err != nil {
							return nil, fmt.Errorf("unable to parse resync duration value: '%v': %v", args[0], err)
						}
						k8s.ResyncPeriod = rp
						continue
					}
					return nil, c.ArgErr()
				case "labels":
					args := c.RemainingArgs()
					if len(args) > 0 {
						labelSelectorString := strings.Join(args, " ")
						ls, err := unversionedapi.ParseToLabelSelector(labelSelectorString)
						if err != nil {
							return nil, fmt.Errorf("unable to parse label selector value: '%v': %v", labelSelectorString, err)
						}
						k8s.LabelSelector = ls
						continue
					}
					return nil, c.ArgErr()
				case "fallthrough":
					args := c.RemainingArgs()
					if len(args) == 0 {
						k8s.Fallthrough = true
						continue
					}
					return nil, c.ArgErr()
				case "upstream":
					args := c.RemainingArgs()
					if len(args) == 0 {
						return nil, c.ArgErr()
					}
					ups, err := dnsutil.ParseHostPortOrFile(args...)
					if err != nil {
						return nil, err
					}
					k8s.Proxy = proxy.NewLookup(ups)
				case "federation": // name zone
					args := c.RemainingArgs()
					if len(args) == 2 {
						k8s.Federations = append(k8s.Federations, Federation{
							name: args[0],
							zone: args[1],
						})
						continue
					}
					return nil, fmt.Errorf("incorrect number of arguments for federation, got %v, expected 2", len(args))
				case "autopath": // name zone
					args := c.RemainingArgs()
					k8s.AutoPathNdots = defautNdots
					gotNdots := false
					hostdomains := []string{}
					for _, arg := range args {
						if strings.HasPrefix(arg, ndotsOptPrefix) {
							if gotNdots {
								return nil, fmt.Errorf("found more than one ndots option, expected at most one")
							}
							ndots, err := strconv.Atoi(arg[len(ndotsOptPrefix):])
							if err != nil {
								return nil, fmt.Errorf("invalid ndots option for autopath, got '%v', expected an integer", ndots)
							}
							k8s.AutoPathNdots = ndots
							gotNdots = true
							continue
						}
						hostdomains = append(hostdomains, arg)
					}
					k8s.HostSearchPath = hostdomains
					if len(k8s.HostSearchPath) == 0 {
						path, err := getSearchPathFromResolvConf(resolveConfPath)
						if err != nil {
							fmt.Errorf("could not get host search path: %v", err)
						}
						k8s.HostSearchPath = path
					}
					k8s.AutoPath = true
					middleware.Zones(k8s.HostSearchPath).Normalize()

					continue
				}
			}
			return k8s, nil
		}
	}
	return nil, errors.New("kubernetes setup called without keyword 'kubernetes' in Corefile")
}

func getSearchPathFromResolvConf(filename string) ([]string, error) {
	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, errResolvConfReadErr
	}
	var path []string
	for _, line := range strings.Split(string(content), "\n") {
		if !strings.HasPrefix(line, "search ") && !strings.HasPrefix(line, "domain ") {
			continue
		}
		for _, s := range strings.Split(line[7:], " ") {
			search := strings.TrimSpace(s)
			if search == "" {
				continue
			}
			path = append(path, search)
		}
	}
	return path, nil
}

var resolveConfPath = defaultResolveConfPath

const (
	defaultResyncPeriod    = 5 * time.Minute
	defaultPodMode         = PodModeDisabled
	ndotsOptPrefix         = "ndots:"
	defautNdots            = 1
	defaultResolveConfPath = "/etc/resolv.conf"
)
