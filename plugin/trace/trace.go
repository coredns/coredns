// Package trace implements OpenTracing-based tracing
package trace

import (
	"context"
	"fmt"
	stdlog "log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metadata"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/rcode"
	_ "github.com/coredns/coredns/plugin/pkg/trace" // Plugin the trace package.
	"github.com/coredns/coredns/request"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/ext"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	"github.com/miekg/dns"
	ot "github.com/opentracing/opentracing-go"
	otext "github.com/opentracing/opentracing-go/ext"
	otlog "github.com/opentracing/opentracing-go/log"
	zipkinot "github.com/openzipkin-contrib/zipkin-go-opentracing"
	"github.com/openzipkin/zipkin-go"
	zipkinhttp "github.com/openzipkin/zipkin-go/reporter/http"
)

const (
	defaultTopLevelSpanName = "servedns"
	metaTraceIdKey          = "trace/traceid"
)

var log = clog.NewWithPlugin("trace")

type traceTags struct {
	Name   string
	Type   string
	Rcode  string
	Proto  string
	Remote string
}

var tagByProvider = map[string]traceTags{
	"default": {
		Name:   "coredns.io/name",
		Type:   "coredns.io/type",
		Rcode:  "coredns.io/rcode",
		Proto:  "coredns.io/proto",
		Remote: "coredns.io/remote",
	},
	"datadog": {
		Name:   "coredns.io@name",
		Type:   "coredns.io@type",
		Rcode:  "coredns.io@rcode",
		Proto:  "coredns.io@proto",
		Remote: "coredns.io@remote",
	},
}

type trace struct {
	count atomic.Uint64 // as per Go spec, needs to be first element in a struct

	Next                   plugin.Handler
	Endpoint               string
	EndpointType           string
	zipkinTracer           ot.Tracer
	serviceEndpoint        string
	serviceName            string
	clientServer           bool
	every                  uint64
	datadogAnalyticsRate   float64
	zipkinMaxBacklogSize   int
	zipkinMaxBatchSize     int
	zipkinMaxBatchInterval time.Duration
	Once                   sync.Once
	tagSet                 traceTags
}

func (t *trace) Tracer() ot.Tracer {
	return t.zipkinTracer
}

// OnStartup sets up the tracer
func (t *trace) OnStartup() error {
	var err error
	t.Once.Do(func() {
		switch t.EndpointType {
		case "zipkin":
			err = t.setupZipkin()
		case "datadog":
			tracer.Start(
				tracer.WithAgentAddr(t.Endpoint),
				tracer.WithDebugMode(clog.D.Value()),
				tracer.WithGlobalTag(ext.SpanTypeDNS, true),
				tracer.WithService(t.serviceName),
				tracer.WithAnalyticsRate(t.datadogAnalyticsRate),
				tracer.WithLogger(&loggerAdapter{log}),
			)
			t.tagSet = tagByProvider["datadog"]
		default:
			err = fmt.Errorf("unknown endpoint type: %s", t.EndpointType)
		}
	})
	return err
}

// OnShutdown cleans up the tracer
func (t *trace) OnShutdown() error {
	if t.EndpointType == "datadog" {
		tracer.Stop()
	}
	return nil
}

func (t *trace) setupZipkin() error {
	var opts []zipkinhttp.ReporterOption
	opts = append(opts, zipkinhttp.Logger(stdlog.New(&loggerAdapter{log}, "", 0)))
	if t.zipkinMaxBacklogSize != 0 {
		opts = append(opts, zipkinhttp.MaxBacklog(t.zipkinMaxBacklogSize))
	}
	if t.zipkinMaxBatchSize != 0 {
		opts = append(opts, zipkinhttp.BatchSize(t.zipkinMaxBatchSize))
	}
	if t.zipkinMaxBatchInterval != 0 {
		opts = append(opts, zipkinhttp.BatchInterval(t.zipkinMaxBatchInterval))
	}
	reporter := zipkinhttp.NewReporter(t.Endpoint, opts...)
	recorder, err := zipkin.NewEndpoint(t.serviceName, t.serviceEndpoint)
	if err != nil {
		log.Warningf("build Zipkin endpoint found err: %v", err)
	}
	tracer, err := zipkin.NewTracer(
		reporter,
		zipkin.WithLocalEndpoint(recorder),
		zipkin.WithSharedSpans(t.clientServer),
	)
	if err != nil {
		return err
	}
	t.zipkinTracer = zipkinot.Wrap(tracer)

	t.tagSet = tagByProvider["default"]
	return err
}

