package view

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/coredns/coredns/plugin/metadata"
	"github.com/coredns/coredns/request"
)

type Parameters struct {
	ctx        context.Context
	state      *request.Request
	extractors extractorMap
}

type extractorMap map[string]func(state *request.Request) string

func makeExtractors() extractorMap {
	return extractorMap{
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

func (p Parameters) Get(s string) (interface{}, error) {
	if v, ok := p.Value(s); ok {
		return v, nil
	}
	f := metadata.ValueFunc(p.ctx, s)
	if f == nil {
		return nil, fmt.Errorf("unknown variable '%v'", s)
	}
	return f(), nil
}

func (p Parameters) Value(s string) (string, bool) {
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
