package trace

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/rcode"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/mocktracer"
	"github.com/miekg/dns"
	"github.com/opentracing/opentracing-go"
	openTracingMock "github.com/opentracing/opentracing-go/mocktracer"
)

func TestStartup(t *testing.T) {
	m, err := traceParse(caddy.NewTestController("dns", `trace`))
	if err != nil {
		t.Errorf("Error parsing test input: %s", err)
		return
	}
	if m.Name() != "trace" {
		t.Errorf("Wrong name from GetName: %s", m.Name())
	}
	err = m.OnStartup()
	if err != nil {
		t.Errorf("Error starting tracing plugin: %s", err)
		return
	}

	if m.tagSet != tagByProvider["default"] {
		t.Errorf("TagSet by proviser hasn't been correctly initialized")
	}

	if m.Tracer() == nil {
		t.Errorf("Error, no tracer created")
	}
}

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
	defaultTagSet := tagByProvider["default"]
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := dnstest.NewRecorder(&test.ResponseWriter{})
			m := openTracingMock.New()
			tr := &trace{
				Next: test.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
					if plugin.ClientWrite(tc.status) {
						m := new(dns.Msg)
						m.SetRcode(r, tc.rcode)
						w.WriteMsg(m)
					}
					return tc.status, tc.err
				}),
				every:        1,
				zipkinTracer: m,
				tagSet:       defaultTagSet,
			}
			ctx := context.TODO()
			if _, err := tr.ServeDNS(ctx, w, tc.question); err != nil && tc.err == nil {
				t.Fatalf("Error during tr.ServeDNS(ctx, w, %v): %v", tc.question, err)
			}

			fs := m.FinishedSpans()
			// Each trace consists of two spans; the root and the Next function.
			if len(fs) != 2 {
				t.Fatalf("Unexpected span count: len(fs): want 2, got %v", len(fs))
			}

			rootSpan := fs[1]
			req := request.Request{W: w, Req: tc.question}
			if rootSpan.OperationName != defaultTopLevelSpanName {
				t.Errorf("Unexpected span name: rootSpan.Name: want %v, got %v", defaultTopLevelSpanName, rootSpan.OperationName)
			}

			if rootSpan.Tag(defaultTagSet.Name) != req.Name() {
				t.Errorf("Unexpected span tag: rootSpan.Tag(%v): want %v, got %v", defaultTagSet.Name, req.Name(), rootSpan.Tag(defaultTagSet.Name))
			}
			if rootSpan.Tag(defaultTagSet.Type) != req.Type() {
				t.Errorf("Unexpected span tag: rootSpan.Tag(%v): want %v, got %v", defaultTagSet.Type, req.Type(), rootSpan.Tag(defaultTagSet.Type))
			}
			if rootSpan.Tag(defaultTagSet.Proto) != req.Proto() {
				t.Errorf("Unexpected span tag: rootSpan.Tag(%v): want %v, got %v", defaultTagSet.Proto, req.Proto(), rootSpan.Tag(defaultTagSet.Proto))
			}
			if rootSpan.Tag(defaultTagSet.Remote) != req.IP() {
				t.Errorf("Unexpected span tag: rootSpan.Tag(%v): want %v, got %v", defaultTagSet.Remote, req.IP(), rootSpan.Tag(defaultTagSet.Remote))
			}
			if rootSpan.Tag(defaultTagSet.Rcode) != rcode.ToString(tc.rcode) {
				t.Errorf("Unexpected span tag: rootSpan.Tag(%v): want %v, got %v", defaultTagSet.Rcode, rcode.ToString(tc.rcode), rootSpan.Tag(defaultTagSet.Rcode))
			}
			if tc.err != nil && rootSpan.Tag("error") != true {
				t.Errorf("Unexpected span tag: rootSpan.Tag(%v): want %v, got %v", "error", true, rootSpan.Tag("error"))
			}
		})
	}
}

func TestTrace_DOH_TraceHeaderExtraction(t *testing.T) {
	w := dnstest.NewRecorder(&test.ResponseWriter{})
	m := openTracingMock.New()
	tr := &trace{
		Next: test.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
			if plugin.ClientWrite(dns.RcodeSuccess) {
				m := new(dns.Msg)
				m.SetRcode(r, dns.RcodeSuccess)
				w.WriteMsg(m)
			}
			return dns.RcodeSuccess, nil
		}),
		every:        1,
		zipkinTracer: m,
	}
	q := new(dns.Msg).SetQuestion("example.net.", dns.TypeA)

	req := httptest.NewRequest(http.MethodPost, "/dns-query", nil)

	outsideSpan := m.StartSpan("test-header-span")
	outsideSpan.Tracer().Inject(outsideSpan.Context(), opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(req.Header))
	defer outsideSpan.Finish()

	ctx := context.TODO()
	ctx = context.WithValue(ctx, dnsserver.HTTPRequestKey{}, req)

	tr.ServeDNS(ctx, w, q)

	fs := m.FinishedSpans()
	rootCoreDNSspan := fs[1]
	rootCoreDNSTraceID := rootCoreDNSspan.Context().(openTracingMock.MockSpanContext).TraceID
	outsideSpanTraceID := outsideSpan.Context().(openTracingMock.MockSpanContext).TraceID
	if rootCoreDNSTraceID != outsideSpanTraceID {
		t.Errorf("Unexpected traceID: rootSpan.TraceID: want %v, got %v", rootCoreDNSTraceID, outsideSpanTraceID)
	}
}

