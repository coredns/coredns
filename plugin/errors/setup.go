package errors

import (
	"regexp"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

func init() { plugin.Register("errors", setup) }

func setup(c *caddy.Controller) error {
	handler, err := errorsParse(c)
	if err != nil {
		return plugin.Error("errors", err)
	}

	c.OnShutdown(func() error {
		handler.stop()
		return nil
	})

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		handler.Next = next
		return handler
	})

	return nil
}

func errorsParse(c *caddy.Controller) (*errorHandler, error) {
	handler := newErrorHandler()

	i := 0
	for c.Next() {
		if i > 0 {
			return nil, plugin.ErrOnce
		}
		i++

		args := c.RemainingArgs()
		switch len(args) {
		case 0:
		case 1:
			if args[0] != "stdout" {
				return nil, c.Errf("invalid log file: %s", args[0])
			}
		default:
			return nil, c.ArgErr()
		}

		for c.NextBlock() {
			switch c.Val() {
			case "stacktrace":
				dnsserver.GetConfig(c).Stacktrace = true
			case "consolidate":
				pattern, err := parseConsolidate(c)
				if err != nil {
					return nil, err
				}
				handler.patterns = append(handler.patterns, pattern)
			default:
				return handler, c.SyntaxErr("Unknown field " + c.Val())
			}
		}
	}
	return handler, nil
}

func parseConsolidate(c *caddy.Controller) (*pattern, error) {
	args := c.RemainingArgs()
	if len(args) < 2 || len(args) > 3 {
		return nil, c.ArgErr()
	}
	p, err := time.ParseDuration(args[0])
	if err != nil {
		return nil, c.Err(err.Error())
	}
	re, err := regexp.Compile(args[1])
	if err != nil {
		return nil, c.Err(err.Error())
	}
	lc, err := parseLogLevel(c, args)
	if err != nil {
		return nil, err
	}
	return &pattern{period: p, pattern: re, logCallback: lc}, nil
}

func parseLogLevel(c *caddy.Controller, args []string) (func(format string, v ...interface{}), error) {
	if len(args) != 3 {
		return log.Errorf, nil
	}

	switch args[2] {
	case "warning":
		return log.Warningf, nil
	case "error":
		return log.Errorf, nil
	case "info":
		return log.Infof, nil
	case "debug":
		return log.Debugf, nil
	default:
		return nil, c.Errf("unknown log level argument in consolidate: %s", args[2])
	}
}
