package gcpdns

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/caddyserver/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/gcpdns/api"
	"github.com/coredns/coredns/plugin/gcpdns/clouddns"
	"github.com/coredns/coredns/plugin/pkg/fall"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/upstream"
	"google.golang.org/api/option"
)

var log = clog.NewWithPlugin("gcpdns")

const dfltNm = "gcpdns"

func init() {
	caddy.RegisterPlugin("gcpdns", caddy.Plugin{
		ServerType: "dns",
		Action: func(c *caddy.Controller) error {
			f := func(ctx context.Context, opts ...option.ClientOption) (*api.Service, error) {
				return clouddns.NewService(ctx, opts...)
			}
			return setup(c, f)
		},
	})
}

// Factory that facilitates mocking of dns.Service.
type dnsServiceFactory func(ctx context.Context, opts ...option.ClientOption) (*api.Service, error)

func setup(c *caddy.Controller, factory dnsServiceFactory) error {

	var dfltCnt = 0

	for c.Next() {
		visited := map[string]struct{}{}
		keys := make(map[string][]zoneID)

		var f fall.F
		var options = make([]option.ClientOption, 0, 1)
		var name = dfltNm

		up := upstream.New()
		args := c.RemainingArgs()

		for i := 0; i < len(args); i++ {
			parts := strings.SplitN(args[i], ":", 2)
			if len(parts) != 2 {
				return c.Errf("invalid zone '%s'", args[i])
			}
			domain, zone := parts[0], parts[1]
			if domain == "" || zone == "" {
				return c.Errf("invalid zone '%s'", args[i])
			}
			zoneParts := strings.SplitN(zone, "/", 2)
			if len(zoneParts) != 2 {
				return c.Errf("invalid zone '%s'", args[i])
			}
			project, id := zoneParts[0], zoneParts[1]
			if project == "" || id == "" {
				return c.Errf("invalid zone '%s'", args[i])
			}

			if _, ok := visited[args[i]]; ok {
				return c.Errf("conflict zone '%s'", args[i])
			}

			visited[args[i]] = struct{}{}
			keys[domain] = append(keys[domain], zoneID{project: project, name: id})
		}

		for c.NextBlock() {
			switch c.Val() {
			case "name":
				v := c.RemainingArgs()
				if len(v) != 1 {
					return c.Err("missing plugin name")
				}
				name = v[0]

			case "gcp_service_account_json":
				if len(options) > 0 {
					return c.Err("credentials already set")
				}
				v := c.RemainingArgs()
				if len(v) < 1 {
					return c.Err("missing GCP service account environmental variable name")
				}
				e := v[0]
				b64, ok := os.LookupEnv(e)
				if !ok {
					return c.Errf("missing environmental variable %s", e)
				}
				j, err := base64.StdEncoding.DecodeString(b64)
				if err != nil {
					return c.Errf("invalid base64 %v", err)
				}
				options = append(options, option.WithCredentialsJSON([]byte(j)))

			case "gcp_service_account_file":
				if len(options) > 0 {
					return c.Err("credentials already set")
				}
				v := c.RemainingArgs()
				if len(v) < 1 {
					return c.Err("missing GCP service account file name")
				}
				f := v[0]
				if _, err := os.Stat(f); os.IsNotExist(err) {
					return fmt.Errorf("service account file does not exist: %s", f)
				}
				options = append(options, option.WithCredentialsFile(f))

			case "upstream":
				c.RemainingArgs() // eats args

			case "fallthrough":
				f.SetZonesFromArgs(c.RemainingArgs())

			default:
				return c.Errf("unknown property '%s'", c.Val())
			}
		}

		if name == dfltNm {
			dfltCnt++
			name = fmt.Sprintf("%s-%d", dfltNm, dfltCnt)
		}

		ctx := context.Background()
		client, err := factory(ctx, options...)
		if err != nil {
			return c.Errf("failed to create gcpdns plugin: %v", err)
		}

		h, err := newGcpDNSHandler(ctx, name, client, keys, up)
		if err != nil {
			return c.Errf("failed to initialize gcpdns plugin: %v", err)
		}
		h.Fall = f

		c.OnStartup(func() error {
			log.Debugf("[%s] GCP Cloud DNS plugin starting up", h.name)
			ctx, cancel := context.WithCancel(context.Background())
			if err := h.startup(ctx); err != nil {
				return c.Errf("failed to initialize gcpdns plugin: %v", err)
			}
			c.OnShutdown(func() error {
				log.Debugf("[%s] GCP Cloud DNS plugin shutting down", h.name)
				cancel()
				return nil
			})
			return nil
		})

		dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
			h.Next = next
			return h
		})
	}
	return nil
}
