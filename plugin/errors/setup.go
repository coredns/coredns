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
	if len(args) < 2 || len(args) > 4 {
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

	lc, showFirst, err := parseOptionalParams(c, args[2:])
	if err != nil {
		return nil, err
	}

	return &pattern{period: p, pattern: re, logCallback: lc, showFirst: showFirst}, nil
}

// parseOptionalParams parses optional parameters (log level and show_first flag).
// Order: log level (optional) must come before show_first (optional).
func parseOptionalParams(c *caddy.Controller, args []string) (func(format string, v ...any), bool, error) {
	logLevels := map[string]func(format string, v ...any){
		"warning": log.Warningf,
		"error":   log.Errorf,
		"info":    log.Infof,
		"debug":   log.Debugf,
	}

	var logCallback func(format string, v ...any) // nil means not set yet
	showFirst := false

	for _, arg := range args {
		if callback, isLogLevel := logLevels[arg]; isLogLevel {
			if logCallback != nil {
				return nil, false, c.Errf("multiple log levels specified in consolidate")
			}
			if showFirst {
				return nil, false, c.Errf("log level must come before show_first in consolidate")
			}
			logCallback = callback
		} else if arg == "show_first" {
			showFirst = true
		} else {
			return nil, false, c.Errf("unknown option in consolidate: %s", arg)
		}
	}

	// Use default log level if not specified
	if logCallback == nil {
		logCallback = log.Errorf
	}

	return logCallback, showFirst, nil
}
