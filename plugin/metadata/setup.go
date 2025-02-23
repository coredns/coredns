package metadata

import (
	"text/template"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"

	"github.com/miekg/dns"
)

func init() { plugin.Register(pluginName, setup) }

func setup(c *caddy.Controller) error {
	m, err := metadataParse(c)
	if err != nil {
		return err
	}
	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		m.Next = next
		return m
	})

	c.OnStartup(func() error {
		plugins := dnsserver.GetConfig(c).Handlers()
		for _, p := range plugins {
			if met, ok := p.(Provider); ok {
				m.Providers = append(m.Providers, met)
			}
		}
		return nil
	})

	return nil
}

func metadataParse(c *caddy.Controller) (*Metadata, error) {
	m := &Metadata{}
	c.Next()

	m.Zones = plugin.OriginsFromArgsOrServerBlock(c.RemainingArgs(), c.ServerBlockKeys)

	// parse mapping
	if c.NextBlock() {
		mapping, err := parseMap(c)
		if err != nil {
			return nil, plugin.Error(pluginName, err)
		}
		m.Map = mapping
	}

	// no more blocks or args expected
	if c.NextBlock() || c.Next() {
		return nil, plugin.Error(pluginName, c.ArgErr())
	}
	return m, nil
}

func parseMap(c *caddy.Controller) (*Map, error) {
	// map
	if c.Val() != "map" {
		return nil, c.Errf("unexpected token '%s'; expect 'map'", c.Val())
	}

	// source
	if !c.NextArg() {
		return nil, c.Err("missing source")
	}
	source := c.Val()
	tmpl, err := template.New(source).Parse(source)
	if err != nil {
		return nil, c.Err(err.Error())
	}

	m := &Map{
		Mapping: make(map[string][]KeyValue),
		Source:  tmpl,
	}

	// labels
	labels := c.RemainingArgs()
	if len(labels) == 0 {
		return nil, c.Err("missing label(s)")
	}
	c.Next() // read "{"

	for c.NextBlock() {
		// input
		sourceVal := c.Val()

		// values
		values := c.RemainingArgs()

		// special vase to handle default RCODE
		if sourceVal == defaultKey && len(values) == 1 {
			if rcode, ok := dns.StringToRcode[values[0]]; ok {
				m.RcodeOnMiss = rcode
				continue
			}
		}

		if len(values) != len(labels) {
			return nil, c.Err("number of values must match labels")
		}

		for i := 0; i < len(values); i++ {
			m.Mapping[sourceVal] = append(m.Mapping[sourceVal], KeyValue{Key: labels[i], Value: values[i]})
		}
	}

	// no values and default are provided
	if len(m.Mapping) == 0 && m.RcodeOnMiss == 0 {
		return nil, c.Err("missing value(s)")
	}

	c.Next() // read "}"

	return m, nil
}
