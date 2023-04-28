package sql

import (
	"context"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metrics"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// SQL database plugin
type SQL struct {
	Next plugin.Handler

	cfg            *Config
	zones          []string
	lastZoneUpdate time.Time
}

// ServeDNS implements the plugin.Handler interface. This method gets called when sql plugin is used
// in a Server.
//
// A records cause no additional section processing.
//
// MX records cause type A additional section processing for the host specified by EXCHANGE.
//
// CNAME RRs cause no additional section processing, but name servers may
// choose to restart the query at the canonical name in certain cases.  See
// the description of name server logic in [RFC-1034](https://datatracker.ietf.org/doc/html/rfc1034) for details.
//
// NS records cause both the usual additional section processing to locate
// a type A record, and, when used in a referral, a special search of the
// zone in which they reside for glue information.
//
// PTR records cause no additional section processing.
func (s *SQL) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	responseMsg := new(dns.Msg)
	responseMsg.SetReply(r)
	responseMsg.Authoritative = true // TODO(jproxx): authoritative only, if question does not contain a "*"
	responseMsg.RecursionAvailable = false
	responseMsg.Compress = true

	state := request.Request{W: w, Req: r}

	reqName := state.Name()
	reqType := state.QType()

	// Export metric with the server label set to the current server handling the request.
	requestCount.WithLabelValues(metrics.WithServer(ctx)).Inc()

	client, err := s.getSqlClient()
	if err != nil {
		return s.errorResponse(state, dns.RcodeServerFailure, err)
	}
	defer client.Close()

	if s.mustReloadZones() {
		err := s.loadZones(ctx, client)
		if err != nil {
			return s.errorResponse(state, dns.RcodeServerFailure, err)
		}
	}

	reqZone := plugin.Zones(s.zones).Matches(reqName)
	if reqZone == "" {
		return plugin.NextOrFailure(s.Name(), s.Next, ctx, w, r)
	}

	// TODO(jproxx): implement axfr requests, because we are needing them
	if reqType == dns.TypeAXFR {
		return s.errorResponse(state, dns.RcodeNotImplemented, nil)
	}

	// handle soa record
	var soaRec []dns.RR
	if reqType == dns.TypeSOA {
		// SOA records cause no additional section processing.
		soaRec, err = s.getSOARecord(ctx, client, reqZone)
		if err != nil {
			return s.errorResponse(state, dns.RcodeServerFailure, err)
		}

		return s.write(state, w, responseMsg, soaRec)
	}

	rrs, err := s.getRRecords(ctx, client, reqName, reqType)
	if err != nil {
		return s.errorResponse(state, dns.RcodeServerFailure, err)
	}

	if len(rrs) == 0 {
		soaRec, err = s.getSOARecord(ctx, client, reqZone)
		if err != nil {
			return s.errorResponse(state, dns.RcodeServerFailure, err)
		}

		return s.write(state, w, responseMsg, soaRec)
	}

	// build authoritative section
	// TODO(jproxx): check, when we have to provide a authoritative section + additional section!
	// TODO(jproxx): this behavior is not correct in all cases, so we removed it
	// if nsRRs, err := a.getNameServers(ctx, client, reqZone); err != nil {
	// 	return a.errorResponse(state, dns.RcodeServerFailure, err)
	// } else {
	// 	responseMsg.Ns = nsRRs
	// }

	return s.write(state, w, responseMsg, rrs)
}

// Name implements the Handler interface.
func (s *SQL) Name() string { return plgName }

// mustReloadZones "trigger" to load zones from database
func (s *SQL) mustReloadZones() bool {
	return time.Since(s.lastZoneUpdate) > s.cfg.zoneUpdateTime
}

// write response message via dns.ResponseWriter
func (s *SQL) write(state request.Request, w dns.ResponseWriter, msg *dns.Msg, answerSection []dns.RR) (int, error) {
	msg.Answer = append(msg.Answer, answerSection...)
	state.SizeAndDo(msg)
	msg = state.Scrub(msg)
	if err := w.WriteMsg(msg); err != nil {
		return dns.RcodeServerFailure, err
	}

	return dns.RcodeSuccess, nil
}

// errorResponse is used to send the error response
func (s *SQL) errorResponse(state request.Request, rCode int, err error) (int, error) {
	m := new(dns.Msg)
	m.SetRcode(state.Req, rCode)
	m.Authoritative, m.RecursionAvailable, m.Compress = true, false, true

	state.SizeAndDo(m)
	if writeErr := state.W.WriteMsg(m); writeErr != nil {
		return dns.RcodeServerFailure, err
	}
	return dns.RcodeSuccess, err
}
