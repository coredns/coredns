package kubernetes

import (
	"sync"

	api "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
)

type endpointips struct {
	keys  map[string]*string
	mutex sync.Mutex
}

func newEndpointIPs() *endpointips {
	epips := new(endpointips)
	epips.keys = make(map[string]*string)
	return epips
}

func (dns *dnsControl) AddEndpoints(obj interface{}) {
	if dns.revIdxAllEndpoints {
		return
	}

	ep, ok := obj.(*api.Endpoints)
	if !ok {
		svc, isSvc := obj.(*api.Service)
		if !isSvc {
			return
		}
		if svc.Spec.ClusterIP != api.ClusterIPNone {
			key, err := cache.MetaNamespaceKeyFunc(svc)
			if err != nil {
				return
			}
			ep := dns.EpIndex(key)
			if ep == nil {
				return
			}
			dns.DeleteEndpoints(ep)
		}
		return
	}

	key, err := cache.MetaNamespaceKeyFunc(ep)
	if err != nil {
		return
	}
	svc := dns.SvcIndex(key)
	if svc != nil {
		if svc.Spec.ClusterIP != api.ClusterIPNone {
			return
		}
	}
	for _, s := range ep.Subsets {
		for _, a := range s.Addresses {
			dns.headlessEndpoints.add(a.IP, ep)
		}
	}
}

func (dns *dnsControl) DeleteEndpoints(obj interface{}) {
	if dns.revIdxAllEndpoints {
		return
	}
	ep, ok := obj.(*api.Endpoints)
	if !ok {
		return
	}
	for _, s := range ep.Subsets {
		for _, a := range s.Addresses {
			dns.headlessEndpoints.delete(a.IP)
		}
	}
}

func (dns *dnsControl) UpdateEndpoints(oldObj, newObj interface{}) {
	if dns.revIdxAllEndpoints {
		return
	}
	oldEp, ok := oldObj.(*api.Endpoints)
	newEp, fine := newObj.(*api.Endpoints)
	if !(ok && fine) {
		return
	}
	key, err := cache.MetaNamespaceKeyFunc(oldEp)
	if err != nil {
		return
	}
	svc := dns.SvcIndex(key)
	if svc == nil {
		return
	}
	if svc.Spec.ClusterIP != api.ClusterIPNone {
		return
	}
	for _, os := range oldEp.Subsets {
		for _, oa := range os.Addresses {
			found := false
		FindDelete:
			for _, ns := range newEp.Subsets {
				for _, na := range ns.Addresses {
					if oa.IP == na.IP {
						found = true
						break FindDelete
					}
				}
			}
			if !found {
				// remove item from map
				dns.headlessEndpoints.delete(oa.IP)
			}
		}
	}
	// Find new IPs (IPs in newEp, but not in oldEp)
	for _, ns := range newEp.Subsets {
		for _, na := range ns.Addresses {
			found := false
		FindAdd:
			for _, os := range oldEp.Subsets {
				for _, oa := range os.Addresses {
					if oa.IP == na.IP {
						found = true
						break FindAdd
					}
				}
			}
			if !found {
				// add item to map
				dns.headlessEndpoints.add(na.IP, newEp)
			}
		}
	}
}

func (e *endpointips) get(ip string) string {
	e.mutex.Lock()
	ep := e.keys[ip]
	e.mutex.Unlock()
	return *ep
}

func (e *endpointips) add(ip string, ep *api.Endpoints) {
	epKey, err := cache.MetaNamespaceKeyFunc(ep)
	if err != nil {
		return
	}
	e.mutex.Lock()
	e.keys[ip] = &epKey
	e.mutex.Unlock()
}

func (e *endpointips) delete(ip string) {
	e.mutex.Lock()
	e.keys[ip] = nil
	delete(e.keys, ip)
	e.mutex.Unlock()
}
