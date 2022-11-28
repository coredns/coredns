package opentelemetry

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"

	"go.opentelemetry.io/otel/sdk/trace"
)

func init() { plugin.Register("opentelemetry", setup) }

func setup(c *caddy.Controller) error {
	ot, err := opentelemetryParse(c)
	if err != nil {
		return plugin.Error("opentelemetry", err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		ot.Next = next
		return ot
	})

	c.OnStartup(func() error { return ot.OnStartup(c) })
	c.OnFinalShutdown(func() error {
		if ot.ShutdownFunc != nil {
			return ot.ShutdownFunc(context.Background())
		}
		return nil
	})

	return nil
}

func opentelemetryParse(c *caddy.Controller) (*opentelemetry, error) {
	var (
		ot  = &opentelemetry{serviceName: defServiceName}
		err error
	)

	for c.Next() {
		args := c.RemainingArgs()
		switch len(args) {
		case 0:
			ot.EndpointType, ot.Endpoint, err = normalizeEndpoint(defEpType, "")
		case 1:
			ot.EndpointType, ot.Endpoint, err = normalizeEndpoint(defEpType, args[0])
		case 2:
			epType := strings.ToLower(args[0])
			ot.EndpointType, ot.Endpoint, err = normalizeEndpoint(epType, args[1])
		default:
			err = c.ArgErr()
		}
		if err != nil {
			return ot, err
		}

		ot.batchSpanOptions = trace.BatchSpanProcessorOptions{
			MaxQueueSize:       2048,
			BatchTimeout:       5 * time.Second,
			ExportTimeout:      30 * time.Second,
			MaxExportBatchSize: 512,
		}
		ot.SamplingProbability = 1
		ot.Insecure = false

		for c.NextBlock() {
			switch c.Val() {
			case "service":
				args := c.RemainingArgs()
				if len(args) != 1 {
					return nil, c.ArgErr()
				}
				ot.serviceName = args[0]
			case "max_queue_size":
				args := c.RemainingArgs()
				if len(args) != 1 {
					return nil, c.ArgErr()
				}
				if ot.batchSpanOptions.MaxQueueSize, err = strconv.Atoi(args[0]); err != nil {
					return ot, err
				}
			case "batch_timeout":
				args := c.RemainingArgs()
				if len(args) != 1 {
					return nil, c.ArgErr()
				}
				if ot.batchSpanOptions.BatchTimeout, err = time.ParseDuration(args[0]); err != nil {
					return nil, err
				}
			case "export_timeout":
				args := c.RemainingArgs()
				if len(args) != 1 {
					return nil, c.ArgErr()
				}
				if ot.batchSpanOptions.ExportTimeout, err = time.ParseDuration(args[0]); err != nil {
					return nil, err
				}
			case "max_export_batch_size":
				args := c.RemainingArgs()
				if len(args) != 1 {
					return nil, c.ArgErr()
				}
				if ot.batchSpanOptions.MaxExportBatchSize, err = strconv.Atoi(args[0]); err != nil {
					return nil, err
				}
			case "sampling_probability":
				args := c.RemainingArgs()
				if len(args) != 1 {
					return nil, c.ArgErr()
				}
				if ot.SamplingProbability, err = strconv.ParseFloat(args[0], 64); err != nil {
					return nil, err
				}
				if ot.SamplingProbability < 0 || ot.SamplingProbability > 1 {
					return nil, fmt.Errorf("sampling probability needs to be in ragne 0<=n<=1")
				}
			case "insecure":
				args := c.RemainingArgs()
				if len(args) != 0 {
					return nil, c.ArgErr()
				}
				ot.Insecure = true
			default:
				log.Warningf("unknown argument while parsing configuration '%s'", c.Val())
			}
		}
	}

	return ot, err
}

func normalizeEndpoint(epType, ep string) (string, string, error) {
	if _, ok := supportedProviders[epType]; !ok {
		return "", "", fmt.Errorf("tracing endpoint type '%s' is not supported", epType)
	}

	if ep == "" {
		ep = supportedProviders[epType]
	}

	if epType == "zipkin" {
		if !strings.Contains(ep, "http") {
			ep = "http://" + ep + "/api/v2/spans"
		}
	} else if epType == "otelhttp" {
		if strings.Contains(ep, "http") || strings.Contains(ep, "/") {
			return "", "", fmt.Errorf("invalid format of OpenTelemetry endpoint: '%s', valid format: 'HOST:PORT'", ep)
		}
	}

	return epType, ep, nil
}

var supportedProviders = map[string]string{
	"zipkin":   "localhost:9411",
	"otelhttp": "localhost:4318",
}
