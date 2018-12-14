// Package coresetup contains various convience function to parse Corefile snippets.
package coresetup

import (
	"fmt"

	"github.com/mholt/caddy"
)

var errNoDefault = fmt.Errorf("default value needed, but none given")

// Parse parses the Corefile part. For each Type will be return a Value which either has the value from the
// config or the default (if no argument was found).
func Parse(c *caddy.Controller, x ...Type) ([]Value, error) {
	args := c.RemainingArgs()
	values := make([]Value, len(x))

	if len(args) > len(x) {
		return nil, c.ArgErr()
	}

	var err error

	for i, p := range x {
		val := ""
		if i < len(args) {
			val = args[i]
		}

		switch t := p.(type) {
		case Int:
			if val == "" {
				values[i] = v{Int: &t.Default}
				continue
			}
			values[i], err = p.Parse(val)
			if err != nil {
				return values, err
			}

		case Duration:
			if val == "" {
				values[i] = v{Duration: &t.Default}
				continue
			}
			values[i], err = p.Parse(val)
			if err != nil {
				return values, err
			}

		case String:
			if val == "" {
				values[i] = v{String: &t.Default}
				continue
			}
			values[i], err = p.Parse(val)
			if err != nil {
				return values, err
			}
		}
	}

	return values, nil
}
