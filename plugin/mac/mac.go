package mac

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
	"github.com/mostlygeek/arp"
)

type Mac struct {
	Next plugin.Handler
}

func (m *Mac) Name() string { return "mac" }

// ServeDNS implements the plugin.Handler interface.
func (m *Mac) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	return plugin.NextOrFailure(m.Name(), m.Next, ctx, w, r)
}

// ServeDNS implements the metadata.Metadater interface.
func (m *Mac) Metadata(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (context.Context, error) {
	// TODO: Add validations
	mac, err := m.fetchMAC(&request.Request{W: w, Req: r})
	_ = err
	return context.WithValue(ctx, "mac", mac), nil
}

func (m *Mac) fetchMAC(req *request.Request) ([]byte, error) {
	clientIP := req.IP()
	macAddr := arp.Search(clientIP)

	// this is required to pad the MAC address so that it doesn't error
	// when parsing MAC.
	if macAddr != "" {
		mac := strings.SplitN(macAddr, ":", 6)
		for i, m := range mac {
			if len(m) == 1 {
				mac[i] = "0" + m
			}
		}

		macAddr = strings.Join(mac, ":")
		macAddress, err := net.ParseMAC(macAddr)
		if err != nil {
			return []byte(""), err
		}
		fmt.Println(macAddr)
		return macAddress, nil
	}
	return []byte(""), nil
}
