// Package trace implements OpenTracing-based tracing
package trace

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/rcode"
	_ "github.com/coredns/coredns/plugin/pkg/trace" // Plugin the trace package.
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	ot "github.com/opentracing/opentracing-go"
	zipkinot "github.com/openzipkin-contrib/zipkin-go-opentracing"
	"github.com/openzipkin/zipkin-go"
	zipkinhttp "github.com/openzipkin/zipkin-go/reporter/http"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/ext"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/opentracer"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

const (
	defaultTopLevelSpanName = "servedns"
)

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

	Next                 plugin.Handler
	Endpoint             string
	EndpointType         string
	tracer               ot.Tracer
	serviceEndpoint      string
	serviceName          string
	clientServer         bool
	every                uint64
	datadogAnalyticsRate float64
	Once                 sync.Once
	tagSet               traceTags
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
				tracer.WithDebugMode(log.D.Value()),
				tracer.WithGlobalTag(ext.SpanTypeDNS, true),
				tracer.WithServiceName(t.serviceName),
				tracer.WithAnalyticsRate(t.datadogAnalyticsRate),
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
	reporter := zipkinhttp.NewReporter(t.Endpoint)
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

	req := request.Request{W: w, Req: r}
	span = t.Tracer().StartSpan(defaultTopLevelSpanName)
	defer span.Finish()

	rw := dnstest.NewRecorder(w)
	ctx = ot.ContextWithSpan(ctx, span)
	status, err := plugin.NextOrFailure(t.Name(), t.Next, ctx, rw, r)

	span.SetTag(t.tagSet.Name, req.Name())
	span.SetTag(t.tagSet.Type, req.Type())
	span.SetTag(t.tagSet.Proto, req.Proto())
	span.SetTag(t.tagSet.Remote, req.IP())
	span.SetTag(t.tagSet.Rcode, rcode.ToString(rw.Rcode))

	return status, err
}
