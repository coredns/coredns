package azure

import (
	"context"
	"os"
	"strings"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/fall"
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
	envSettings, keys, fall, err := parseCorefile(c)
	if err != nil {
		return err
	}
	up := upstream.New()
	ctx := context.Background()
	dnsClient := dns.NewRecordSetsClient(envSettings.Values[auth.SubscriptionID])
	dnsClient.Authorizer, err = envSettings.GetAuthorizer()
	if err != nil {
		return c.Errf("failed to create azure plugin: %v", err)
	}
	h, err := New(ctx, dnsClient, keys, up)
	h.Fall = fall
	if err := h.Run(ctx); err != nil {
		return c.Errf("failed to initialize azure plugin: %v", err)
	}
	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		h.Next = next
		return h
	})
	return nil
}

func parseCorefile(c *caddy.Controller) (auth.EnvironmentSettings, map[string][]string, fall.F, error) {
	keyPairs := map[string]struct{}{}
	keys := map[string][]string{}
	var err error
	var fall fall.F
	envSettings := auth.EnvironmentSettings{Values: map[string]string{}}
	// envSettings.Values[auth.Resource] = "866a1291-8274-4540-9779-b5d4c5d53f5a"

	for c.Next() {
		args := c.RemainingArgs()

		for i := 0; i < len(args); i++ {
			parts := strings.SplitN(args[i], ":", 2)
			if len(parts) != 2 {
				return envSettings, keys, fall, c.Errf("invalid resource group / zone '%s'", args[i])
			}
			resourceGroup, zoneName := parts[0], parts[1]
			if resourceGroup == "" || zoneName == "" {
				return envSettings, keys, fall, c.Errf("invalid resource group / zone '%s'", args[i])
			}
			if _, ok := keyPairs[args[i]]; ok {
				return envSettings, keys, fall, c.Errf("conflict zone '%s'", args[i])
			}

			keyPairs[args[i]] = struct{}{}
			keys[resourceGroup] = append(keys[resourceGroup], zoneName)
		}
		for c.NextBlock() {
			switch c.Val() {
			case "auth_location":
				var path string
				if c.NextArg() {
					path = c.Val()
				} else {
					return envSettings, keys, fall, c.ArgErr()
				}
				os.Setenv("AZURE_AUTH_LOCATION", path)
				fileSettings, err := auth.GetSettingsFromFile()
				os.Unsetenv("AZURE_AUTH_LOCATION")
				if err != nil {
					return envSettings, keys, fall, c.Errf("cannot use azure auth location: %s", err.Error())
				}
				envSettings.Values = fileSettings.Values
			case "subscription_id":
				if c.NextArg() {
					envSettings.Values[auth.SubscriptionID] = c.Val()
				} else {
					return envSettings, keys, fall, c.ArgErr()
				}
			case "tenant_id":
				if c.NextArg() {
					envSettings.Values[auth.TenantID] = c.Val()
				} else {
					return envSettings, keys, fall, c.ArgErr()
				}
			case "client_id":
				if c.NextArg() {
					envSettings.Values[auth.ClientID] = c.Val()
				} else {
					return envSettings, keys, fall, c.ArgErr()
				}
			case "client_secret":
				if c.NextArg() {
					envSettings.Values[auth.ClientSecret] = c.Val()
				} else {
					return envSettings, keys, fall, c.ArgErr()
				}
			case "environment":
				if c.NextArg() {
					envSettings.Values[auth.ClientSecret] = c.Val()
					envSettings.Environment, err = azure.EnvironmentFromName(c.Val())
					if err != nil {
						return envSettings, keys, fall, c.Errf("cannot set azure environment: %s", err.Error())
					}
				} else {
					return envSettings, keys, fall, c.ArgErr()
				}
			case "fallthrough":
				fall.SetZonesFromArgs(c.RemainingArgs())
			default:
				return envSettings, keys, fall, c.Errf("unknown property '%s'", c.Val())
			}
		}
	}
	envSettings.Values[auth.Resource] = azure.PublicCloud.ResourceManagerEndpoint
	envSettings.Environment = azure.PublicCloud
	return envSettings, keys, fall, nil
}