// Name implements the Handler interface.
func (t *trace) Name() string { return "trace" }

// ServeDNS implements the plugin.Handle interface.
func (t *trace) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	shouldTrace := false
	if t.every > 0 {
		queryNr := t.count.Add(1)
		if queryNr%t.every == 0 {
			shouldTrace = true
		}
	}

	if t.EndpointType == "datadog" {
		return t.serveDNSDatadog(ctx, w, r, shouldTrace)
	}
	return t.serveDNSZipkin(ctx, w, r, shouldTrace)
}

func (t *trace) serveDNSDatadog(ctx context.Context, w dns.ResponseWriter, r *dns.Msg, shouldTrace bool) (int, error) {
	if !shouldTrace {
		return plugin.NextOrFailure(t.Name(), t.Next, ctx, w, r)
	}

	span, spanCtx := tracer.StartSpanFromContext(ctx, defaultTopLevelSpanName)
	defer span.Finish()

	metadata.SetValueFunc(ctx, metaTraceIdKey, func() string { return span.Context().TraceID() })

	req := request.Request{W: w, Req: r}
	rw := dnstest.NewRecorder(w)
	status, err := plugin.NextOrFailure(t.Name(), t.Next, spanCtx, rw, r)

	t.setDatadogSpanTags(span, req, rw, status, err)

	return status, err
}

func (t *trace) serveDNSZipkin(ctx context.Context, w dns.ResponseWriter, r *dns.Msg, shouldTrace bool) (int, error) {
	span := ot.SpanFromContext(ctx)
	if !shouldTrace || span != nil {
		return plugin.NextOrFailure(t.Name(), t.Next, ctx, w, r)
	}

	var spanCtx ot.SpanContext
	if val := ctx.Value(dnsserver.HTTPRequestKey{}); val != nil {
		if httpReq, ok := val.(*http.Request); ok {
			spanCtx, _ = t.Tracer().Extract(ot.HTTPHeaders, ot.HTTPHeadersCarrier(httpReq.Header))
		}
	}

	req := request.Request{W: w, Req: r}
	span = t.Tracer().StartSpan(defaultTopLevelSpanName, otext.RPCServerOption(spanCtx))
	defer span.Finish()

	if spanCtx, ok := span.Context().(zipkinot.SpanContext); ok {
		metadata.SetValueFunc(ctx, metaTraceIdKey, func() string { return spanCtx.TraceID.String() })
	}

	rw := dnstest.NewRecorder(w)
	ctx = ot.ContextWithSpan(ctx, span)
	status, err := plugin.NextOrFailure(t.Name(), t.Next, ctx, rw, r)

	t.setZipkinSpanTags(span, req, rw, status, err)

	return status, err
}

// setDatadogSpanTags sets span tags using DataDog v2 API
func (t *trace) setDatadogSpanTags(span *tracer.Span, req request.Request, rw *dnstest.Recorder, status int, err error) {
	span.SetTag(t.tagSet.Name, req.Name())
	span.SetTag(t.tagSet.Type, req.Type())
	span.SetTag(t.tagSet.Proto, req.Proto())
	span.SetTag(t.tagSet.Remote, req.IP())
	rc := rw.Rcode
	if !plugin.ClientWrite(status) {
		rc = status
	}
	span.SetTag(t.tagSet.Rcode, rcode.ToString(rc))
	if err != nil {
		span.SetTag("error.message", err.Error())
		span.SetTag("error", true)
		span.SetTag("error.type", "dns_error")
	}
}

// setZipkinSpanTags sets span tags for Zipkin/OpenTracing spans
func (t *trace) setZipkinSpanTags(span ot.Span, req request.Request, rw *dnstest.Recorder, status int, err error) {
	span.SetTag(t.tagSet.Name, req.Name())
	span.SetTag(t.tagSet.Type, req.Type())
	span.SetTag(t.tagSet.Proto, req.Proto())
	span.SetTag(t.tagSet.Remote, req.IP())
	rc := rw.Rcode
	if !plugin.ClientWrite(status) {
		// when no response was written, fallback to status returned from next plugin as this status
		// is actually used as rcode of DNS response
		// see https://github.com/coredns/coredns/blob/master/core/dnsserver/server.go#L318
		rc = status
	}
	span.SetTag(t.tagSet.Rcode, rcode.ToString(rc))
	if err != nil {
		// Use OpenTracing error handling
		otext.Error.Set(span, true)
		span.LogFields(otlog.Event("error"), otlog.Error(err))
	}
}
