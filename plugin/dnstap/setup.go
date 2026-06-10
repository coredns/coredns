package dnstap

import (
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/replacer"
)

var log = clog.NewWithPlugin("dnstap")

func init() { plugin.Register("dnstap", setup) }

const (
	// Upper bounds chosen to keep memory use and kernel socket buffer requests reasonable
	// while allowing large configurations. Write buffer multiple is in MiB units; queue
	// multiple is applied to 10,000 messages. See plugin README for parameter semantics.
	maxMultipleTcpWriteBuf = 1024 // up to 1 GiB write buffer per TCP connection
	maxMultipleQueue       = 4096 // up to 40,960,000 enqueued messages
)

func parseConfig(c *caddy.Controller) ([]*Dnstap, error) {
	dnstaps := []*Dnstap{}

	for c.Next() { // directive name
		d := Dnstap{
			MultipleTcpWriteBuf: 1,
			MultipleQueue:       1,
		}

		d.repl = replacer.New()

		args := c.RemainingArgs()

		if len(args) == 0 {
			return nil, c.ArgErr()
		}

		endpoint := args[0]

		// Check if this is a 'listen' directive for incoming connections
		isListener := endpoint == "listen"
		if isListener {
			if len(args) < 2 {
				return nil, c.Errf("dnstap listen requires an endpoint argument")
			}
			endpoint = args[1]
			// Shift args for listener mode
			args = args[1:]
		}

		if len(args) >= 3 {
			tcpWriteBuf := args[2]
			v, err := strconv.Atoi(tcpWriteBuf)
			if err != nil {
				return nil, c.Errf("dnstap: invalid MultipleTcpWriteBuf %q: %v", tcpWriteBuf, err)
			}
			if v < 1 || v > maxMultipleTcpWriteBuf {
				return nil, c.Errf("dnstap: MultipleTcpWriteBuf must be between 1 and %d (MiB units): %d", maxMultipleTcpWriteBuf, v)
			}
			d.MultipleTcpWriteBuf = v
		}
		if len(args) >= 4 {
			qSize := args[3]
			v, err := strconv.Atoi(qSize)
			if err != nil {
				return nil, c.Errf("dnstap: invalid MultipleQueue %q: %v", qSize, err)
			}
			if v < 1 || v > maxMultipleQueue {
				return nil, c.Errf("dnstap: MultipleQueue must be between 1 and %d (x10k messages): %d", maxMultipleQueue, v)
			}
			d.MultipleQueue = v
		}

		var dio *dio
		var lstnr *listener

		if isListener {
			// Incoming connection listener
			if strings.HasPrefix(endpoint, "tls://") {
				endpointURL, err := url.Parse(endpoint)
				if err != nil {
					return nil, c.ArgErr()
				}
				lstnr = newListener("tls", endpointURL.Host)
				d.listener = lstnr
			} else if strings.HasPrefix(endpoint, "tcp://") {
				endpointURL, err := url.Parse(endpoint)
				if err != nil {
					return nil, c.ArgErr()
				}
				lstnr = newListener("tcp", endpointURL.Host)
				d.listener = lstnr
			} else {
				endpoint = strings.TrimPrefix(endpoint, "unix://")
				lstnr = newListener("unix", endpoint)
				d.listener = lstnr
			}
		} else {
			// Outgoing connection
			if strings.HasPrefix(endpoint, "tls://") {
				// remote network endpoint
				endpointURL, err := url.Parse(endpoint)
				if err != nil {
					return nil, c.ArgErr()
				}
				dio = newIO("tls", endpointURL.Host, d.MultipleQueue, d.MultipleTcpWriteBuf)
				d.io = dio
			} else if strings.HasPrefix(endpoint, "tcp://") {
				// remote network endpoint
				endpointURL, err := url.Parse(endpoint)
				if err != nil {
					return nil, c.ArgErr()
				}
				dio = newIO("tcp", endpointURL.Host, d.MultipleQueue, d.MultipleTcpWriteBuf)
				d.io = dio
			} else {
				endpoint = strings.TrimPrefix(endpoint, "unix://")
				dio = newIO("unix", endpoint, d.MultipleQueue, d.MultipleTcpWriteBuf)
				d.io = dio
			}
		}

		d.IncludeRawMessage = len(args) >= 2 && args[1] == "full"

		hostname, _ := os.Hostname()
		d.Identity = []byte(hostname)
		d.Version = []byte(caddy.AppName + "-" + caddy.AppVersion)

		for c.NextBlock() {
			switch c.Val() {
			case "skipverify":
				{
					if isListener && lstnr != nil {
						lstnr.skipVerify = true
					} else if dio != nil {
						dio.skipVerify = true
					}
				}
			case "tls":
				{
					// TLS configuration for listeners: tls <cert> <key> [ca]
					if !isListener || lstnr == nil {
						return nil, c.Errf("tls directive only valid for listeners")
					}
					args := c.RemainingArgs()
					if len(args) < 2 {
						return nil, c.Errf("tls requires cert and key file paths")
					}
					lstnr.certFile = args[0]
					lstnr.keyFile = args[1]
					if len(args) >= 3 {
						lstnr.caFile = args[2]
					}
				}
			case "identity":
				{
					if !c.NextArg() {
						return nil, c.ArgErr()
					}
					d.Identity = []byte(c.Val())
				}
			case "version":
				{
					if !c.NextArg() {
						return nil, c.ArgErr()
					}
					d.Version = []byte(c.Val())
				}
			case "extra":
				{
					if !c.NextArg() {
						return nil, c.ArgErr()
					}
					d.ExtraFormat = c.Val()
				}
			default:
				return nil, c.Errf("unknown property '%s'", c.Val())
			}
		}
		dnstaps = append(dnstaps, &d)
	}
	return dnstaps, nil
}

func setup(c *caddy.Controller) error {
	dnstaps, err := parseConfig(c)
	if err != nil {
		return plugin.Error("dnstap", err)
	}

	for i := range dnstaps {
		dnstap := dnstaps[i]
		c.OnStartup(func() error {
			// Start outgoing connection if configured
			if dnstap.io != nil {
				if err := dnstap.io.(*dio).connect(); err != nil {
					log.Errorf("No connection to dnstap endpoint: %s", err)
				}
			}
			// Start listener if configured
			if dnstap.listener != nil {
				if err := dnstap.listener.listen(); err != nil {
					log.Errorf("Failed to start dnstap listener: %s", err)
				}
			}
			return nil
		})

		c.OnRestart(func() error {
			if dnstap.io != nil {
				dnstap.io.(*dio).close()
			}
			if dnstap.listener != nil {
				dnstap.listener.close()
			}
			return nil
		})

		c.OnFinalShutdown(func() error {
			if dnstap.io != nil {
				dnstap.io.(*dio).close()
			}
			if dnstap.listener != nil {
				dnstap.listener.close()
			}
			return nil
		})

		if i == len(dnstaps)-1 {
			// last dnstap plugin in block: point next to next plugin
			dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
				dnstap.Next = next
				return dnstap
			})
		} else {
			// not last dnstap plugin in block: point next to next dnstap
			nextDnstap := dnstaps[i+1]
			dnsserver.GetConfig(c).AddPlugin(func(plugin.Handler) plugin.Handler {
				dnstap.Next = nextDnstap
				return dnstap
			})
		}
	}

	return nil
}
