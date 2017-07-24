// Package consul provides the consul backend middleware.
package consul

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/coredns/coredns/middleware"
	"github.com/coredns/coredns/middleware/etcd/msg"
	"github.com/coredns/coredns/middleware/pkg/cache"
	"github.com/coredns/coredns/middleware/pkg/singleflight"
	"github.com/coredns/coredns/middleware/proxy"
	"github.com/coredns/coredns/request"

	consulapi "github.com/hashicorp/consul/api"
	"github.com/miekg/dns"
)

var errNoItems = errors.New("no items found")

// Consul is a middleware talks to the Consul server.
type Consul struct {
	Next       middleware.Handler
	Zones      []string
	PathPrefix string
	Proxy      proxy.Proxy // Proxy for looking up names during the resolution process
	Client     *consulapi.Client
	ConsulKV   *consulapi.KV
	Inflight   *singleflight.Group
	Stubmap    *map[string]proxy.Proxy // list of proxies for stub resolving.
	Debugging  bool                    // Do we allow debug queries.

	endpoint string // Stored here as well, to aid in testing.
}

// Services implements the ServiceBackend interface.
func (e *Consul) Services(state request.Request, exact bool, opt middleware.Options) (services, debug []msg.Service, err error) {
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

// Reverse implements the ServiceBackend interface.
func (e *Consul) Reverse(state request.Request, exact bool, opt middleware.Options) (services, debug []msg.Service, err error) {
	return e.Services(state, exact, opt)
}

// Lookup implements the ServiceBackend interface.
func (e *Consul) Lookup(state request.Request, name string, typ uint16) (*dns.Msg, error) {
	return e.Proxy.Lookup(state, name, typ)
}

// IsNameError implements the ServiceBackend interface.
func (e *Consul) IsNameError(err error) bool {
	return err == errNoItems
}

// Debug implements the ServiceBackend interface.
func (e *Consul) Debug() string {
	return e.PathPrefix
}

// Records looks up records in Consul. If exact is true, it will lookup just this
// name. This is used when find matches when completing SRV lookups for instance.
func (e *Consul) Records(name string, exact bool) (records []msg.Service, err error) {
	path, star := msg.PathWithWildcard(name, e.PathPrefix)
	pairs, err := e.get(path)
	if err != nil {
		return nil, err
	}

	if len(*pairs) == 0 {
		return nil, errNoItems
	}

	segments := strings.Split(msg.Path(name, e.PathPrefix), "/")

	if exact {
		// Consul doesn't return keys with '/' as a prefix.
		// Instead of chaning all keys, only change the path
		path = path[1:]
		for _, pair := range *pairs {
			if pair.Key == path {
				return e.loopKV(&consulapi.KVPairs{pair}, segments, star)
			}
		}
		return nil, errNoItems
	} else {
		return e.loopKV(pairs, segments, star)
	}
}

// get is a wrapper for client.KV().List() that uses SingleInflight to suppress multiple outstanding queries.
func (e *Consul) get(path string) (*consulapi.KVPairs, error) {
	hash := cache.Hash([]byte(path))

	resp, err := e.Inflight.Do(hash, func() (interface{}, error) {
		pairs, _, err := e.ConsulKV.List(path, nil)
		if err != nil {
			return nil, err
		}
		return &pairs, nil
	})
	if err != nil {
		return nil, err
	}

	return resp.(*consulapi.KVPairs), nil
}

// Consul adds an * after the search if recurse=true.
// Thus, if we have something like:
//    ConsulKV.List("/skydns/test/skydns/mx", nil)
// It will return:
//    skydns/test/skydns/mx/a
//    skydns/test/skydns/mx2/a
//    skydns/test/skydns/mx2/b
//    skydns/test/skydns/mx3/a
//    skydns/test/skydns/mx3/a
// In reality, we only want the first one.
// Adding a '/' at the end of the key only works if the key is a directory, not a value.
// loopKV() returns the valid KV pairs as []msg.Service
func (e *Consul) loopKV(pairs *consulapi.KVPairs, nameParts []string, star bool) (records []msg.Service, err error) {
	bx := make(map[msg.Service]bool)
Pairs:
	for _, pair := range *pairs {
		// Consul returns keys without a '/', but CoreDNS needs it.
		pair.Key = "/" + pair.Key
		keyParts := strings.Split(pair.Key, "/")

		// Magic for star and Consul filtering
		for i, n := range nameParts {
			if i > len(keyParts)-1 {
				// name is longer than key
				continue Pairs
			}
			if star && (n == "*" || n == "any") {
				continue
			}
			if keyParts[i] != n {
				continue Pairs
			}
		}

		s := new(msg.Service)
		err := json.Unmarshal(pair.Value, s)
		if err != nil {
			return nil, err
		}

		b := msg.Service{Host: s.Host, Port: s.Port, Priority: s.Priority, Weight: s.Weight, Text: s.Text, Key: pair.Key}
		if _, ok := bx[b]; ok {
			continue
		}
		bx[b] = true

		s.Key = pair.Key
		if s.TTL == 0 {
			s.TTL = ttl
		}
		if s.Priority == 0 {
			s.Priority = priority
		}
		records = append(records, *s)
	}
	return records, nil
}

const (
	priority = 10  // default priority when nothing is set
	ttl      = 300 // default ttl when nothing is set
)
