package tsig

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// TSIGServer verifies tsig status and adds tsig to responses
type TSIGServer struct {
	Zones      []string
	secrets    map[string]string // [key-name]secret
	types      qTypes
	opcodes    opCodes
	allTypes   bool
	allOpcodes bool
	Next       plugin.Handler
}

type qTypes map[uint16]struct{}
type opCodes map[int]struct{}

// Name implements plugin.Handler
func (t TSIGServer) Name() string { return pluginName }

// ServeDNS implements plugin.Handler
func (t *TSIGServer) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	var (
		state  = request.Request{Req: r, W: w}
		tsigRR = r.IsTsig()
	)
	switch {
	case tsigRR == nil && !t.tsigRequired(state.QType(), r.Opcode):
		fallthrough
	case plugin.Zones(t.Zones).Matches(state.Name()) == "":
		return plugin.NextOrFailure(t.Name(), t.Next, ctx, w, r)
	case tsigRR == nil:
		log.Debugf("rejecting '%s' request without TSIG\n", dns.TypeToString[state.QType()])
		resp := new(dns.Msg).SetRcode(r, dns.RcodeRefused)
		w.WriteMsg(resp)
		return dns.RcodeSuccess, nil
	}

	// Strip the TSIG RR. Next, and subsequent plugins will not see the TSIG RRs.
	// This violates forwarding cases (RFC 8945 5.5). See README.md Bugs
	r.Extra = r.Extra[:len(r.Extra)-1]

	// Wrap the response writer so the response will be TSIG signed.
	w = &restoreTsigWriter{w, r, tsigRR}

	if tsigStatus := w.TsigStatus(); tsigStatus != nil {
		log.Debugf("TSIG validation failed: %v %v", dns.TypeToString[state.QType()], tsigStatus)
		switch tsigStatus {
		case dns.ErrSecret:
			tsigRR.Error = dns.RcodeBadKey
		case dns.ErrTime:
			tsigRR.Error = dns.RcodeBadTime
		default:
			tsigRR.Error = dns.RcodeBadSig
		}
		resp := new(dns.Msg).SetRcode(r, dns.RcodeNotAuth)
		w.WriteMsg(resp)
		return dns.RcodeSuccess, nil
	}

	tsigRR.Error = dns.RcodeSuccess
	rcode, err := plugin.NextOrFailure(t.Name(), t.Next, ctx, w, r)
	if err != nil {
		log.Errorf("request handler returned an error: %v\n", err)
	}
	// If the downstream plugin chain did not write, use custom ResponseWriter here
	// because [dnsserver.errorFunc] ignores TSIG.
	if !plugin.ClientWrite(rcode) {
		resp := new(dns.Msg).SetRcode(r, rcode)
		w.WriteMsg(resp)
	}
	return dns.RcodeSuccess, nil
}

func (t *TSIGServer) tsigRequired(qtype uint16, opcode int) bool {
	typeMatches := t.allTypes
	if !typeMatches {
		_, typeMatches = t.types[qtype]
	}

	opcodeMatches := t.allOpcodes
	if !opcodeMatches {
		_, opcodeMatches = t.opcodes[opcode]
	}

	return typeMatches || opcodeMatches
}

// restoreTsigWriter implements [dns.ResponseWriter], and adds a [dns.TSIG] RR to a response.
type restoreTsigWriter struct {
	dns.ResponseWriter
	req     *dns.Msg  // original request excluding TSIG
	reqTSIG *dns.TSIG // original TSIG
}

// WriteMsg adds a TSIG RR to the response
func (r *restoreTsigWriter) WriteMsg(m *dns.Msg) error {
	if repTSIG := m.IsTsig(); repTSIG == nil { // respect TSIG set downstream
		// Make sure the response has an EDNS OPT RR if the request had it.
		// Otherwise [request.ScrubWriter] would append it *after* TSIG, making it a non-compliant DNS message.
		state := request.Request{Req: r.req, W: r.ResponseWriter}
		state.SizeAndDo(m)

		repTSIG = new(dns.TSIG)
		repTSIG.Hdr = dns.RR_Header{Name: r.reqTSIG.Hdr.Name, Rrtype: dns.TypeTSIG, Class: dns.ClassANY}
		repTSIG.Algorithm = r.reqTSIG.Algorithm
		repTSIG.OrigId = m.Id
		repTSIG.Error = r.reqTSIG.Error
		repTSIG.MAC = r.reqTSIG.MAC
		repTSIG.MACSize = r.reqTSIG.MACSize
		if repTSIG.Error == dns.RcodeBadTime {
			// per RFC 8945 5.2.3. client time goes into TimeSigned, server time in OtherData, OtherLen = 6 ...
			repTSIG.TimeSigned = r.reqTSIG.TimeSigned
			b := make([]byte, 8)
			// TimeSigned is network byte order.
			binary.BigEndian.PutUint64(b, uint64(time.Now().Unix())) // #nosec G115 -- Unix time fits in uint64
			// truncate to 48 least significant bits (network order 6 rightmost bytes)
			repTSIG.OtherData = hex.EncodeToString(b[2:])
			repTSIG.OtherLen = 6
		}
		m.Extra = append(m.Extra, repTSIG)
	}
	return r.ResponseWriter.WriteMsg(m)
}

const pluginName = "tsig"
