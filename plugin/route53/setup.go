package route53

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/fall"
	clog "github.com/coredns/coredns/plugin/pkg/log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/route53/route53iface"
)

var log = clog.NewWithPlugin("route53")

func init() { plugin.Register("route53", setup) }

// exposed for testing
var f = func(endpoint *string) route53iface.Route53API {
	cfg := aws.Config{}
	cfg.Endpoint = endpoint
	return route53.New(session.Must(session.NewSession(&cfg)))
}

func setup(c *caddy.Controller) error {
	for c.Next() {
		keyPairs := map[string]struct{}{}
		keys := map[string][]string{}

		// Route53 plugin attempts to find AWS credentials by using ChainCredentials.
		// And the order of that provider chain is as follows:
		// Static AWS keys -> Environment Variables -> Credentials file -> IAM role
		// With that said, even though a user doesn't define any credentials in
		// Corefile, we should still attempt to read the default credentials file,
		// ~/.aws/credentials with the default profile.
		var fall fall.F
		var endpoint string

		refresh := time.Duration(1) * time.Minute // default update frequency to 1 minute

		args := c.RemainingArgs()

		for i := 0; i < len(args); i++ {
			parts := strings.SplitN(args[i], ":", 2)
			if len(parts) != 2 {
				return plugin.Error("route53", c.Errf("invalid zone %q", args[i]))
			}
			dns, hostedZoneID := parts[0], parts[1]
			if dns == "" || hostedZoneID == "" {
				return plugin.Error("route53", c.Errf("invalid zone %q", args[i]))
			}
			if _, ok := keyPairs[args[i]]; ok {
				return plugin.Error("route53", c.Errf("conflict zone %q", args[i]))
			}

			keyPairs[args[i]] = struct{}{}
			keys[dns] = append(keys[dns], hostedZoneID)
		}

		for c.NextBlock() {
			switch c.Val() {
			case "aws_access_key":
				v := c.RemainingArgs()
				if len(v) < 2 {
					return plugin.Error("route53", c.Errf("invalid access key: '%v'", v))
				}
				os.Setenv("AWS_ACCESS_KEY_ID", v[0])
				os.Setenv("AWS_SECRET_ACCESS_KEY", v[1])
				log.Warningf("Save aws_access_key in Corefile has been deprecated, please use other authentication methods instead")
			case "aws_endpoint":
				if c.NextArg() {
					endpoint = c.Val()
				} else {
					return plugin.Error("route53", c.ArgErr())
				}
			case "upstream":
				c.RemainingArgs() // eats args
			case "credentials":
				if c.NextArg() {
					os.Setenv("AWS_PROFILE", c.Val())
				} else {
					return c.ArgErr()
				}
				if c.NextArg() {
					os.Setenv("AWS_SHARED_CREDENTIAL_FILE", c.Val())
				}
			case "fallthrough":
				fall.SetZonesFromArgs(c.RemainingArgs())
			case "refresh":
				if c.NextArg() {
					refreshStr := c.Val()
					_, err := strconv.Atoi(refreshStr)
					if err == nil {
						refreshStr = fmt.Sprintf("%ss", c.Val())
					}
					refresh, err = time.ParseDuration(refreshStr)
					if err != nil {
						return plugin.Error("route53", c.Errf("Unable to parse duration: %v", err))
					}
					if refresh <= 0 {
						return plugin.Error("route53", c.Errf("refresh interval must be greater than 0: %q", refreshStr))
					}
				} else {
					return plugin.Error("route53", c.ArgErr())
				}
			default:
				return plugin.Error("route53", c.Errf("unknown property %q", c.Val()))
			}
		}

		ctx, cancel := context.WithCancel(context.Background())
		h, err := New(ctx, f(&endpoint), keys, refresh)
		if err != nil {
			return plugin.Error("route53", c.Errf("failed to create route53 plugin: %v", err))
		}
		h.Fall = fall
		if err := h.Run(ctx); err != nil {
			return plugin.Error("route53", c.Errf("failed to initialize route53 plugin: %v", err))
		}
		dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
			h.Next = next
			return h
		})
		c.OnShutdown(func() error { cancel(); return nil })
	}
	return nil
}
