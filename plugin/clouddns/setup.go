package clouddns

import (
	"context"
	"encoding/json"
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
	"github.com/coredns/coredns/plugin/pkg/upstream"

	gcp "google.golang.org/api/dns/v1"
	"google.golang.org/api/option"
)

var log = clog.NewWithPlugin("clouddns")

func init() { plugin.Register("clouddns", setup) }

// exposed for testing
var f = func(ctx context.Context, opts ...option.ClientOption) (gcpDNS, error) {
	// if credentials file is not provided in the Corefile, authenticate the
	// client using env variables; option.WithEndpoint, if any, still applies.
	client, err := gcp.NewService(ctx, opts...)
	return gcpClient{client}, err
}

func setup(c *caddy.Controller) error {
	for c.Next() {
		keyPairs := map[string]struct{}{}
		keys := map[string][]string{}

		var fall fall.F
		up := upstream.New()

		args := c.RemainingArgs()

		for i := range args {
			parts := strings.SplitN(args[i], ":", 3)
			if len(parts) != 3 {
				return plugin.Error("clouddns", c.Errf("invalid zone %q", args[i]))
			}
			dnsName, projectName, hostedZone := parts[0], parts[1], parts[2]
			if dnsName == "" || projectName == "" || hostedZone == "" {
				return plugin.Error("clouddns", c.Errf("invalid zone %q", args[i]))
			}
			if _, ok := keyPairs[args[i]]; ok {
				return plugin.Error("clouddns", c.Errf("conflict zone %q", args[i]))
			}

			keyPairs[args[i]] = struct{}{}
			keys[dnsName] = append(keys[dnsName], projectName+":"+hostedZone)
		}

		var opts []option.ClientOption
		refresh := time.Duration(1) * time.Minute // default update frequency to 1 minute
		for c.NextBlock() {
			switch c.Val() {
			case "upstream":
				c.RemainingArgs()
			case "credentials":
				if !c.NextArg() {
					return plugin.Error("clouddns", c.ArgErr())
				}
				credType, err := getCredType(c.Val())
				if err != nil {
					return plugin.Error("clouddns", c.Errf("invalid credentials file %q: %v", c.Val(), err))
				}
				opts = append(opts, option.WithAuthCredentialsFile(credType, c.Val()))
			case "endpoint":
				if !c.NextArg() {
					return plugin.Error("clouddns", c.ArgErr())
				}
				opts = append(opts, option.WithEndpoint(c.Val()))
			case "fallthrough":
				fall.SetZonesFromArgs(c.RemainingArgs())
			case "refresh":
				if !c.NextArg() {
					return plugin.Error("clouddns", c.ArgErr())
				}
				refreshStr := c.Val()
				_, err := strconv.Atoi(refreshStr)
				if err == nil {
					refreshStr += "s"
				}
				refresh, err = time.ParseDuration(refreshStr)
				if err != nil {
					return plugin.Error("clouddns", c.Errf("unable to parse duration: %v", err))
				}
				if refresh <= 0 {
					return plugin.Error("clouddns", c.Errf("refresh interval must be greater than 0: %q", refreshStr))
				}
			default:
				return plugin.Error("clouddns", c.Errf("unknown property %q", c.Val()))
			}
		}

		ctx, cancel := context.WithCancel(context.Background())
		client, err := f(ctx, opts...)
		if err != nil {
			cancel()
			return err
		}

		h, err := New(ctx, client, keys, up, refresh)
		if err != nil {
			cancel()
			return plugin.Error("clouddns", c.Errf("failed to create plugin: %v", err))
		}
		h.Fall = fall

		if err := h.Run(ctx); err != nil {
			cancel()
			return plugin.Error("clouddns", c.Errf("failed to initialize plugin: %v", err))
		}

		dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
			h.Next = next
			return h
		})
		c.OnShutdown(func() error { cancel(); return nil })
	}

	return nil
}

func getCredType(filename string) (option.CredentialsType, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return "", err
	}
	var f struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return "", err
	}
	if f.Type == "" {
		return "", fmt.Errorf("missing `type` field in credential")
	}

	// Check against allowed types
	ct := option.CredentialsType(f.Type)
	switch ct {
	case option.ServiceAccount,
		option.AuthorizedUser,
		option.ImpersonatedServiceAccount,
		option.ExternalAccount:
		return ct, nil
	}
	return "", fmt.Errorf("unknown credential type: %s", f.Type)
}