func TestStartup_Datadog(t *testing.T) {
	m, err := traceParse(caddy.NewTestController("dns", `trace datadog localhost:8126`))
	if err != nil {
		t.Errorf("Error parsing test input: %s", err)
		return
	}
	if m.Name() != "trace" {
		t.Errorf("Wrong name from GetName: %s", m.Name())
	}

	// Test that we can start and stop the DataDog tracer without errors
	err = m.OnStartup()
	if err != nil {
		t.Errorf("Error starting DataDog tracing plugin: %s", err)
		return
	}

	if m.tagSet != tagByProvider["datadog"] {
		t.Errorf("TagSet for DataDog hasn't been correctly initialized")
	}

	// Test shutdown
	err = m.OnShutdown()
	if err != nil {
		t.Errorf("Error shutting down DataDog tracing plugin: %s", err)
	}
}

func TestTrace_DataDog(t *testing.T) {
	// Test the complete DataDog tracing flow using mocktracer
	mt := mocktracer.Start()
	defer mt.Stop()

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
			name:     "SERVFAIL with error",
			rcode:    dns.RcodeServerFailure,
			status:   dns.RcodeSuccess,
			question: new(dns.Msg).SetQuestion("example.net.", dns.TypeA),
			err:      errors.New("test error"),
		},
	}

	datadogTagSet := tagByProvider["datadog"]
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset spans for each test
			mt.Reset()

			w := dnstest.NewRecorder(&test.ResponseWriter{})
			tr := &trace{
				Next: test.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
					if plugin.ClientWrite(tc.status) {
						m := new(dns.Msg)
						m.SetRcode(r, tc.rcode)
						w.WriteMsg(m)
					}
					return tc.status, tc.err
				}),
				every:        1,
				EndpointType: "datadog",
				tagSet:       datadogTagSet,
			}

			ctx := context.TODO()
			if _, err := tr.ServeDNS(ctx, w, tc.question); err != nil && tc.err == nil {
				t.Fatalf("Error during tr.ServeDNS(ctx, w, %v): %v", tc.question, err)
			}

			spans := mt.FinishedSpans()
			if len(spans) == 0 {
				t.Fatal("Expected at least one span, got none")
			}

			// Find the DNS span
			var dnsSpan *mocktracer.Span
			for _, span := range spans {
				if span.OperationName() == defaultTopLevelSpanName {
					dnsSpan = span
					break
				}
			}

			if dnsSpan == nil {
				t.Fatal("Could not find DNS span with operation name 'servedns'")
			}

			req := request.Request{W: w, Req: tc.question}

			// Test DataDog-specific tags
			if dnsSpan.Tag(datadogTagSet.Name) != req.Name() {
				t.Errorf("Unexpected span tag: span.Tag(%v): want %v, got %v",
					datadogTagSet.Name, req.Name(), dnsSpan.Tag(datadogTagSet.Name))
			}
			if dnsSpan.Tag(datadogTagSet.Type) != req.Type() {
				t.Errorf("Unexpected span tag: span.Tag(%v): want %v, got %v",
					datadogTagSet.Type, req.Type(), dnsSpan.Tag(datadogTagSet.Type))
			}
			if dnsSpan.Tag(datadogTagSet.Proto) != req.Proto() {
				t.Errorf("Unexpected span tag: span.Tag(%v): want %v, got %v",
					datadogTagSet.Proto, req.Proto(), dnsSpan.Tag(datadogTagSet.Proto))
			}
			if dnsSpan.Tag(datadogTagSet.Remote) != req.IP() {
				t.Errorf("Unexpected span tag: span.Tag(%v): want %v, got %v",
					datadogTagSet.Remote, req.IP(), dnsSpan.Tag(datadogTagSet.Remote))
			}
			if dnsSpan.Tag(datadogTagSet.Rcode) != rcode.ToString(tc.rcode) {
				t.Errorf("Unexpected span tag: span.Tag(%v): want %v, got %v",
					datadogTagSet.Rcode, rcode.ToString(tc.rcode), dnsSpan.Tag(datadogTagSet.Rcode))
			}

			// Test DataDog v2 error handling
			if tc.err != nil {
				errorMsg := dnsSpan.Tag("error.message")
				if errorMsg == nil {
					t.Error("Expected error.message tag to be set")
				} else if !strings.Contains(errorMsg.(string), "test error") {
					t.Errorf("Expected error.message to contain 'test error', got %v", errorMsg)
				}

				// Check error type tag
				errorType := dnsSpan.Tag("error.type")
				if errorType == nil {
					t.Error("Expected error.type tag to be set")
				}
			}

			// Verify trace ID exists (mocktracer uses uint64)
			traceID := dnsSpan.TraceID()
			if traceID == 0 {
				t.Error("Expected non-zero trace ID")
			}
		})
	}
}
