package timeouts

import (
	"strconv"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/durations"
)

func init() { plugin.Register("timeouts", setup) }

func setup(c *caddy.Controller) error {
	err := parseTimeouts(c)
	if err != nil {
		return plugin.Error("timeouts", err)
	}
	return nil
}

func parseTimeouts(c *caddy.Controller) error {
	config := dnsserver.GetConfig(c)

	for c.Next() {
		args := c.RemainingArgs()
		if len(args) > 0 {
			return plugin.Error("timeouts", c.ArgErr())
		}

		b := 0
		for c.NextBlock() {
			switch block := c.Val(); block {
			case "read":
				timeout, err := parseTimeout(c)
				if err != nil {
					return err
				}
				config.ReadTimeout = timeout

			case "write":
				timeout, err := parseTimeout(c)
				if err != nil {
					return err
				}
				config.WriteTimeout = timeout

			case "idle":
				timeout, err := parseTimeout(c)
				if err != nil {
					return err
				}
				config.IdleTimeout = timeout

			case "max_queries":
				args := c.RemainingArgs()
				if len(args) != 1 {
					return c.ArgErr()
				}

				maxQueries, err := strconv.Atoi(args[0])
				if err != nil {
					return c.Err(err.Error())
				}

				if maxQueries < 0 {
					return c.Err("max_queries must be a positive integer")
				}
				config.MaxConnectionQueries = maxQueries

			default:
				return c.Errf("unknown option: '%s'", block)
			}
			b++
		}

		if b == 0 {
			return plugin.Error("timeouts", c.Err("timeouts block with no timeouts or limits specified"))
		}
	}
	return nil
}

func parseTimeout(c *caddy.Controller) (time.Duration, error) {
	timeoutArgs := c.RemainingArgs()
	if len(timeoutArgs) != 1 {
		return 0, c.ArgErr()
	}

	timeout, err := durations.NewDurationFromArg(timeoutArgs[0])
	if err != nil {
		return 0, c.Err(err.Error())
	}

	if timeout < (1*time.Second) || timeout > (24*time.Hour) {
		return 0, c.Errf("timeout provided '%s' needs to be between 1 second and 24 hours", timeout)
	}

	return timeout, nil
}
