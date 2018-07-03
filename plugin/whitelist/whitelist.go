package whitelist

import (
	"context"
	"errors"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/kubernetes"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/miekg/dns"
	"net"
	"strings"
)

var log = clog.NewWithPlugin("whitelist")

type Whitelist struct {
	Kubernetes      *kubernetes.Kubernetes
	Next            plugin.Handler
	ServicesToHosts map[string]string
}

func (whitelist Whitelist) ServeDNS(ctx context.Context, rw dns.ResponseWriter, r *dns.Msg) (int, error) {

	log.Infof("query %s", r.Question[0].Name)

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative, m.RecursionAvailable = true, true
	remoteAddr := rw.RemoteAddr()

	var ipAddr string
	if ip, ok := remoteAddr.(*net.UDPAddr); ok {
		ipAddr = ip.IP.String()
	}

	log.Infof("remote addr %v", ipAddr)
	services := whitelist.Kubernetes.APIConn.ServiceList()

	pod := whitelist.Kubernetes.APIConn.PodIndex(ipAddr)[0]

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

	log.Infof("handling service %s", service)

	if wlRecord, ok := whitelist.ServicesToHosts[service]; ok {
		log.Infof("query %s", r.Question[0].Name)
		if r.Question[0].Name == wlRecord {
			log.Infof("%s whitelisted for service %s", r.Question[0].Name, service)
			return plugin.NextOrFailure(whitelist.Name(), whitelist.Next, ctx, rw, r)
		}
	}

	m.SetRcode(r, dns.RcodeNameError)
	// does not matter if this write fails
	rw.WriteMsg(m)

	//return whitelist.Kubernetes.ServeDNS(ctx, rw, r)

	return dns.RcodeNameError, errors.New("not whitelisted")

}

func (whitelist Whitelist) Name() string {
	return "whitelist"
}

func toServiceName(addr net.Addr) string {
	return "test"
}
