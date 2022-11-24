package opentelemetry

import (
	"context"
	"errors"
	"testing"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/rcode"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	"go.opentelemetry.io/otel/attribute"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestTrace(t *testing.T) {
	cases := []struct {
		name     string
		rcode    int
		status   int
		question *dns.Msg
		err      error
	}{
		{
			name:     "NXDOMAIN",
			rcode:    dns.RcodeNameError,
			status:   dns.RcodeSuccess,
			question: new(dns.Msg).SetQuestion("example.org.", dns.TypeA),
		},
		{
			name:     "NOERROR",
			rcode:    dns.RcodeSuccess,
			status:   dns.RcodeSuccess,
			question: new(dns.Msg).SetQuestion("example.net.", dns.TypeCNAME),
		},
		{
			name:     "SERVFAIL",
			rcode:    dns.RcodeServerFailure,
			status:   dns.RcodeSuccess,
			question: new(dns.Msg).SetQuestion("example.net.", dns.TypeA),
			err:      errors.New("test error"),
		},
		{
			name:     "No response written",
			rcode:    dns.RcodeServerFailure,
			status:   dns.RcodeServerFailure,
			question: new(dns.Msg).SetQuestion("example.net.", dns.TypeA),
			err:      errors.New("test error"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := dnstest.NewRecorder(&test.ResponseWriter{})

			next := test.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
				if plugin.ClientWrite(tc.status) {
					m := new(dns.Msg)
					m.SetRcode(r, tc.rcode)
					w.WriteMsg(m)
				}
				return tc.status, tc.err
			})

			var exporter = tracetest.InMemoryExporter{}
			processor := tracesdk.NewSimpleSpanProcessor(&exporter)

			tp := tracesdk.NewTracerProvider(
				tracesdk.WithSpanProcessor(processor),
			)

			tr := &opentelemetry{
				Next:   next,
				tracer: tp.Tracer("test"),
			}

			if _, err := tr.ServeDNS(context.TODO(), w, tc.question); err != nil && tc.err == nil {
				t.Fatalf("Error during tr.ServeDNS(ctx, w, %v): %v", tc.question, err)
			}

			spans := exporter.GetSpans()
			rootSpan := spans[1]
			req := request.Request{W: w, Req: tc.question}

			if rootSpan.Name != defaultTopLevelSpanName {
				t.Errorf("Unexpected span name: rootSpan.Name: want %v, got %v", defaultTopLevelSpanName, rootSpan.Name)
			}

			if val := findAttributeValueAsString(rootSpan.Attributes, attrKeyName); val != req.Name() {
				t.Errorf("Unexpected span tag: rootSpan.Attributes[%v): want %v, got %v", attrKeyName, req.Name(), val)
			}
			if val := findAttributeValueAsString(rootSpan.Attributes, attrKeyType); val != req.Type() {
				t.Errorf("Unexpected span tag: rootSpan.Attributes[%v): want %v, got %v", attrKeyType, req.Name(), val)
			}
			if val := findAttributeValueAsString(rootSpan.Attributes, attrKeyProto); val != req.Proto() {
				t.Errorf("Unexpected span tag: rootSpan.Attributes[%v): want %v, got %v", attrKeyProto, req.Name(), val)
			}
			if val := findAttributeValueAsString(rootSpan.Attributes, attrKeyRemote); val != req.IP() {
				t.Errorf("Unexpected span tag: rootSpan.Attributes[%v): want %v, got %v", attrKeyRemote, req.Name(), val)
			}
			if val := findAttributeValueAsString(rootSpan.Attributes, attrKeyRcode); val != rcode.ToString(tc.rcode) {
				t.Errorf("Unexpected span tag: rootSpan.Attributes[%v): want %v, got %v", attrKeyName, req.Name(), val)
			}
		})
	}
}

func findAttributeValueAsString(attrs []attribute.KeyValue, key string) string {
	for _, v := range attrs {
		if string(v.Key) == key {
			return v.Value.AsString()
		}
	}
	return ""
}
