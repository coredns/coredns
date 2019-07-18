package azure

import (
	"context"
	"strings"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	pkgFall "github.com/coredns/coredns/plugin/pkg/fall"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/upstream"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/dns/mgmt/dns"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/caddyserver/caddy"
)

var log = clog.NewWithPlugin("azure")

func init() {
	caddy.RegisterPlugin("azure", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func setup(c *caddy.Controller) error {
	env, keys, fall, err := parse(c)
	if err != nil {
		return err
	}
	up := upstream.New()
	ctx := context.Background()
	dnsClient := dns.NewRecordSetsClient(env.Values[auth.SubscriptionID])
	dnsClient.Authorizer, err = env.GetAuthorizer()
	if err != nil {
		return c.Errf("failed to create azure plugin: %v", err)
	}
	h, err := New(ctx, dnsClient, keys, up)
	if err != nil {
		return c.Errf("failed to initialize azure plugin: %v", err)
	}
	h.Fall = fall
	if err := h.Run(ctx); err != nil {
		return c.Errf("failed to run azure plugin: %v", err)
	}
	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		h.Next = next
		return h
	})
	return nil
}

func parse(c *caddy.Controller) (auth.EnvironmentSettings, map[string][]string, pkgFall.F, error) {
	keys := map[string][]string{}
	keyPairs := map[string]struct{}{}
	var err error
	var fall pkgFall.F

	azureEnv := azure.PublicCloud
	env := auth.EnvironmentSettings{Values: map[string]string{}}

	for c.Next() {
		args := c.RemainingArgs()

		for i := 0; i < len(args); i++ {
			parts := strings.SplitN(args[i], ":", 2)
			if len(parts) != 2 {
				return env, keys, fall, c.Errf("invalid resource group / zone '%s'", args[i])
			}
			resourceGroup, zoneName := parts[0], parts[1]
			if resourceGroup == "" || zoneName == "" {
				return env, keys, fall, c.Errf("invalid resource group / zone '%s'", args[i])
			}
			if _, ok := keyPairs[args[i]]; ok {
				return env, keys, fall, c.Errf("conflict zone '%s'", args[i])
			}

			keyPairs[args[i]] = struct{}{}
			keys[resourceGroup] = append(keys[resourceGroup], zoneName)
		}
		for c.NextBlock() {
			switch c.Val() {
			case "subscription_id":
				if c.NextArg() {
					env.Values[auth.SubscriptionID] = c.Val()
				} else {
					return env, keys, fall, c.ArgErr()
				}
			case "tenant_id":
				if c.NextArg() {
					env.Values[auth.TenantID] = c.Val()
				} else {
					return env, keys, fall, c.ArgErr()
				}
			case "client_id":
				if c.NextArg() {
					env.Values[auth.ClientID] = c.Val()
				} else {
					return env, keys, fall, c.ArgErr()
				}
			case "client_secret":
				if c.NextArg() {
					env.Values[auth.ClientSecret] = c.Val()
				} else {
					return env, keys, fall, c.ArgErr()
				}
			case "environment":
				if c.NextArg() {
					env.Values[auth.ClientSecret] = c.Val()
					azureEnv, err = azure.EnvironmentFromName(c.Val())
					if err != nil {
						return env, keys, fall, c.Errf("cannot set azure environment: %s", err.Error())
					}
				} else {
					return env, keys, fall, c.ArgErr()
				}
			case "fallthrough":
				fall.SetZonesFromArgs(c.RemainingArgs())
			default:
				return env, keys, fall, c.Errf("unknown property '%s'", c.Val())
			}
		}
	}
	env.Values[auth.Resource] = azureEnv.ResourceManagerEndpoint
	env.Environment = azureEnv
	return env, keys, fall, nil
}
