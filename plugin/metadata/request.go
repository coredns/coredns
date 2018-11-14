package metadata

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"

	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

const (
	typeEDNS0Bytes = iota
	typeEDNS0Hex
	typeEDNS0IP
)

const (
	queryName  = "qname"
	queryType  = "qtype"
	clientIP   = "client_ip"
	clientPort = "client_port"
	protocol   = "protocol"
	serverIP   = "server_ip"
	serverPort = "server_port"
	responseIP = "response_ip"
)

func newMetadata() *Metadata {
	pol := &Metadata{options: make(map[uint16][]*edns0Map, 0)}
	return pol
}

type edns0Map struct {
	name     string
	code     uint16
	dataType uint16
	size     uint
	start    uint
	end      uint
}

var stringToEDNS0MapType = map[string]uint16{
	"bytes":   typeEDNS0Bytes,
	"hex":     typeEDNS0Hex,
	"address": typeEDNS0IP,
}

func newEDNS0Map(code, name, dataType, sizeStr, startStr, endStr string) (*edns0Map, error) {
	c, err := strconv.ParseUint(code, 0, 16)
	if err != nil {
		return nil, fmt.Errorf("could not parse EDNS0 code: %s", err)
	}
	size, err := strconv.ParseUint(sizeStr, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("could not parse EDNS0 data size: %s", err)
	}
	start, err := strconv.ParseUint(startStr, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("could not parse EDNS0 start index: %s", err)
	}
	end, err := strconv.ParseUint(endStr, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("could not parse EDNS0 end index: %s", err)
	}
	if end <= start && end != 0 {
		return nil, fmt.Errorf("end index should be > start index (actual %d <= %d)", end, start)
	}
	if end > size && size != 0 {
		return nil, fmt.Errorf("end index should be <= size (actual %d > %d)", end, size)
	}
	ednsType, ok := stringToEDNS0MapType[dataType]
	if !ok {
		return nil, fmt.Errorf("invalid dataType for EDNS0 map: %s", dataType)
	}
	ecode := uint16(c)
	return &edns0Map{name, ecode, ednsType, uint(size), uint(start), uint(end)}, nil
}

func (m *Metadata) addEDNS0Map(code, name, dataType, sizeStr, startStr, endStr string) error {
	meta, err := newEDNS0Map(code, name, dataType, sizeStr, startStr, endStr)
	if err != nil {
		return err
	}
	m.options[meta.code] = append(m.options[meta.code], meta)
	return nil
}

// Metadata adds additional standard values to the context.
func (m *Metadata) Metadata(ctx context.Context, state request.Request) context.Context {
	return m.fillMetadata(ctx, state)
}

func (m *Metadata) fillMetadata(ctx context.Context, state request.Request) context.Context {

	m.declareMetadata(ctx, queryName, state.QName())
	m.declareMetadata(ctx, queryType, dns.Type(state.QType()).String())
	m.declareMetadata(ctx, clientIP, state.IP())
	m.declareMetadata(ctx, serverIP, state.LocalIP())
	m.declareMetadata(ctx, clientPort, state.Port())
	m.declareMetadata(ctx, serverPort, state.LocalPort())
	m.declareMetadata(ctx, protocol, state.Proto())

	m.getAttrsFromEDNS0(ctx, state.Req)

	SetValueFunc(ctx, "metadata/"+responseIP, func() string {
		ip := getRespIP(state.Req)
		if ip != nil {
			return ip.String()
		}
		return ""
	})
	return ctx

}

func (m *Metadata) declareMetadata(ctx context.Context, name string, value string) {
	SetValueFunc(ctx, "metadata/"+name, func() string { return value })
}

func (m *Metadata) getAttrsFromEDNS0(ctx context.Context, r *dns.Msg) {
	o := r.IsEdns0()
	if o == nil {
		return
	}

	for _, opt := range o.Option {
		optLocal, local := opt.(*dns.EDNS0_LOCAL)
		if !local {
			continue
		}
		opts, ok := m.options[optLocal.Code]
		if !ok {
			continue
		}
		m.parseOptionGroup(ctx, optLocal.Data, opts)
	}
}

func (m *Metadata) parseOptionGroup(ctx context.Context, data []byte, options []*edns0Map) {
	for _, option := range options {
		var value string
		switch option.dataType {
		case typeEDNS0Bytes:
			value = string(data)
		case typeEDNS0Hex:
			value = parseHex(data, option)
		case typeEDNS0IP:
			ip := net.IP(data)
			value = ip.String()
		}
		if value != "" {
			m.declareMetadata(ctx, option.name, value)
		}
	}
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

func getRespIP(r *dns.Msg) net.IP {
	if r == nil {
		return nil
	}

	var ip net.IP
	for _, rr := range r.Answer {
		switch rr := rr.(type) {
		case *dns.A:
			ip = rr.A

		case *dns.AAAA:
			ip = rr.AAAA
		}
		// If there are several responses, currently
		// only return the first one and break.
		if ip != nil {
			break
		}
	}
	return ip
}
