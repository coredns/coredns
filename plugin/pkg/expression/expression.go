package expression

import (
	"context"
	"errors"
	"net"

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
		"client_ip":   state.IP,
		"port":        state.Port,
		"id":          func() int { return int(state.Req.Id) },
		"opcode":      func() int { return state.Req.Opcode },
		"do":          state.Do,
		"bufsize":     state.Size,
		"server_ip":   state.LocalIP,
		"server_port": state.LocalPort,
	}
}
