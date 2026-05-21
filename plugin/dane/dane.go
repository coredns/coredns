package dane

import (
	"context"
	"errors"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/response"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// Dane is a plugin that automatically generates TLSA records from certificate files
type Dane struct {
	Next         plugin.Handler
	Zones        []string
	Certificates map[string][]string
}

var (
	ErrInvalidUpstreamResponse = errors.New("invalid upstream response")
	ErrUnsupportedMatchingType = errors.New("unsupported matching type")
)

// ResponseWriter replaces "dummy" TLSA responses with real certificate contents
type ResponseWriter struct {
	dns.ResponseWriter
	d *Dane
}

func certIndex(usage, selector, matching uint8) (uint, error) {
	if usage > 3 || selector > 1 || matching > 2 {
		return 0, ErrInvalidUpstreamResponse
	}
	if matching == 0 {
		return 0, ErrUnsupportedMatchingType
	}
	if usage > 1 {
		usage -= 2
	}
	return uint(usage) | (uint(selector) << 1) | (uint(matching-1) << 2), nil
}

// WriteMsg implements the dns.ResponseWriter interface.
func (d *ResponseWriter) WriteMsg(res *dns.Msg) error {
	mt, _ := response.Typify(res, time.Now().UTC())
	if mt != response.NoError {
		return d.ResponseWriter.WriteMsg(res)
	}

	var resCopy *dns.Msg
	for i, rr := range res.Answer {
		if rr.Header().Rrtype != dns.TypeTLSA {
			continue
		}
		tlsaRr := rr.(*dns.TLSA)
		replacements, ok := d.d.Certificates[tlsaRr.Certificate]
		if !ok {
			continue
		}
		idx, err := certIndex(tlsaRr.Usage, tlsaRr.Selector, tlsaRr.MatchingType)
		if err != nil {
			return err
		}
		if resCopy == nil {
			resCopy = res.Copy()
		}
		tgtRr := resCopy.Answer[i].(*dns.TLSA)
		tgtRr.Certificate = replacements[idx]
	}
	if resCopy != nil {
		return d.ResponseWriter.WriteMsg(resCopy)
	}
	return d.ResponseWriter.WriteMsg(res)
}

// Write implements the dns.ResponseWriter interface.
func (d *ResponseWriter) Write(buf []byte) (int, error) {
	log.Warning("Dane called with Write: not rewriting reply")
	n, err := d.ResponseWriter.Write(buf)
	return n, err
}

// ServeDNS implements the plugin.Handler interface.
func (d *Dane) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}

	zone := plugin.Zones(d.Zones).Matches(state.Name())
	if zone == "" {
		return plugin.NextOrFailure(d.Name(), d.Next, ctx, w, r)
	}

	drr := &ResponseWriter{w, d}
	return plugin.NextOrFailure(d.Name(), d.Next, ctx, drr, r)
}

// Name implements the Handler interface.
func (d *Dane) Name() string { return "dane" }
