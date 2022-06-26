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

	"github.com/miekg/dns"
	ot "github.com/opentracing/opentracing-go"
	otext "github.com/opentracing/opentracing-go/ext"
	otlog "github.com/opentracing/opentracing-go/log"
	zipkinot "github.com/openzipkin-contrib/zipkin-go-opentracing"
	"github.com/openzipkin/zipkin-go"
	zipkinhttp "github.com/openzipkin/zipkin-go/reporter/http"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/ext"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/opentracer"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
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
	count uint64 // as per Go spec, needs to be first element in a struct

	Next                   plugin.Handler
	Endpoint               string
	EndpointType           string
	tracer                 ot.Tracer
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
	return t.tracer
}

// OnStartup sets up the tracer
func (t *trace) OnStartup() error {
	var err error
	t.Once.Do(func() {
		switch t.EndpointType {
		case "zipkin":
			err = t.setupZipkin()
		case "datadog":
			tracer := opentracer.New(
				tracer.WithAgentAddr(t.Endpoint),
				tracer.WithDebugMode(clog.D.Value()),
				tracer.WithGlobalTag(ext.SpanTypeDNS, true),
				tracer.WithServiceName(t.serviceName),
				tracer.WithAnalyticsRate(t.datadogAnalyticsRate),
				tracer.WithLogger(&loggerAdapter{log}),
			)
			t.tracer = tracer
			t.tagSet = tagByProvider["datadog"]
		default:
			err = fmt.Errorf("unknown endpoint type: %s", t.EndpointType)
		}
	})
	return err
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
	t.tracer = zipkinot.Wrap(tracer)

	t.tagSet = tagByProvider["default"]
	return err
}

// Name implements the Handler interface.
func (t *trace) Name() string { return "trace" }

// ServeDNS implements the plugin.Handle interface.
func (t *trace) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	trace := false
	if t.every > 0 {
		queryNr := atomic.AddUint64(&t.count, 1)

		if queryNr%t.every == 0 {
			trace = true
		}
	}
	span := ot.SpanFromContext(ctx)
	if !trace || span != nil {
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

	switch spanCtx := span.Context().(type) {
	case zipkinot.SpanContext:
		metadata.SetValueFunc(ctx, metaTraceIdKey, func() string { return spanCtx.TraceID.String() })
	case ddtrace.SpanContext:
		metadata.SetValueFunc(ctx, metaTraceIdKey, func() string { return fmt.Sprint(spanCtx.TraceID()) })
	}

	rw := dnstest.NewRecorder(w)
	ctx = ot.ContextWithSpan(ctx, span)
	status, err := plugin.NextOrFailure(t.Name(), t.Next, ctx, rw, r)

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
		otext.Error.Set(span, true)
		span.LogFields(otlog.Event("error"), otlog.Error(err))
	}

	return status, err
}
