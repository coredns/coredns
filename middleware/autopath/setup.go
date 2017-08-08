package autopath

import (
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/middleware"
	"github.com/coredns/coredns/middleware/chaos"

	"github.com/mholt/caddy"
)

func init() {
	caddy.RegisterPlugin("autopath", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})

}

func setup(c *caddy.Controller) error {
	ap, mw, err := autoPathParse(c)
	if err != nil {
		return middleware.Error("autopath", err)
	}

	c.OnStartup(func() error {
		// So we know for sure the mw is initialized.
		m := dnsserver.GetMiddleware(c, mw)
		switch mw {
		case "kubernetes":
			//if k, ok := m.(kubernetes.Kubernetes); ok {
			//&ap.searchFunc = k.AutoPath
			//}
		case "chaos":
			if c, ok := m.(chaos.Chaos); ok {
				ap.searchFunc = c.AutoPath
			}
		}
		return nil
	})

	dnsserver.GetConfig(c).AddMiddleware(func(next middleware.Handler) middleware.Handler {
		ap.Next = next
		return ap
	})

	return nil
}

func autoPathParse(c *caddy.Controller) (*AutoPath, string, error) {
	return &AutoPath{search: []string{"default.svc.cluster.local.", "svc.cluster.local.", "cluster.local.", "com.", ""}}, "chaos", nil
}

/*
	case "autopath": // name zone
		args := c.RemainingArgs()
		k8s.autoPath = &autopath.AutoPath{
			NDots:          defautNdots,
			HostSearchPath: []string{},
			ResolvConfFile: defaultResolvConfFile,
			OnNXDOMAIN:     defaultOnNXDOMAIN,
		}
		if len(args) > 3 {
			return nil, fmt.Errorf("incorrect number of arguments for autopath, got %v, expected at most 3", len(args))

		}
		if len(args) > 0 {
			ndots, err := strconv.Atoi(args[0])
			if err != nil {
				return nil, fmt.Errorf("invalid NDOTS argument for autopath, got '%v', expected an integer", ndots)
			}
			k8s.autoPath.NDots = ndots
		}
		if len(args) > 1 {
			switch args[1] {
			case dns.RcodeToString[dns.RcodeNameError]:
				k8s.autoPath.OnNXDOMAIN = dns.RcodeNameError
			case dns.RcodeToString[dns.RcodeSuccess]:
				k8s.autoPath.OnNXDOMAIN = dns.RcodeSuccess
			case dns.RcodeToString[dns.RcodeServerFailure]:
				k8s.autoPath.OnNXDOMAIN = dns.RcodeServerFailure
			default:
				return nil, fmt.Errorf("invalid RESPONSE argument for autopath, got '%v', expected SERVFAIL, NOERROR, or NXDOMAIN", args[1])
			}
		}
		if len(args) > 2 {
			k8s.autoPath.ResolvConfFile = args[2]
		}
		rc, err := dns.ClientConfigFromFile(k8s.autoPath.ResolvConfFile)
		if err != nil {
			return nil, fmt.Errorf("error when parsing %v: %v", k8s.autoPath.ResolvConfFile, err)
		}
		k8s.autoPath.HostSearchPath = rc.Search
		middleware.Zones(k8s.autoPath.HostSearchPath).Normalize()
		continue
*/
