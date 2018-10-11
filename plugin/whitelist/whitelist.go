package whitelist

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
	"k8s.io/api/core/v1"
	api "k8s.io/api/core/v1"
)

var log = clog.NewWithPlugin("whitelist")

type kubeAPI interface {
	ServiceList() []*api.Service
	GetNamespaceByName(string) (*api.Namespace, error)
	PodIndex(string) []*api.Pod
}

type whitelist struct {
	Kubernetes    kubeAPI
	Next          plugin.Handler
	Discovery     DiscoveryServiceClient
	Fallthrough   []string
	Configuration whitelistConfig
	plugin.Zones
}

func (whitelist whitelist) ServeDNS(ctx context.Context, rw dns.ResponseWriter, r *dns.Msg) (int, error) {

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative, m.RecursionAvailable = true, true
	remoteAddr := rw.RemoteAddr()

	state := request.Request{W: rw, Req: r, Context: ctx}

	var sourceIPAddr string
	if ip, ok := remoteAddr.(*net.UDPAddr); !ok {
		log.Debug("failed to cast source IP")
		return plugin.NextOrFailure(whitelist.Name(), whitelist.Next, ctx, rw, r)
	} else {
		sourceIPAddr = ip.IP.String()
		if sourceIPAddr == "" {
			log.Debugf("empty source IP: '%v'", ip)
			return plugin.NextOrFailure(whitelist.Name(), whitelist.Next, ctx, rw, r)
		}
	}
	log.Debugf("source IP: '%s'", sourceIPAddr)

	segs := dns.SplitDomainName(state.Name())
	if len(segs) <= 1 {
		log.Debugf("number of segments: '%d' for state name: '%s'", state.Name())
		return plugin.NextOrFailure(whitelist.Name(), whitelist.Next, ctx, rw, r)
	}

	sourceService := whitelist.getServiceFromIP(sourceIPAddr)
	if sourceService == nil {
		return plugin.NextOrFailure(whitelist.Name(), whitelist.Next, ctx, rw, r)
	}

	query := strings.TrimRight(state.Name(), ".")
	for _, domain := range whitelist.Fallthrough {
		if strings.EqualFold(domain, query) {
			return plugin.NextOrFailure(whitelist.Name(), whitelist.Next, ctx, rw, r)
		}
	}

	querySrcService := fmt.Sprintf("%s.%s", sourceService.Name, sourceService.Namespace)
	queryDstLocation, origin, dstConf := "", "", ""

	if ns, _ := whitelist.Kubernetes.GetNamespaceByName(segs[1]); ns != nil {
		//local kubernetes dstConf
		queryDstLocation = fmt.Sprintf("%s.listentry.%s", segs[0], segs[1])
		origin = ""
		dstConf = fmt.Sprintf("%s.%s", segs[0], segs[1])
	} else {
		//make sure that this is real external query without .cluster.local in the end
		zone := plugin.Zones(whitelist.Zones).Matches(state.Name())
		if zone != "" {
			return plugin.NextOrFailure(whitelist.Name(), whitelist.Next, ctx, rw, r)
		}
		//external query
		origin = "dns"
		queryDstLocation = state.Name()
		dstConf = strings.TrimRight(state.Name(), ".")
	}

	serviceName := fmt.Sprintf("%s.svc.%s", querySrcService, whitelist.Zones[0])

	if whitelist.Configuration.blacklist {
		if whitelist.Discovery != nil {
			go whitelist.log(serviceName, queryDstLocation, origin, "allow")
		}
		return plugin.NextOrFailure(whitelist.Name(), whitelist.Next, ctx, rw, r)
	} else {
		if whitelisted, ok := whitelist.Configuration.SourceToDestination[querySrcService]; ok {
			if _, ok := whitelisted[dstConf]; ok {
				if whitelist.Discovery != nil {
					go whitelist.log(serviceName, queryDstLocation, origin, "allow")
				}
				return plugin.NextOrFailure(whitelist.Name(), whitelist.Next, ctx, rw, r)
			}
		}
	}

	if whitelist.Discovery != nil {
		go whitelist.log(serviceName, queryDstLocation, origin, "deny")
	}

	m.SetRcode(r, dns.RcodeNameError)
	rw.WriteMsg(m)
	return dns.RcodeNameError, errors.New("not whitelisted")

}

func (whitelist whitelist) getServiceFromIP(ipAddr string) *v1.Service {

	services := whitelist.Kubernetes.ServiceList()
	if services == nil || len(services) == 0 {
		return nil
	}

	pods := whitelist.Kubernetes.PodIndex(ipAddr)
	if pods == nil || len(pods) == 0 {
		log.Debugf("failed to get pod from IP: '%s'", ipAddr)
		return nil
	}

	var service *v1.Service
	for _, currService := range services {
		for podLabelKey, podLabelValue := range pods[0].Labels {
			if svcLabelValue, ok := currService.Spec.Selector[podLabelKey]; ok {
				if strings.EqualFold(podLabelValue, svcLabelValue) {
					service = currService
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

	services := whitelist.Kubernetes.ServiceList()
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

func (whitelist whitelist) log(service string, query, origin, action string) {
	fields := make(map[string]string)
	fields["src"] = service
	fields["dst"] = strings.TrimRight(query, ".")
	fields["action"] = action
	fields["origin"] = origin

	actionBytes := new(bytes.Buffer)
	json.NewEncoder(actionBytes).Encode(fields)

	_, err := whitelist.Discovery.Discover(context.Background(), &Discovery{Msg: actionBytes.Bytes()})
	if err != nil {
		log.Errorf("Log not sent to discovery: %+v", err)
	}

}
