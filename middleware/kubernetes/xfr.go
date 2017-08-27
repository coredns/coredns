package kubernetes

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/coredns/coredns/middleware/pkg/dnsutil"

	"github.com/miekg/dns"
	"k8s.io/client-go/1.5/pkg/api"
)

type Xfr struct {
	*Kubernetes
	sync.RWMutex
	epoch      time.Time
	transferTo []string
}

// ServeDNS implements the middleware.Handler interface.
func (x *Xfr) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	/*
		state := request.Request{W: w, Req: r}
		if transfer.Allowed(state, x.TransferTo) {
			return dns.RcodeServerFailure, nil
		}
		if state.QType() != dns.TypeAXFR && state.QType() != dns.TypeIXFR {
			return 0, middleware.Error(x.Name(), fmt.Errorf("xfr called with non transfer type: %d", state.QType()))
		}

		records := x.All()
		if len(records) == 0 {
			return dns.RcodeServerFailure, nil
		}

		log.Printf("[INFO] Outgoing transfer of %d records of zone %s to %s started", len(records), x.origin, state.IP())
		// get soa record
		//	records = append(records, records[0]) // add closing SOA to the end

		//	transfer.Out(state, records)
	*/

	return dns.RcodeSuccess, nil
}

// Name implements the middleware.Hander interface.
func (x *Xfr) Name() string { return "xfr" }

func NewXfr(k *Kubernetes) *Xfr {
	return &Xfr{Kubernetes: k, epoch: time.Now().UTC()}
}

// All returns all kubernetes records with a SOA at the start.
func (x *Xfr) All(zone string) []dns.RR {

	res := []dns.RR{}

	serviceList := x.APIConn.ServiceList()
	for _, svc := range serviceList {

		name := dnsutil.Join([]string{svc.Name, svc.Namespace, zone})

		// Endpoint query or headless service
		if svc.Spec.ClusterIP == api.ClusterIPNone {

			endpointsList := x.APIConn.EndpointsList()
			for _, ep := range endpointsList.Items {
				if ep.ObjectMeta.Name != svc.Name || ep.ObjectMeta.Namespace != svc.Namespace {
					continue
				}
				for _, eps := range ep.Subsets {
					for _, addr := range eps.Addresses {
						for _, p := range eps.Ports {

							ip := net.ParseIP(addr.IP).To4()

							h := newHdr(dns.TypeA, name)
							a := &dns.A{Hdr: h, A: ip}
							res = append(res, a)

							h = newHdr(dns.TypeA, endpointHostname(addr), name)
							a = &dns.A{Hdr: h, A: ip}
							res = append(res, a)

							h = newHdr(dns.TypeSRV, p.Name, string(p.Protocol), name)
							s := &dns.SRV{Hdr: h, Port: uint16(p.Port), Target: dnsutil.Join([]string{endpointHostname(addr), name})}
							res = append(res, s)

						}
					}
				}
			}
			continue
		}

		// External service
		if svc.Spec.ExternalName != "" {
			h := newHdr(dns.TypeCNAME, name)
			c := &dns.CNAME{Hdr: h, Target: dns.Fqdn(name)}
			res = append(res, c)
			continue
		}

		// ClusterIP service
		h := newHdr(dns.TypeA, name)
		a := &dns.A{Hdr: h, A: net.ParseIP(svc.Spec.ClusterIP).To4()}
		res = append(res, a)

		for _, p := range svc.Spec.Ports {
			h := newHdr(dns.TypeSRV, p.Name, string(p.Protocol), name)
			s := &dns.SRV{Hdr: h, Port: uint16(p.Port), Target: dns.Fqdn(name)}
			res = append(res, s)
		}
	}
	return res
}

func (x *Xfr) serial() uint32 {
	x.RLock()
	defer x.RUnlock()
	return uint32(x.epoch.Unix())
}

// These handlers are called whenever a watch fires. We just update the serial to now.
func (x *Xfr) AddDeleteHandler(a interface{}) {
	x.Lock()
	defer x.Unlock()
	x.epoch = time.Now().UTC()
}

func (x *Xfr) UpdateHandler(a, b interface{}) {
	x.Lock()
	defer x.Unlock()
	x.epoch = time.Now().UTC()
}

// newHdr returns a new RR header with ownername fully qualifed and ttl set.
func newHdr(typ uint16, labels ...string) dns.RR_Header {
	h := dns.RR_Header{}
	h.Name = dnsutil.Join(labels)
	h.Rrtype = typ
	h.Ttl = 5
	h.Class = dns.ClassINET
	return h
}
