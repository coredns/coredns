package log

import (
	"log/slog"
	"strconv"
	"time"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

func logJSON(msg string, state request.Request, rr *dnstest.Recorder) {
	// A missing response is not NOERROR (for example an ACL drop). Deferred
	// errors have already been synthesized by ServeDNS, just as in text mode.
	rcode := slog.Any("rcode", nil)
	if rr.Msg != nil {
		rc := dns.RcodeToString[rr.Rcode]
		if rc == "" {
			rc = strconv.Itoa(rr.Rcode)
		}
		rcode = slog.String("rcode", rc)
	}
	port, _ := strconv.Atoi(state.Port())
	clog.InfoAttrs(msg,
		slog.String("plugin", "log"),
		slog.String("client_ip", state.IP()),
		slog.Int("client_port", port),
		slog.String("qname", state.Name()),
		slog.String("qtype", state.Type()),
		slog.String("qclass", state.Class()),
		slog.String("protocol", state.Proto()),
		slog.Int("id", int(state.Req.Id)),
		slog.Int("opcode", state.Req.Opcode),
		slog.Int("request_size", state.Len()),
		slog.Bool("dnssec_ok", state.Do()),
		slog.Int("bufsize", state.Size()),
		rcode,
		slog.Int("response_size", rr.Len),
		slog.Float64("duration_seconds", time.Since(rr.Start).Seconds()),
	)
}
