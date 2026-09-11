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

type instanceDSOKey struct{}

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
	instHandler, ok := c.Get(instanceDSOKey{}).(*instanceDSO)
	if !ok {
		instHandler = &instanceDSO{}
		c.Set(instanceDSOKey{}, instHandler)
		c.OnStartup(instHandler.onStartup)
		c.OnRestart(instHandler.onRestart)
		c.OnRestartFailed(instHandler.onRestartFailed)
		c.OnFinalShutdown(instHandler.onFinalShutdown)
	}
	if dsoCfg != handlerOnlyConfig {
		instHandler.addConfig(dnsCfg, dsoCfg)
	}

	// While DSO does not synthesise responses, handler
	// must be installed to pair DSO and DNS servers.
	handler := &dso{instanceDSO: instHandler}
	dnsCfg.AddPlugin(func(h plugin.Handler) plugin.Handler {
		handler.Next = h
		return handler
	})

	return nil
}

var (
	errShared    = fmt.Errorf("DSO cannot be shared by DNS servers")
	errRedefined = fmt.Errorf("DSO must be defined once per DNS server")
)
