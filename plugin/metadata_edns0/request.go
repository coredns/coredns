package metadata_edns0

import (
	"github.com/miekg/dns"

	"context"

	"github.com/coredns/coredns/plugin"
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

func newRequestPlugin() *metadataEdns0 {
	pol := &metadataEdns0{options: make(map[uint16][]*edns0Map, 0)}
	return pol
}

// ServeDNS implements the Handler interface.
func (rq metadataEdns0) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	return plugin.NextOrFailure(rq.Name(), rq.Next, ctx, w, r)

}

// Name implements the Handler interface.
func (m metadataEdns0) Name() string { return "metadata_edns0" }
