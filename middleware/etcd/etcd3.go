package etcd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/miekg/coredns/middleware"
	"github.com/miekg/coredns/middleware/etcd/msg"
	"github.com/miekg/coredns/request"
	"github.com/miekg/dns"

	etcdc3 "github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/mvcc/mvccpb"
)

// Etcd3 talks to an etcd cluster with the v3 gRPC protocol.
type Etcd3 struct {
	*Etcd
}

// Services implements the ServiceBackend interface.
func (e *Etcd3) Services(state request.Request, exact bool, opt middleware.Options) (services, debug []msg.Service, err error) {
	services, err = e.Records(state.Name(), exact)
	if err != nil {
		return
	}
	if opt.Debug != "" {
		debug = services
	}
	services = msg.Group(services)
	return
}

// Lookup implements the ServiceBackend interface.
func (e *Etcd3) Lookup(state request.Request, name string, typ uint16) (*dns.Msg, error) {
	return e.Proxy.Lookup(state, name, typ)
}

// IsNameError implements the ServiceBackend interface.
func (e *Etcd3) IsNameError(err error) bool {
	//	if ee, ok := err.(etcdc.Error); ok && ee.Code == etcdc.ErrorCodeKeyNotFound {
	//		return true
	//	}
	return false
}

// Debug implements the ServiceBackend interface.
func (e *Etcd3) Debug() string {
	return e.PathPrefix
}

// Records looks up records in etcd. If exact is true, it will lookup just this
// name. This is used when find matches when completing SRV lookups for instance.
func (e *Etcd3) Records(name string, exact bool) ([]msg.Service, error) {
	path, star := msg.PathWithWildcard(name, e.PathPrefix)
	r, err := e.get(path, true)
	if err != nil {
		return nil, err
	}
	segments := strings.Split(msg.Path(name, e.PathPrefix), "/")

	return e.loopNodes(r.Kvs, segments, star, nil)
}

func (e *Etcd3) get(path string, recursive bool) (*etcdc3.GetResponse, error) {
	resp, err := e.Inflight.Do(path, func() (interface{}, error) {
		ctx, cancel := context.WithTimeout(e.Ctx, etcdTimeout)
		defer cancel()

		if recursive == true {
			r, e := e.Client3.Get(ctx, path, etcdc3.WithPrefix())
			//			if r.Kvs == nil {
			//				return nil, fmt.Errorf("ErrorCodeKeyNotFound")
			//			}
			if e != nil {
				return nil, e
			}
			return r, e
		}

		r, e := e.Client3.Get(ctx, path)
		if e != nil {
			return nil, e
		}
		return r, e
	})
	if err != nil {
		return nil, err
	}
	return resp.(*etcdc3.GetResponse), err
}

func (e *Etcd3) loopNodes(kv []*mvccpb.KeyValue, nameParts []string, star bool, bx map[msg.Service]bool) (sx []msg.Service, err error) {
	if bx == nil {
		bx = make(map[msg.Service]bool)
	}

	for i := range kv {

		serv := new(msg.Service)
		if err := json.Unmarshal(kv[i].Value, serv); err != nil {
			return nil, fmt.Errorf("%s: %s", kv[i].Key, err.Error())
		}

		b := msg.Service{Host: serv.Host, Port: serv.Port, Priority: serv.Priority, Weight: serv.Weight, Text: serv.Text, Key: string(kv[i].Key)}
		if _, ok := bx[b]; ok {
			continue
		}
		bx[b] = true

		serv.Key = string(kv[i].Key)
		//TODO: shouldn't be that another call (LeaseRequest) for TTL??
		serv.TTL = e.TTL(kv[i], serv)

		if serv.Priority == 0 {
			serv.Priority = priority
		}

		sx = append(sx, *serv)
	}

	return sx, nil
}

// TTL returns the smaller of the etcd TTL and the service's
// TTL. If neither of these are set (have a zero value), a default is used.
func (e *Etcd3) TTL(node *mvccpb.KeyValue, serv *msg.Service) uint32 {
	etcdTTL := uint32(node.Lease) // TODO: still waiting for Least request rpc to be available in etcdv3's api

	if etcdTTL == 0 && serv.TTL == 0 {
		return ttl
	}
	if etcdTTL == 0 {
		return serv.TTL
	}
	if serv.TTL == 0 {
		return etcdTTL
	}
	if etcdTTL < serv.TTL {
		return etcdTTL
	}
	return serv.TTL
}
