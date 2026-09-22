package log

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	golog "log"
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metadata"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/replacer"
	"github.com/coredns/coredns/plugin/pkg/response"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

func jsonQueryOutput(tb testing.TB, format string, w io.Writer) {
	tb.Helper()
	output, flags, prefix := golog.Writer(), golog.Flags(), golog.Prefix()
	tb.Cleanup(func() {
		if err := clog.Configure("text", output); err != nil {
			tb.Error(err)
		}
		golog.SetFlags(flags)
		golog.SetPrefix(prefix)
	})
	if err := clog.Configure(format, w); err != nil {
		tb.Fatal(err)
	}
}

func TestJSONQueryEscaping(t *testing.T) {
	for _, name := range []string{"Example.ORG.", `a\.b.example.`, `a\001b.example.`, `a\"b.example.`, `a\\b.example.`} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			jsonQueryOutput(t, "json", &out)
			ctx := metadata.ContextWithMetadata(t.Context())
			value := "arbitrary \"metadata\"\nwith\t\\escapes"
			metadata.SetValueFunc(ctx, "test/value", func() string { return value })
			r := new(dns.Msg)
			r.SetQuestion(name, dns.TypeA)
			r.SetEdns0(1232, true)
			r.Id = 42
			wire, err := r.Pack()
			if err != nil {
				t.Fatal(err)
			}
			if err := r.Unpack(wire); err != nil {
				t.Fatal(err)
			}
			state := request.Request{Req: r}
			logger := Logger{
				Rules: []Rule{{NameScope: ".", Class: map[response.Class]struct{}{response.All: {}},
					Format: `{"name":"{name}","metadata":"{/test/value}"}`}},
				Next: test.NextHandler(dns.RcodeRefused, nil),
			}
			rec := dnstest.NewRecorder(&test.ResponseWriter{TCP: true, RemoteIP: "2001:db8::1"})
			rc, err := logger.ServeDNS(ctx, rec, r)
			if rc != dns.RcodeRefused || err != nil || rec.Msg != nil {
				t.Fatalf("logging changed deferred response: rc=%d, err=%v, msg=%v", rc, err, rec.Msg)
			}
			if strings.Count(out.String(), "\n") != 1 {
				t.Fatalf("not one physical line: %q", out.String())
			}
			var record map[string]any
			if err := json.Unmarshal(out.Bytes(), &record); err != nil {
				t.Fatalf("invalid JSON %q: %v", out.String(), err)
			}
			want := map[string]any{
				"plugin": "log", "level": "INFO", "qname": state.Name(), "qtype": "A", "qclass": "IN",
				"client_ip": "2001:db8::1", "client_port": float64(40212), "protocol": "tcp",
				"id": float64(42), "opcode": float64(0), "request_size": float64(r.Len()),
				"dnssec_ok": true, "bufsize": float64(dns.MaxMsgSize), "rcode": "REFUSED",
				"msg": `{"name":"` + state.Name() + `","metadata":"` + value + `"}`,
			}
			for field, value := range want {
				if record[field] != value {
					t.Errorf("%s = %v, want %v", field, record[field], value)
				}
			}
			if d, ok := record["duration_seconds"].(float64); !ok || d < 0 {
				t.Errorf("invalid duration: %v", record["duration_seconds"])
			}
			response := new(dns.Msg)
			response.SetRcode(r, dns.RcodeRefused)
			if record["response_size"] != float64(response.Len()) {
				t.Errorf("unexpected deferred response size: %v", record)
			}
		})
	}
}

func TestJSONQueryResponses(t *testing.T) {
	writeErr := errors.New("test write failure")
	for _, tc := range []struct {
		name   string
		rcode  int
		write  bool
		nodata bool
		fail   bool
		class  response.Class
		logged bool
	}{
		{"success", dns.RcodeSuccess, true, false, false, response.Success, true},
		{"nxdomain", dns.RcodeNameError, true, false, false, response.Denial, true},
		{"nodata", dns.RcodeSuccess, true, true, false, response.Denial, true},
		{"servfail", dns.RcodeServerFailure, false, false, false, response.Error, true},
		{"refused", dns.RcodeRefused, false, false, false, response.Error, true},
		{"drop", dns.RcodeSuccess, false, false, false, response.All, true},
		{"write-error", dns.RcodeSuccess, true, false, true, response.All, true},
		{"unknown-rcode", 4095, true, false, false, response.All, true},
		{"denial-filtered", dns.RcodeNameError, true, false, false, response.Success, false},
		{"error-filtered", dns.RcodeServerFailure, false, false, false, response.Denial, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			jsonQueryOutput(t, "json", &out)
			r := new(dns.Msg)
			r.SetQuestion("example.org.", dns.TypeA)
			logger := Logger{
				Rules: []Rule{{NameScope: "example.org.", Class: map[response.Class]struct{}{tc.class: {}}, Format: DefaultLogFormat}},
				Next: plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
					if !tc.write {
						return tc.rcode, nil
					}
					m := new(dns.Msg)
					m.SetRcode(r, tc.rcode)
					if tc.nodata {
						m.Ns = []dns.RR{test.SOA("example.org. 60 IN SOA ns.example.org. hostmaster.example.org. 1 2 3 4 5")}
					}
					return tc.rcode, w.WriteMsg(m)
				}),
			}
			w := &jsonFailWriter{err: nil}
			if tc.fail {
				w.err = writeErr
			}
			rc, err := logger.ServeDNS(t.Context(), w, r)
			if rc != tc.rcode || !errors.Is(err, w.err) {
				t.Fatalf("return changed: rc=%d err=%v", rc, err)
			}
			if !tc.logged {
				if out.Len() != 0 {
					t.Fatalf("class filter ignored: %s", &out)
				}
				return
			}
			var record map[string]any
			if err := json.Unmarshal(out.Bytes(), &record); err != nil {
				t.Fatal(err)
			}
			if tc.name == "drop" {
				if rc, exists := record["rcode"]; !exists || rc != nil || record["response_size"] != float64(0) {
					t.Errorf("missing response misrepresented: %v", record)
				}
			} else if tc.name == "unknown-rcode" {
				if record["rcode"] != "4095" {
					t.Errorf("unknown rcode lost: %v", record)
				}
			} else if record["rcode"] != dns.RcodeToString[tc.rcode] {
				t.Errorf("wrong rcode: %v", record)
			}
			out.Reset()
			r.SetQuestion("outside.example.net.", dns.TypeA)
			logger.ServeDNS(t.Context(), w, r)
			if out.Len() != 0 {
				t.Fatalf("name filter ignored: %s", &out)
			}
		})
	}
}

type jsonFailWriter struct {
	test.ResponseWriter
	err error
}

func (w *jsonFailWriter) WriteMsg(_ *dns.Msg) error { return w.err }

func BenchmarkQueryLogFormat(b *testing.B) {
	for _, format := range []string{"text", "json"} {
		b.Run(format, func(b *testing.B) {
			jsonQueryOutput(b, format, io.Discard)
			logger := Logger{
				Rules: []Rule{{NameScope: ".", Class: map[response.Class]struct{}{response.All: {}}, Format: DefaultLogFormat}},
				Next:  test.NextHandler(dns.RcodeRefused, nil), repl: replacer.New(),
			}
			r := new(dns.Msg)
			r.SetQuestion("example.org.", dns.TypeA)
			w := &test.ResponseWriter{}
			b.ReportAllocs()
			for b.Loop() {
				logger.ServeDNS(b.Context(), w, r)
			}
		})
	}
}
