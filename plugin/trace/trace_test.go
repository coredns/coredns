package trace

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/rcode"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/mocktracer"
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
			m := mocktracer.New()
			tr := &trace{
				Next: test.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
					if plugin.ClientWrite(tc.status) {
						m := new(dns.Msg)
						m.SetRcode(r, tc.rcode)
						w.WriteMsg(m)
					}
					return tc.status, tc.err
				}),
				every:  1,
				tracer: m,
				tagSet: defaultTagSet,
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
	m := mocktracer.New()
	tr := &trace{
		Next: test.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
			if plugin.ClientWrite(dns.RcodeSuccess) {
				m := new(dns.Msg)
				m.SetRcode(r, dns.RcodeSuccess)
				w.WriteMsg(m)
			}
			return dns.RcodeSuccess, nil
		}),
		every:  1,
		tracer: m,
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
	rootCoreDNSTraceID := rootCoreDNSspan.Context().(mocktracer.MockSpanContext).TraceID
	outsideSpanTraceID := outsideSpan.Context().(mocktracer.MockSpanContext).TraceID
	if rootCoreDNSTraceID != outsideSpanTraceID {
		t.Errorf("Unexpected traceID: rootSpan.TraceID: want %v, got %v", rootCoreDNSTraceID, outsideSpanTraceID)
	}
}
