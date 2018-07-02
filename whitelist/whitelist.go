package whitelist

import (
	"github.com/miekg/dns"
	"errors"
	"net"
	"github.com/coredns/coredns/plugin"
	"context"
	"github.com/coredns/coredns/plugin/kubernetes"
	"github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"
)

type Whitelist struct {
	kubernetes.Kubernetes
	Next            plugin.Handler
	ServicesToHosts map[string]string
}

func (whitelist Whitelist) ServeDNS(ctx context.Context, rw dns.ResponseWriter, r *dns.Msg) (int, error) {

	state := request.Request{W: rw, Req: r, Context: ctx}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative, m.RecursionAvailable = true, true

	zone := plugin.Zones(whitelist.Kubernetes.Zones).Matches(state.Name())
	if zone == "" {
		return plugin.NextOrFailure(whitelist.Kubernetes.Name(), whitelist.Kubernetes.Next, ctx, rw, r)
	}

	remoteAddr := rw.RemoteAddr()
	service := toServiceName(remoteAddr)
	log.Infof("handling service %s", service)

	if wlRecord, ok := whitelist.ServicesToHosts[service]; ok {
		log.Infof("query %s", r.Question[0].Name)
		if r.Question[0].Name == wlRecord {
			log.Infof("%s whitelisted for service %s", r.Question[0].Name, wlRecord)
			return plugin.NextOrFailure(whitelist.Name(), whitelist.Next, ctx, rw, r)
		}
	}

	m.SetRcode(r, dns.RcodeNameError)
	// does not matter if this write fails
	rw.WriteMsg(m)


	return whitelist.Kubernetes.ServeDNS(ctx, rw, r)

	return dns.RcodeNameError, errors.New("not whitelisted")

}

func (whitelist Whitelist) Name() string {
	return "whitelist"
}

func toServiceName(addr net.Addr) string {
	return "test"
}


