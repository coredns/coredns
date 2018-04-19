// Package etcd3 provides the etcd version 3 backend plugin.
package etcd3

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/etcd/msg"
	"github.com/coredns/coredns/plugin/pkg/fall"
	"github.com/coredns/coredns/plugin/proxy"
	"github.com/coredns/coredns/request"

	"github.com/coredns/coredns/plugin/pkg/upstream"
	etcdcv3 "github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/mvcc/mvccpb"
	"github.com/miekg/dns"
	"golang.org/x/net/context"
)

const (
	priority       = 10  // default priority when nothing is set
	ttl            = 300 // default ttl when nothing is set
	etcdTimeout    = 5 * time.Second
	keyNotFoundErr = "Key not found"
)

// Etcd is a plugin talks to an etcd cluster.
type Etcd struct {
	Next       plugin.Handler
	Fall       fall.F
	Zones      []string
	PathPrefix string
	Upstream   upstream.Upstream // Proxy for looking up names during the resolution process
	Client     *etcdcv3.Client
	Ctx        context.Context
	Stubmap    *map[string]proxy.Proxy // list of proxies for stub resolving.

	endpoints []string // Stored here as well, to aid in testing.
}

// Services implements the ServiceBackend interface.
func (e *Etcd) Services(state request.Request, exact bool, opt plugin.Options) (services []msg.Service, err error) {
	services, err = e.Records(state, exact)
	if err != nil {
		return
	}

	services = msg.Group(services)
	return
}

// Reverse implements the ServiceBackend interface.
func (e *Etcd) Reverse(state request.Request, exact bool, opt plugin.Options) (services []msg.Service, err error) {
	return e.Services(state, exact, opt)
}

// Lookup implements the ServiceBackend interface.
func (e *Etcd) Lookup(state request.Request, name string, typ uint16) (*dns.Msg, error) {
	return e.Upstream.Lookup(state, name, typ)
}

// IsNameError implements the ServiceBackend interface.
func (e *Etcd) IsNameError(err error) bool {
	if err.Error() == keyNotFoundErr {
		return true
	}
	return false
}

// Records looks up records in etcd. If exact is true, it will lookup just this
// name. This is used when find matches when completing SRV lookups for instance.
func (e *Etcd) Records(state request.Request, exact bool) ([]msg.Service, error) {
	name := state.Name()

	path, star := msg.PathWithWildcard(name, e.PathPrefix)
	r, err := e.get(path, true)
	if err != nil {
		return nil, err
	}
	segments := strings.Split(msg.Path(name, e.PathPrefix), "/")
	return e.loopNodes(r.Kvs, segments, star, nil)
}

// get is a wrapper for client.Get
func (e *Etcd) get(path string, recursive bool) (*etcdcv3.GetResponse, error) {
	ctx, cancel := context.WithTimeout(e.Ctx, etcdTimeout)
	defer cancel()
	if recursive == true {
		r, err := e.Client.Get(ctx, path, etcdcv3.WithPrefix())
		if err != nil {
			return nil, err
		}
		return r, nil
	}

	r, err := e.Client.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	if r.Count == 0 {
		return nil, errors.New(keyNotFoundErr)
	}
	return r, nil
}

// skydns/local/skydns/east/staging/web
// skydns/local/skydns/west/production/web
//
// skydns/local/skydns/*/*/web
// skydns/local/skydns/*/web

// loopNodes recursively loops through the nodes and returns all the values. The nodes' keyname
// will be match against any wildcards when star is true.
func (e *Etcd) loopNodes(kv []*mvccpb.KeyValue, nameParts []string, star bool, bx map[msg.Service]bool) (sx []msg.Service, err error) {
	if bx == nil {
		bx = make(map[msg.Service]bool)
	}
Nodes:
	for _, n := range kv {
		if star {
			s := string(n.Key[:])
			keyParts := strings.Split(s, "/")
			for i, n := range nameParts {
				if i > len(keyParts)-1 {
					// name is longer than key
					continue Nodes
				}
				if n == "*" || n == "any" {
					continue
				}
				if keyParts[i] != n {
					continue Nodes
				}
			}
		}
		serv := new(msg.Service)
		if err := json.Unmarshal(n.Value, serv); err != nil {
			return nil, fmt.Errorf("%s: %s", n.Key, err.Error())
		}
		b := msg.Service{Host: serv.Host, Port: serv.Port, Priority: serv.Priority, Weight: serv.Weight, Text: serv.Text, Key: string(n.Key[:])}
		bx[b] = true

		serv.Key = string(n.Key[:])
		serv.TTL = e.TTL(n, serv)
		if serv.Priority == 0 {
			serv.Priority = priority
		}
		sx = append(sx, *serv)
	}
	return sx, nil
}

// TTL returns the smaller of the etcd TTL and the service's
// TTL. If neither of these are set (have a zero value), a default is used.
func (e *Etcd) TTL(kv *mvccpb.KeyValue, serv *msg.Service) uint32 {
	etcdTTL := uint32(kv.Lease)

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
