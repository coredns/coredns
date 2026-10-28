package azure

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/fall"
	clog "github.com/coredns/coredns/plugin/pkg/log"

	publicAzureDNS "github.com/Azure/azure-sdk-for-go/profiles/latest/dns/mgmt/dns"
	privateAzureDNS "github.com/Azure/azure-sdk-for-go/profiles/latest/privatedns/mgmt/privatedns"
	azurerest "github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/azure/auth"
)

var log = clog.NewWithPlugin("azure")

func init() { plugin.Register("azure", setup) }

func setup(c *caddy.Controller) error {
	env, keys, accessMap, fall, refresh, err := parse(c)
	if err != nil {
		return plugin.Error("azure", err)
	}
	ctx, cancel := context.WithCancel(context.Background())

	publicDNSClient := publicAzureDNS.NewRecordSetsClient(env.Values[auth.SubscriptionID])
	if publicDNSClient.Authorizer, err = env.GetAuthorizer(); err != nil {
		cancel()
		return plugin.Error("azure", err)
	}

	privateDNSClient := privateAzureDNS.NewRecordSetsClient(env.Values[auth.SubscriptionID])
	if privateDNSClient.Authorizer, err = env.GetAuthorizer(); err != nil {
		cancel()
		return plugin.Error("azure", err)
	}

	h, err := New(ctx, publicDNSClient, privateDNSClient, keys, accessMap, refresh)
	if err != nil {
		cancel()
		return plugin.Error("azure", err)
	}
	h.Fall = fall

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		h.Next = next
		return h
	})
	c.OnStartup(func() error { return h.Run(ctx) })
	c.OnShutdown(func() error {
		cancel()
		h.updates.Wait()
		return nil
	})
	return nil
}

func parse(c *caddy.Controller) (auth.EnvironmentSettings, map[string][]string, map[string]string, fall.F, time.Duration, error) {
	resourceGroupMapping := map[string][]string{}
	accessMap := map[string]string{}
	resourceGroupSet := map[string]struct{}{}
	azureEnv := azurerest.PublicCloud
	env := auth.EnvironmentSettings{Values: map[string]string{}}
	refresh := time.Duration(1) * time.Minute // default update frequency to 1 minute

	var fall fall.F
	var access string

	for c.Next() {
		args := c.RemainingArgs()
		var currentZoneKeys []string

		for i := range args {
			parts := strings.SplitN(args[i], ":", 2)
			if len(parts) != 2 {
				return env, resourceGroupMapping, accessMap, fall, refresh, c.Errf("invalid resource group/zone: %q", args[i])
			}
			resourceGroup, zoneName := parts[0], parts[1]
			if resourceGroup == "" || zoneName == "" {
				return env, resourceGroupMapping, accessMap, fall, refresh, c.Errf("invalid resource group/zone: %q", args[i])
			}
			if _, ok := resourceGroupSet[resourceGroup+zoneName]; ok {
				return env, resourceGroupMapping, accessMap, fall, refresh, c.Errf("conflicting zone: %q", args[i])
			}

			resourceGroupSet[resourceGroup+zoneName] = struct{}{}
			accessMap[resourceGroup+zoneName] = "public"
			currentZoneKeys = append(currentZoneKeys, resourceGroup+zoneName)
			resourceGroupMapping[resourceGroup] = append(resourceGroupMapping[resourceGroup], zoneName)
		}

		for c.NextBlock() {
			switch c.Val() {
			case "subscription":
				if !c.NextArg() {
					return env, resourceGroupMapping, accessMap, fall, refresh, c.ArgErr()
				}
				env.Values[auth.SubscriptionID] = c.Val()
			case "tenant":
				if !c.NextArg() {
					return env, resourceGroupMapping, accessMap, fall, refresh, c.ArgErr()
				}
				env.Values[auth.TenantID] = c.Val()
			case "client":
				if !c.NextArg() {
					return env, resourceGroupMapping, accessMap, fall, refresh, c.ArgErr()
				}
				env.Values[auth.ClientID] = c.Val()
			case "secret":
				if !c.NextArg() {
					return env, resourceGroupMapping, accessMap, fall, refresh, c.ArgErr()
				}
				env.Values[auth.ClientSecret] = c.Val()
			case "environment":
				if !c.NextArg() {
					return env, resourceGroupMapping, accessMap, fall, refresh, c.ArgErr()
				}
				var err error
				if azureEnv, err = azurerest.EnvironmentFromName(c.Val()); err != nil {
					return env, resourceGroupMapping, accessMap, fall, refresh, c.Errf("cannot set azure environment: %q", err.Error())
				}
			case "fallthrough":
				fall.SetZonesFromArgs(c.RemainingArgs())
			case "access":
				if !c.NextArg() {
					return env, resourceGroupMapping, accessMap, fall, refresh, c.ArgErr()
				}
				access = c.Val()
				if access != "public" && access != "private" {
					return env, resourceGroupMapping, accessMap, fall, refresh, c.Errf("invalid access value: can be public/private, found: %s", access)
				}
				for _, k := range currentZoneKeys {
					accessMap[k] = access
				}
			case "refresh":
				if !c.NextArg() {
					return env, resourceGroupMapping, accessMap, fall, refresh, c.ArgErr()
				}
				refreshStr := c.Val()
				_, err := strconv.Atoi(refreshStr)
				if err == nil {
					refreshStr += "s"
				}
				refresh, err = time.ParseDuration(refreshStr)
				if err != nil {
					return env, resourceGroupMapping, accessMap, fall, refresh, c.Errf("unable to parse duration: %v", err)
				}
				if refresh <= 0 {
					return env, resourceGroupMapping, accessMap, fall, refresh, c.Errf("refresh interval must be greater than 0: %q", refreshStr)
				}
			default:
				return env, resourceGroupMapping, accessMap, fall, refresh, c.Errf("unknown property: %q", c.Val())
			}
		}
	}

	env.Values[auth.Resource] = azureEnv.ResourceManagerEndpoint
	env.Environment = azureEnv
	return env, resourceGroupMapping, accessMap, fall, refresh, nil
}
