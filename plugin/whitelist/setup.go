package whitelist

import (
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/kubernetes"
	"github.com/mholt/caddy"
)

func init() {
	caddy.RegisterPlugin("whitelist", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func kubernetesParse(c *caddy.Controller) (*kubernetes.Kubernetes, error) {
	var (
		k8s *kubernetes.Kubernetes
		err error
	)

	i := 0
	for c.Next() {
		if i > 0 {
			return nil, plugin.ErrOnce
		}
		i++

		k8s, err = kubernetes.ParseStanza(c)
		if err != nil {
			return k8s, err
		}
	}
	return k8s, nil
}

func setup(c *caddy.Controller) error {

	k8s, err := kubernetesParse(c)
	if err != nil {
		return plugin.Error("whitelist", err)
	}

	err = k8s.InitKubeCache()

	k8s.RegisterKubeCache(c)

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		return &Whitelist{Kubernetes: k8s, Next: next, ServicesToHosts: map[string]string{"sleep": "www.google.com."}}
	})

	return nil
}
