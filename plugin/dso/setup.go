package dso

import (
	"fmt"
	"slices"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/parse"
)

const Name = "dso"

var log = clog.NewWithPlugin(Name)

func init() { plugin.Register(Name, setup) }

func setup(c *caddy.Controller) error {
	// DSO is paired to single DNS server and cannot be shared.
	listenPorts := make([]string, 0, len(c.ServerBlockKeys))
	for _, k := range c.ServerBlockKeys {
		_, k = parse.Transport(k)
		_, port, _ := plugin.SplitHostPort(k)
		listenPorts = append(listenPorts, port)
	}
	i := slices.IndexFunc(listenPorts, func(port string) bool { return port != listenPorts[0] })
	if i >= 0 {
		return plugin.Error(Name, errShared)
	}

	dsoCfg, err := parseConfig(c)
	if err != nil {
		return plugin.Error(Name, err)
	}
	dnsCfg := dnsserver.GetConfig(c)
	instHandler := globalDSO.getInstanceDSO(c)
	instHandler.addConfigPair(dnsCfg, dsoCfg)

	// While DSO does not synthesise responses, siteHandler
	// must be installed to pair DSO and DNS servers.
	siteHandler := instHandler.newSiteDSO()
	dnsCfg.AddPlugin(func(h plugin.Handler) plugin.Handler {
		siteHandler.Next = h
		return siteHandler
	})

	return nil
}

var (
	errBare      = fmt.Errorf("missing definition")
	errShared    = fmt.Errorf("cannot share definition")
	errRedefined = fmt.Errorf("cannot redefine")
	errAddress   = fmt.Errorf("bad address")
	errTLSConfig = fmt.Errorf("missing TLS config")
)
