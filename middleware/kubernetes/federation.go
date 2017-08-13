package kubernetes

import (
	"context"
	"strings"

	"github.com/coredns/coredns/middleware"
	"github.com/coredns/coredns/middleware/etcd/msg"
	"github.com/coredns/coredns/middleware/pkg/dnsutil"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// Federation holds TODO(...).
type Federation struct {
	name string
	zone string
}

type Fed struct {
	k    *Kubernetes
	name string
	zone string
}

const (
	// TODO: Do not hardcode these labels. Pull them out of the API instead.
	//
	// We can get them via ....
	//   import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	//     metav1.LabelZoneFailureDomain
	//     metav1.LabelZoneRegion
	//
	// But importing above breaks coredns with flag collision of 'log_dir'

	availabilityZone = "failure-domain.beta.kubernetes.io/zone"
	region           = "failure-domain.beta.kubernetes.io/region"
)

func (f Fed) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	zone := middleware.Zones(f.k.Zones).Matches(state.Name())
	state.Zone = zone

	newname, ok := stripFederation(state, f.name)
	if ok {
		state.Clear()
		// Fix name in state
		newname = newname
	}
	println("FEDERATION IN", state.Name, "after", newname)

	pr, e := parseRequest(state)
	if e != nil {
		return dns.RcodeServerFailure, e
	}

	services, _, err := f.Services(state, pr, middleware.Options{})
	if f.k.IsNameError(err) {
		if f.k.Fallthrough {
			return middleware.NextOrFailure(f.k.Name(), f.k.Next, ctx, w, r)
		}
		return middleware.BackendError(f.k, zone, dns.RcodeNameError, state, nil /*debug*/, nil /* err */, middleware.Options{})
	}
	if err != nil {
		return dns.RcodeServerFailure, err
	}

	var records []dns.RR
	for _, serv := range services {
		records = append(records, serv.NewCNAME(state.QName(), serv.Host))
	}

	if len(records) == 0 {
		return middleware.BackendError(f.k, zone, dns.RcodeSuccess, state, nil /*debug*/, nil, middleware.Options{})
	}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative, m.RecursionAvailable, m.Compress = true, true, true
	m.Answer = append(m.Answer, records...)

	m = dnsutil.Dedup(m)
	state.SizeAndDo(m)
	m, _ = state.Scrub(m)
	w.WriteMsg(m)

	return dns.RcodeSuccess, nil
}

func (f Fed) Services(state request.Request, r recordRequest, opt middleware.Options) ([]msg.Service, []msg.Service, error) {
	services, pods, err := f.k.get(r)
	if err != nil {
		return nil, nil, err
	}
	if len(services) != 0 || len(pods) != 0 {
		println("uuuuuh")
		return nil, nil, nil /* TODO: should probably be not nil */
	}

	cname := f.CNAME(r)
	if cname.Key != "" {
		return []msg.Service{cname}, nil, nil
	}
	return nil, nil, nil
}

// CNAME returns a service record for the requested federated service with the
// target host in the federated CNAME format which the external DNS provider
// should be able to resolve.
func (f Fed) CNAME(r recordRequest) msg.Service {
	return f.k.federationCNAMERecord(r)
}

// stripFederation removes the federation name from the query. The returned
// boolean is true when the label was removed.
func stripFederation(state request.Request, name string) (string, bool) {

	base, _ := dnsutil.TrimZone(state.Name(), state.Zone)
	segs := dns.SplitDomainName(base)

	if len(segs) < 3 {
		return state.Name(), false
	}
	if name != segs[len(segs)-2] {
		return state.Name(), false
	}

	segs[len(segs)-2] = segs[len(segs)-1]
	segs = segs[:len(segs)-1]
	return strings.Join(segs, "."), true
}

// isFederation checks if the query is a federation query, it returns the
// federation name and zone if true.
func isFederation(state request.Request, feds []Federation) (string, string) {

	base, _ := dnsutil.TrimZone(state.Name(), state.Zone)
	segs := dns.SplitDomainName(base)

	if len(segs) < 3 {
		return "", ""
	}
	for _, f := range feds {
		if f.name == segs[len(segs)-2] {
			return f.name, f.zone
		}
	}
	return "", ""
}

// federationCNAMERecord returns a service record for the requested federated service
// with the target host in the federated CNAME format which the external DNS provider
// should be able to resolve
func (k *Kubernetes) federationCNAMERecord(r recordRequest) msg.Service {

	myNodeName := k.localNodeName()
	node, err := k.APIConn.GetNodeByName(myNodeName)
	if err != nil {
		return msg.Service{}
	}

	for _, f := range k.Federations {
		if f.name != r.federation {
			continue
		}
		if r.endpoint == "" {
			return msg.Service{
				Key:  strings.Join([]string{msg.Path(r.zone, "coredns"), r.podOrSvc, r.federation, r.namespace, r.service}, "/"),
				Host: strings.Join([]string{r.service, r.namespace, r.federation, r.podOrSvc, node.Labels[availabilityZone], node.Labels[region], f.zone}, "."),
			}
		}
		return msg.Service{
			Key:  strings.Join([]string{msg.Path(r.zone, "coredns"), r.podOrSvc, r.federation, r.namespace, r.service, r.endpoint}, "/"),
			Host: strings.Join([]string{r.endpoint, r.service, r.namespace, r.federation, r.podOrSvc, node.Labels[availabilityZone], node.Labels[region], f.zone}, "."),
		}
	}

	return msg.Service{}
}
