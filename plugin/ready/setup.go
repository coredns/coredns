package ready

import (
	"errors"
	"net"
	"slices"
	"sync"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

func init() { plugin.Register("ready", setup) }

type readinessEndpointsKey struct{}

type readinessBlock struct {
	config  *dnsserver.Config
	monitor monitorType
}

func setup(c *caddy.Controller) error {
	addr, monType, err := parse(c)
	if err != nil {
		return plugin.Error("ready", err)
	}

	endpoints, _ := c.Get(readinessEndpointsKey{}).(map[string]*ready)
	if endpoints == nil {
		endpoints = make(map[string]*ready)
		c.Set(readinessEndpointsKey{}, endpoints)
		registerEventHook()
	}
	rd := endpoints[addr]
	if rd == nil {
		rd = &ready{Addr: addr}
		endpoints[addr] = rd
		c.OnRestart(rd.onFinalShutdown)
		c.OnRestartFailed(rd.onStartup)
		c.OnFinalShutdown(rd.onFinalShutdown)
	}
	rd.blocks = append(rd.blocks, readinessBlock{config: dnsserver.GetConfig(c), monitor: monType})
	return nil
}

var eventHookMu sync.Mutex

func registerEventHook() {
	eventHookMu.Lock()
	defer eventHookMu.Unlock()
	// Caddy clears the hooks before a SIGUSR1 reload.
	if !slices.Contains(caddy.ListPlugins()["event_hooks"], "ready") {
		caddy.RegisterEventHook("ready", startEndpoints)
	}
}

// Start the readiness listeners after the DNS listeners are successfully started.
func startEndpoints(event caddy.EventName, info any) error {
	if event != caddy.InstanceStartupEvent {
		return nil
	}
	instance := info.(*caddy.Instance)
	instance.StorageMu.RLock()
	endpoints, _ := instance.Storage[readinessEndpointsKey{}].(map[string]*ready)
	instance.StorageMu.RUnlock()

	var startErr error
	for _, rd := range endpoints {
		var checks []readinessCheck
		for _, block := range rd.blocks {
			for _, p := range block.config.Handlers() {
				if r, ok := p.(Readiness); ok {
					checks = append(checks, readinessCheck{p.Name(), r, block.monitor})
				}
			}
		}
		rd.checks.replace(checks)
		startErr = errors.Join(startErr, rd.onStartup())
	}
	return startErr
}

// monitorType represents the type of monitoring behavior for the readiness plugin.
type monitorType string

const (
	// monitorTypeUntilReady indicates the monitoring should continue until the system is ready.
	monitorTypeUntilReady monitorType = "until-ready"

	// monitorTypeContinuously indicates the monitoring should continue indefinitely.
	monitorTypeContinuously monitorType = "continuously"
)

func parse(c *caddy.Controller) (string, monitorType, error) {
	addr := ":8181"
	monType := monitorTypeUntilReady

	i := 0
	for c.Next() {
		if i > 0 {
			return "", "", plugin.ErrOnce
		}
		i++
		args := c.RemainingArgs()

		switch len(args) {
		case 0:
		case 1:
			addr = args[0]
			if _, _, e := net.SplitHostPort(addr); e != nil {
				return "", "", e
			}
		default:
			return "", "", c.ArgErr()
		}

		for c.NextBlock() {
			switch c.Val() {
			case "monitor":
				args := c.RemainingArgs()
				if len(args) != 1 {
					return "", "", c.ArgErr()
				}

				var err error
				monType, err = parseMonitorType(c, args[0])
				if err != nil {
					return "", "", err
				}
			default:
				return "", "", c.Errf("unknown property '%s'", c.Val())
			}
		}
	}
	return addr, monType, nil
}

func parseMonitorType(c *caddy.Controller, arg string) (monitorType, error) {
	switch arg {
	case "until-ready":
		return monitorTypeUntilReady, nil
	case "continuously":
		return monitorTypeContinuously, nil
	default:
		return "", c.Errf("monitor type '%s' not supported", arg)
	}
}
