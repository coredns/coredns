package whitelist

import (
	"context"
	"errors"
	"fmt"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/kubernetes"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
	"net"
	"strings"
)

var log = clog.NewWithPlugin("whitelist")

type Whitelist struct {
	Kubernetes          *kubernetes.Kubernetes
	Next                plugin.Handler
	ServicesToWhitelist map[string]map[string]struct{}
}

func (whitelist Whitelist) ServeDNS(ctx context.Context, rw dns.ResponseWriter, r *dns.Msg) (int, error) {

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative, m.RecursionAvailable = true, true
	remoteAddr := rw.RemoteAddr()

	state := request.Request{W: rw, Req: r, Context: ctx}

	var ipAddr string
	if ip, ok := remoteAddr.(*net.UDPAddr); ok {
		ipAddr = ip.IP.String()
	}

	if ipAddr == "" {
		return plugin.NextOrFailure(whitelist.Name(), whitelist.Next, ctx, rw, r)
	}

	segs := dns.SplitDomainName(state.Name())

	if len(segs) <= 1 {
		code, err := plugin.NextOrFailure(whitelist.Name(), whitelist.Next, ctx, rw, r)
		fmt.Printf("plugin result %d, %s", code, err)
	}

	if ns, _ := whitelist.Kubernetes.APIConn.GetNamespaceByName(segs[1]); ns != nil {
		return plugin.NextOrFailure(whitelist.Name(), whitelist.Next, ctx, rw, r)
	}

	service := whitelist.getServiceFromIP(ipAddr)

	if service == "" {
		log.Infof("no service found for ip %s", ipAddr)
		return plugin.NextOrFailure(whitelist.Name(), whitelist.Next, ctx, rw, r)
	}

	query := state.Name()
	if whitelisted, ok := whitelist.ServicesToWhitelist[service]; ok {
		if _, ok := whitelisted[query]; ok {
			return plugin.NextOrFailure(whitelist.Name(), whitelist.Next, ctx, rw, r)
		}

	}

	m.SetRcode(r, dns.RcodeNameError)
	rw.WriteMsg(m)
	return dns.RcodeNameError, errors.New("not whitelisted")

}

func (whitelist Whitelist) getServiceFromIP(ipAddr string) string {

	services := whitelist.Kubernetes.APIConn.ServiceList()
	if services == nil || len(services) == 0 {
		return ""
	}

	pods := whitelist.Kubernetes.APIConn.PodIndex(ipAddr)
	if pods == nil || len(pods) == 0 {
		return ""
	}

	pod := pods[0]

	var service string
	for _, svc := range services {
		for pLabelKey, pLabelValue := range pod.Labels {
			if svcLabelValue, ok := svc.Labels[pLabelKey]; ok {
				if strings.EqualFold(pLabelValue, svcLabelValue) {
					service = svc.Name
				}
			}
		}
	}
	return service
}

func (whitelist Whitelist) Name() string {
	return "whitelist"
}
