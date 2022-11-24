package opentelemetry

import (
	"context"
	"errors"
	"fmt"
	stdlog "log"
	"net/http"
	"sync"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/coremain"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metadata"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/rcode"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/zipkin"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"
	traceapi "go.opentelemetry.io/otel/trace"
)

const (
	pluginName              = "opentelemetry"
	defEpType               = "zipkin"
	defServiceName          = "coredns"
	defaultTopLevelSpanName = "servedns"
	metaTraceIdKey          = "opentelemetry/traceid"
)

var log = clog.NewWithPlugin(pluginName)

type opentelemetry struct {
	Next                plugin.Handler
	Endpoint            string
	EndpointType        string
	serviceName         string
	tracer              traceapi.Tracer
	Once                sync.Once
	ShutdownFunc        func(context.Context) error
	SamplingProbability float64
	Insecure            bool

	batchSpanOptions tracesdk.BatchSpanProcessorOptions
}

func (ot *opentelemetry) OnStartup(c *caddy.Controller) error {
	if isTracePluginEnabled(dnsserver.GetConfig(c)) {
		return errors.New("cannot use 'opentelemetry' plugin when 'trace' plugin is enabled")
	}

	var err error
	ot.Once.Do(func() {
		var exporter tracesdk.SpanExporter
		exporter, err = ot.createExporter()
		if err == nil {
			ot.setupTraceProvider(exporter)
		}
	})

	return err
}

func isTracePluginEnabled(cfg *dnsserver.Config) bool {
	if t := cfg.Handler("trace"); t != nil {
		return true
	}
	return false
}

func (ot *opentelemetry) createExporter() (tracesdk.SpanExporter, error) {
	switch ot.EndpointType {
	case "zipkin":
		return ot.createZipkinExporter()
	case "otelhttp":
		return ot.createOpenTelemetryExporter()
	default:
		return nil, fmt.Errorf("unknown endpoint type: %s", ot.EndpointType)
	}
}

func (ot *opentelemetry) createZipkinExporter() (tracesdk.SpanExporter, error) {
	exp, err := zipkin.New(ot.Endpoint, zipkin.WithLogger(stdlog.New(&loggerAdapter{log}, "", 0)))
	if err != nil {
		return nil, fmt.Errorf("error while creating zipkin exporter: %w", err)
	}

	return exp, nil
}

func (ot *opentelemetry) createOpenTelemetryExporter() (tracesdk.SpanExporter, error) {
	var opts []otlptracehttp.Option
	opts = append(opts, otlptracehttp.WithEndpoint(ot.Endpoint))
	if ot.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	client := otlptracehttp.NewClient(opts...)
	exp, err := otlptrace.New(context.Background(), client)
	if err != nil {
		return nil, fmt.Errorf("error while creating OTLP exporter: %w", err)
	}

	return exp, nil
}

func (ot *opentelemetry) setupTraceProvider(exporter tracesdk.SpanExporter) {
	batcher := tracesdk.NewBatchSpanProcessor(
		exporter,
		tracesdk.WithMaxQueueSize(ot.batchSpanOptions.MaxQueueSize),
		tracesdk.WithBatchTimeout(ot.batchSpanOptions.BatchTimeout),
		tracesdk.WithExportTimeout(ot.batchSpanOptions.ExportTimeout),
		tracesdk.WithMaxExportBatchSize(ot.batchSpanOptions.MaxExportBatchSize),
	)

	tp := tracesdk.NewTracerProvider(
		tracesdk.WithSpanProcessor(batcher),
		tracesdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(ot.serviceName),
			semconv.ServiceVersionKey.String(coremain.CoreVersion),
		)),
		tracesdk.WithSampler(tracesdk.TraceIDRatioBased(ot.SamplingProbability)),
	)

	ot.ShutdownFunc = tp.Shutdown
	otel.SetTracerProvider(tp)
	ot.tracer = otel.GetTracerProvider().Tracer(plugin.Namespace)
}

// Name implements the Handler interface.
func (ot *opentelemetry) Name() string { return pluginName }

const (
	attrKeyName   = "coredns.io/name"
	attrKeyType   = "coredns.io/type"
	attrKeyProto  = "coredns.io/proto"
	attrKeyRemote = "coredns.io/remote"
	attrKeyRcode  = "coredns.io/rcode"
)

func (ot *opentelemetry) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	if val := ctx.Value(dnsserver.HTTPRequestKey{}); val != nil {
		if httpReq, ok := val.(*http.Request); ok {
			ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(httpReq.Header))
		}
	}
	req := request.Request{W: w, Req: r}
	var span traceapi.Span
	ctx, span = ot.tracer.Start(ctx, defaultTopLevelSpanName)
	defer span.End()

	metadata.SetValueFunc(ctx, metaTraceIdKey, func() string { return span.SpanContext().TraceID().String() })

	rw := dnstest.NewRecorder(w)
	st, err := plugin.NextOrFailure(ot.Name(), ot.Next, ctx, rw, r)

	span.SetAttributes(
		attribute.KeyValue{
			Key:   attrKeyName,
			Value: attribute.StringValue(req.Name()),
		},
		attribute.KeyValue{
			Key:   attrKeyType,
			Value: attribute.StringValue(req.Type()),
		},
		attribute.KeyValue{
			Key:   attrKeyProto,
			Value: attribute.StringValue(req.Proto()),
		},
		attribute.KeyValue{
			Key:   attrKeyRemote,
			Value: attribute.StringValue(req.IP()),
		},
	)

	rc := rw.Rcode
	if !plugin.ClientWrite(st) {
		// when no response was written, fallback to status returned from next plugin as this status
		// is actually used as rcode of DNS response
		// see https://github.com/coredns/coredns/blob/master/core/dnsserver/server.go#L318
		rc = st
	}
	span.SetAttributes(attribute.KeyValue{
		Key:   attrKeyRcode,
		Value: attribute.StringValue(rcode.ToString(rc)),
	})

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return st, err
}
