package whitelist

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/kubernetes"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
	"k8s.io/api/core/v1"
	"net"
	"strings"
)

var log = clog.NewWithPlugin("whitelist")

type whitelist struct {
	Kubernetes          *kubernetes.Kubernetes
	Next                plugin.Handler
	Discovery           DiscoveryServiceClient
	ServicesToWhitelist map[string]map[string]struct{}
	configPath          string
}

func (whitelist whitelist) ServeDNS(ctx context.Context, rw dns.ResponseWriter, r *dns.Msg) (int, error) {

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
		return plugin.NextOrFailure(whitelist.Name(), whitelist.Next, ctx, rw, r)
	}

	if ns, _ := whitelist.Kubernetes.APIConn.GetNamespaceByName(segs[1]); ns != nil {
		return plugin.NextOrFailure(whitelist.Name(), whitelist.Next, ctx, rw, r)
	}

	service := whitelist.getServiceFromIP(ipAddr)

	if service == nil {
		return plugin.NextOrFailure(whitelist.Name(), whitelist.Next, ctx, rw, r)
	}

	serviceNameInConfig := fmt.Sprintf("%s.%s", service.Name, service.Namespace)
	serviceName := fmt.Sprintf("%s.svc.%s", serviceNameInConfig, whitelist.Kubernetes.Zones[0])

	query := state.Name()
	if whitelisted, ok := whitelist.ServicesToWhitelist[serviceNameInConfig]; ok {
		if _, ok := whitelisted[query]; ok {
			if whitelist.Discovery != nil {
				go whitelist.log(serviceName, state.Name(), "allow")
			}
			return plugin.NextOrFailure(whitelist.Name(), whitelist.Next, ctx, rw, r)
		}
	}

	if whitelist.Discovery != nil {
		go whitelist.log(serviceName, state.Name(), "deny")
	}

	m.SetRcode(r, dns.RcodeNameError)
	rw.WriteMsg(m)
	return dns.RcodeNameError, errors.New("not whitelisted")

}

func (whitelist whitelist) getServiceFromIP(ipAddr string) *v1.Service {

	services := whitelist.Kubernetes.APIConn.ServiceList()
	if services == nil || len(services) == 0 {
		return nil
	}

	pods := whitelist.Kubernetes.APIConn.PodIndex(ipAddr)
	if pods == nil || len(pods) == 0 {
		return nil
	}

	pod := pods[0]

	var service *v1.Service
	for _, svc := range services {
		for pLabelKey, pLabelValue := range pod.Labels {
			if svcLabelValue, ok := svc.Labels[pLabelKey]; ok {
				if strings.EqualFold(pLabelValue, svcLabelValue) {
					service = svc
				}
			}
		}
	}
	return service
}

func (whitelist whitelist) getIpByServiceName(serviceName string) string {

	if serviceName == "" {
		return ""
	}

	serviceNameParts := strings.Split(serviceName, ".")

	service, namespace := "", ""

	//only service name introduced ("zipkin")"
	if len(serviceNameParts) == 1 {
		service, namespace = serviceName, v1.NamespaceDefault
	}

	if len(serviceNameParts) == 2 {
		service, namespace = serviceNameParts[0], serviceNameParts[1]
	} else {
		return ""
	}

	services := whitelist.Kubernetes.APIConn.ServiceList()
	if services == nil || len(services) == 0 {
		return ""
	}

	for _, svc := range services {
		if svc.Name == service && svc.Namespace == namespace {
			return svc.Spec.ClusterIP
		}
	}

	return ""
}

func (whitelist whitelist) Name() string {
	return "whitelist"
}

func (whitelist whitelist) log(service string, query string, action string) {
	fields := make(map[string]string)
	fields["src"] = service
	fields["dst"] = query
	fields["action"] = action
	fields["origin"] = "dns"

	actionBytes := new(bytes.Buffer)
	json.NewEncoder(actionBytes).Encode(fields)

	_, err := whitelist.Discovery.Discover(context.Background(), &Discovery{Msg: actionBytes.Bytes()})
	if err != nil {
		log.Errorf("Log not sent to discovery: %+v", err)
	}

}
