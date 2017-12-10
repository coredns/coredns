package allow

import (
    "net"

    "github.com/coredns/coredns/plugin"
    "github.com/coredns/coredns/request"

    "github.com/miekg/dns"
    "golang.org/x/net/context"
)

type Allow struct {
    Next     plugin.Handler
    Cidrs  []string
}

func (rw Allow) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
    state := request.Request{W: w, Req: r}
    ip := state.IP()

    for _, it := range rw.Cidrs {
        _, cidr, err := net.ParseCIDR(it)
        if err != nil {
            return 0, err
        }
        if cidr.Contains(net.ParseIP(ip)) {
            return plugin.NextOrFailure(rw.Name(), rw.Next, ctx, w, r)
        }
    }

    m := new(dns.Msg)
    m.SetRcode(r, dns.RcodeRefused)
    state.SizeAndDo(m)
    w.WriteMsg(m)

    return 0, nil
}

func (rw Allow) Name() string { return "allow" }
