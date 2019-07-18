package azure

import (
	"context"
	"strings"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/fall"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/upstream"

	azure "github.com/Azure/azure-sdk-for-go/profiles/latest/dns/mgmt/dns"
	azureCloud "github.com/Azure/go-autorest/autorest/azure"
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
	ctx := context.Background()
	dnsClient := azure.NewRecordSetsClient(env.Values[auth.SubscriptionID])
	dnsClient.Authorizer, err = env.GetAuthorizer()
	if err != nil {
		return c.Errf("failed to create azure plugin: %v", err)
	}
	h, err := New(ctx, dnsClient, keys, upstream.New())
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

func parse(c *caddy.Controller) (auth.EnvironmentSettings, map[string][]string, fall.F, error) {
	resourceGroupMapping := map[string][]string{}
	resourceGroupSet := map[string]struct{}{}
	var err error
	var fall fall.F

	azureEnv := azureCloud.PublicCloud
	env := auth.EnvironmentSettings{Values: map[string]string{}}

	for c.Next() {
		args := c.RemainingArgs()

		for i := 0; i < len(args); i++ {
			parts := strings.SplitN(args[i], ":", 2)
			if len(parts) != 2 {
				return env, resourceGroupMapping, fall, c.Errf("invalid resource group / zone '%s'", args[i])
			}
			resourceGroup, zoneName := parts[0], parts[1]
			if resourceGroup == "" || zoneName == "" {
				return env, resourceGroupMapping, fall, c.Errf("invalid resource group / zone '%s'", args[i])
			}
			if _, ok := resourceGroupSet[args[i]]; ok {
				return env, resourceGroupMapping, fall, c.Errf("conflict zone '%s'", args[i])
			}

			resourceGroupSet[args[i]] = struct{}{}
			resourceGroupMapping[resourceGroup] = append(resourceGroupMapping[resourceGroup], zoneName)
		}
		for c.NextBlock() {
			switch c.Val() {
			case "subscription_id":
				if c.NextArg() {
					env.Values[auth.SubscriptionID] = c.Val()
				} else {
					return env, resourceGroupMapping, fall, c.ArgErr()
				}
			case "tenant_id":
				if c.NextArg() {
					env.Values[auth.TenantID] = c.Val()
				} else {
					return env, resourceGroupMapping, fall, c.ArgErr()
				}
			case "client_id":
				if c.NextArg() {
					env.Values[auth.ClientID] = c.Val()
				} else {
					return env, resourceGroupMapping, fall, c.ArgErr()
				}
			case "client_secret":
				if c.NextArg() {
					env.Values[auth.ClientSecret] = c.Val()
				} else {
					return env, resourceGroupMapping, fall, c.ArgErr()
				}
			case "environment":
				if c.NextArg() {
					env.Values[auth.ClientSecret] = c.Val()
					azureEnv, err = azureCloud.EnvironmentFromName(c.Val())
					if err != nil {
						return env, resourceGroupMapping, fall, c.Errf("cannot set azure environment: %s", err.Error())
					}
				} else {
					return env, resourceGroupMapping, fall, c.ArgErr()
				}
			case "fallthrough":
				fall.SetZonesFromArgs(c.RemainingArgs())
			default:
				return env, resourceGroupMapping, fall, c.Errf("unknown property '%s'", c.Val())
			}
		}
	}
	env.Values[auth.Resource] = azureEnv.ResourceManagerEndpoint
	env.Environment = azureEnv
	return env, resourceGroupMapping, fall, nil
}
