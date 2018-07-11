package whitelist

import (
	"context"
	"errors"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/kubernetes"
	"github.com/coredns/coredns/plugin/pkg/dnsutil"
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

	log.Infof("query %s", r.Question[0].Name)

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative, m.RecursionAvailable = true, true
	remoteAddr := rw.RemoteAddr()

	state := request.Request{W: rw, Req: r, Context: ctx}

	req, err := parseRequest(state)

	if err != nil {
		log.Error(err)
		return plugin.NextOrFailure(whitelist.Name(), whitelist.Next, ctx, rw, r)
	}

	if req.namespace != "" && req.namespace != "default" {
		log.Infof("not handling namespace %s", req.namespace)
		return plugin.NextOrFailure(whitelist.Name(), whitelist.Next, ctx, rw, r)
	}

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

	if whitelisted, ok := whitelist.ServicesToWhitelist[service]; ok {
		query := r.Question[0].Name
		log.Infof("handling service %s", service)
		if _, ok := whitelisted[query]; ok {
			log.Infof("%s whitelisted for service %s", query, service)
			return plugin.NextOrFailure(whitelist.Name(), whitelist.Next, ctx, rw, r)
		}
	}

	m.SetRcode(r, dns.RcodeNameError)
	rw.WriteMsg(m)
	return dns.RcodeNameError, errors.New("not whitelisted")

}

func (whitelist Whitelist) Name() string {
	return "whitelist"
}

func parseRequest(state request.Request) (r recordRequest, err error) {
	// 3 Possible cases:
	// 1. _port._protocol.service.namespace.pod|svc.zone
	// 2. (endpoint): endpoint.service.namespace.pod|svc.zone
	// 3. (service): service.namespace.pod|svc.zone
	//
	// Federations are handled in the federation plugin. And aren't parsed here.

	base, _ := dnsutil.TrimZone(state.Name(), state.Zone)
	// return NODATA for apex queries
	if base == "" || base == kubernetes.Svc || base == kubernetes.Pod {
		return r, nil
	}

	log.Infof("base %v", base)

	segs := dns.SplitDomainName(base)

	r.port = "*"
	r.protocol = "*"
	r.service = "*"
	r.namespace = "*"
	// r.endpoint is the odd one out, we need to know if it has been set or not. If it is
	// empty we should skip the endpoint check in k.get(). Hence we cannot set if to "*".

	// start at the right and fill out recordRequest with the bits we find, so we look for
	// pod|svc.namespace.service and then either
	// * endpoint
	// *_protocol._port

	last := len(segs) - 1
	if last < 0 {
		return r, nil
	}

	log.Infof("segs %v", segs)
	r.podOrSvc = segs[last]
	if r.podOrSvc != kubernetes.Pod && r.podOrSvc != kubernetes.Svc {
		return r, errors.New("invalid request1")
	}
	last--
	if last < 0 {
		return r, nil
	}

	r.namespace = segs[last]
	last--
	if last < 0 {
		return r, nil
	}

	r.service = segs[last]
	last--
	if last < 0 {
		return r, nil
	}

	// Because of ambiquity we check the labels left: 1: an endpoint. 2: port and protocol.
	// Anything else is a query that is too long to answer and can safely be delegated to return an nxdomain.
	switch last {

	case 0: // endpoint only
		r.endpoint = segs[last]
	case 1: // service and port
		r.protocol = stripUnderscore(segs[last])
		r.port = stripUnderscore(segs[last-1])

	default: // too long
		return r, errors.New("invalid request2")
	}

	return r, nil
}

type recordRequest struct {
	// The named port from the kubernetes DNS spec, this is the service part (think _https) from a well formed
	// SRV record.
	port string
	// The protocol is usually _udp or _tcp (if set), and comes from the protocol part of a well formed
	// SRV record.
	protocol string
	endpoint string
	// The servicename used in Kubernetes.
	service string
	// The namespace used in Kubernetes.
	namespace string
	// A each name can be for a pod or a service, here we track what we've seen, either "pod" or "service".
	podOrSvc string
}

// stripUnderscore removes a prefixed underscore from s.
func stripUnderscore(s string) string {
	if s[0] != '_' {
		return s
	}
	return s[1:]
}
