package expression

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/coredns/coredns/plugin/metadata"
	"github.com/coredns/coredns/request"
)

// StateExtractor extracts information from the state and metadata from context for expressions
type StateExtractor struct {
	ctx        context.Context
	state      *request.Request
	extractors ExtractorMap
}

// NewStateExtractor creates a new state extractor with given state and context and default extractors
func NewStateExtractor(ctx context.Context, state request.Request, extractors ExtractorMap) StateExtractor {
	return StateExtractor{ctx: ctx, state: &state, extractors: extractors}
}

type ExtractorMap map[string]func(state *request.Request) string

// DefaultExtractors returns the default set of variable that can be used in expressions, and how to extract
// them from state.
func DefaultExtractors() ExtractorMap {
	return ExtractorMap{
		"type": func(state *request.Request) string {
			return state.Type()
		},
		"name": func(state *request.Request) string {
			return state.Name()
		},
		"class": func(state *request.Request) string {
			return state.Class()
		},
		"proto": func(state *request.Request) string {
			return state.Proto()
		},
		"size": func(state *request.Request) string {
			return strconv.Itoa(state.Len())
		},
		"client_ip": func(state *request.Request) string {
			return addrToRFC3986(state.IP())
		},
		"port": func(state *request.Request) string {
			return addrToRFC3986(state.Port())
		},
		"id": func(state *request.Request) string {
			return strconv.Itoa(int(state.Req.Id))
		},
		"opcode": func(state *request.Request) string {
			return strconv.Itoa(int(state.Req.Opcode))
		},
		"do": func(state *request.Request) string {
			return boolToString(state.Do())
		},
		"bufsize": func(state *request.Request) string {
			return strconv.Itoa(state.Size())
		},
		"server_ip": func(state *request.Request) string {
			return addrToRFC3986(state.LocalIP())
		},
		"server_port": func(state *request.Request) string {
			return addrToRFC3986(state.LocalPort())
		},
	}
}

func (p StateExtractor) Get(s string) (interface{}, error) {
	if v, ok := p.Value(s); ok {
		return v, nil
	}
	f := metadata.ValueFunc(p.ctx, s)
	if f == nil {
		return nil, fmt.Errorf("unknown variable '%v'", s)
	}
	return f(), nil
}

func (p StateExtractor) Value(s string) (string, bool) {
	fn := p.extractors[s]
	if fn == nil {
		return "", false
	}
	return fn(p.state), true
}

func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// addrToRFC3986 will add brackets to the address if it is an IPv6 address.
func addrToRFC3986(addr string) string {
	if strings.Contains(addr, ":") {
		return "[" + addr + "]"
	}
	return addr
}
