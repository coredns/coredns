package metadata_edns0

import (
	"context"
	"encoding/hex"
	"net"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metadata"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

const (
	typeEDNS0Bytes = iota
	typeEDNS0Hex
	typeEDNS0IP
)

var stringToEDNS0MapType = map[string]uint16{
	"bytes":   typeEDNS0Bytes,
	"hex":     typeEDNS0Hex,
	"address": typeEDNS0IP,
}

type edns0Map struct {
	name     string
	code     uint16
	dataType uint16
	size     uint
	start    uint
	end      uint
}

// metadataEdns0 represents a plugin instance that can validate DNS
// requests and replies using PDP server.
type metadataEdns0 struct {
	Next    plugin.Handler
	options map[uint16][]*edns0Map
}

func New() *metadataEdns0 {
	pol := &metadataEdns0{options: make(map[uint16][]*edns0Map, 0)}
	return pol
}

// ServeDNS implements the Handler interface.
func (rq metadataEdns0) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	return plugin.NextOrFailure(rq.Name(), rq.Next, ctx, w, r)

}

// Name implements the Handler interface.
func (m metadataEdns0) Name() string { return "metadata_edns0" }

func (p *metadataEdns0) Metadata(ctx context.Context, state request.Request) context.Context {
	return p.registerFuncExtractEDNS0(state.Req, ctx)
}

func (p *metadataEdns0) registerFuncExtractEDNS0(r *dns.Msg, ctx context.Context) context.Context {
	o := r.IsEdns0()
	if o == nil {
		return nil
	}

	for _, opt := range p.options {
		for _, d := range opt {
			metadata.SetValueFunc(ctx, "metadata_edns0/"+d.name, func() string {
				return p.extractEDNS0(o, d)
			})
		}
	}

	return ctx
}

// following function will be called ONLY when a plugin requires the value.
func (p *metadataEdns0) extractEDNS0(opt *dns.OPT, params *edns0Map) string {

	// loop over all OPT records of the dns.Msg until find one that match params.Code
	for _, opt := range opt.Option {
		optLocal, local := opt.(*dns.EDNS0_LOCAL)
		if !local {
			continue
		}
		if optLocal.Code != params.code {
			continue
		}
		// now extract the right information based on params information and return the corresponding String
		var value string
		var data = optLocal.Data
		switch params.dataType {
		case typeEDNS0Bytes:
			value = string(data)
		case typeEDNS0Hex:
			value = parseHex(data, params)
		case typeEDNS0IP:
			ip := net.IP(data)
			value = ip.String()
		}
		return value
	}
	return ""
}

func parseHex(data []byte, option *edns0Map) string {
	size := uint(len(data))
	// if option.size == 0 - don't check size
	if option.size > 0 {
		if size != option.size {
			// skip parsing option with wrong size
			return ""
		}
	}
	start := uint(0)
	if option.start < size {
		// set start index
		start = option.start
	} else {
		// skip parsing option if start >= data size
		return ""
	}
	end := size
	// if option.end == 0 - return data[start:]
	if option.end > 0 {
		if option.end <= size {
			// set end index
			end = option.end
		} else {
			// skip parsing option if end > data size
			return ""
		}
	}
	return hex.EncodeToString(data[start:end])
}
