package expression

import (
	"context"
	"errors"
	"net"
	"strings"

	"github.com/coredns/coredns/plugin/metadata"
	"github.com/coredns/coredns/request"
)

// DefaultEnv returns the default set of custom state variables and functions available to for use in expression evaluation.
func DefaultEnv(ctx context.Context, state *request.Request) map[string]interface{} {
	return map[string]interface{}{
		"incidr": func(ipStr, cidrStr string) (bool, error) {
			ip := net.ParseIP(ipStr)
			if ip == nil {
				return false, errors.New("first argument is not an IP address")
			}
			_, cidr, err := net.ParseCIDR(cidrStr)
			if err != nil {
				return false, err
			}
			return cidr.Contains(ip), nil
		},
		"metadata": func(label string) string {
			f := metadata.ValueFunc(ctx, label)
			if f == nil {
				return ""
			}
			return f()
		},
		"type":        state.Type,
		"name":        state.Name,
		"class":       state.Class,
		"proto":       state.Proto,
		"size":        state.Len,
		"client_ip":   func() string { return addrToRFC3986(state.IP()) },
		"port":        func() string { return addrToRFC3986(state.Port()) },
		"id":          func() int { return int(state.Req.Id) },
		"opcode":      func() int { return state.Req.Opcode },
		"do":          state.Do,
		"bufsize":     state.Size,
		"server_ip":   func() string { return addrToRFC3986(state.LocalIP()) },
		"server_port": func() string { return addrToRFC3986(state.LocalPort()) },
	}
}

// addrToRFC3986 will add brackets to the address if it is an IPv6 address.
func addrToRFC3986(addr string) string {
	if strings.Contains(addr, ":") {
		return "[" + addr + "]"
	}
	return addr
}
