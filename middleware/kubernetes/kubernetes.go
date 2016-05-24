// Package kubernetes provides the kubernetes backend.
package kubernetes

import (
    "fmt"
	"strings"
	"time"

	"github.com/miekg/coredns/middleware"
	"github.com/miekg/coredns/middleware/kubernetes/msg"
	"github.com/miekg/coredns/middleware/proxy"
	"github.com/miekg/coredns/middleware/singleflight"

	"golang.org/x/net/context"
)

type Kubernetes struct {
	Next       middleware.Handler
	Zones      []string
	Proxy      proxy.Proxy // Proxy for looking up names during the resolution process
	PathPrefix string
	Ctx        context.Context
	Inflight   *singleflight.Group
}

// Records looks up records in etcd. If exact is true, it will lookup just
// this name. This is used when find matches when completing SRV lookups
// for instance.
func (g Kubernetes) Records(name string, exact bool) ([]msg.Service, error) {
	path, star := g.PathWithWildcard(name)
	r, err := g.Get(path, true)
	if err != nil {
		return nil, err
	}
	segments := strings.Split(g.Path(name), "/")
    fmt.Println("segments: %v", segments)
    fmt.Println("star:     %v", star)
	switch {
	//case exact && r.Node.Dir:
	case exact:
		return nil, nil
	//case r.Node.Dir:
	case r:
        // TODO: implement loopNodes
        fmt.Println("Hello... here #2 in kubernetes.Records()")
        return nil, nil
		//return g.loopNodes(r.Node.Nodes, segments, star, nil)
	default:
        // TODO: implement loopNodes
        fmt.Println("Hello... here #2 in kubernetes.Records()")
        return nil, nil
		//return g.loopNodes([]*etcdc.Node{r.Node}, segments, false, nil)
	}
}

// Get is a wrapper for client.Get that uses SingleInflight to suppress multiple outstanding queries.
func (g Kubernetes) Get(path string, recursive bool) (bool, error) {

	return false, nil
}

// skydns/local/skydns/east/staging/web
// skydns/local/skydns/west/production/web
//
// skydns/local/skydns/*/*/web
// skydns/local/skydns/*/web

// loopNodes recursively loops through the nodes and returns all the values. The nodes' keyname
// will be match against any wildcards when star is true.
/*
func (g Kubernetes) loopNodes(ns []*etcdc.Node, nameParts []string, star bool, bx map[msg.Service]bool) (sx []msg.Service, err error) {
	if bx == nil {
		bx = make(map[msg.Service]bool)
	}
Nodes:
	for _, n := range ns {
		if n.Dir {
			nodes, err := g.loopNodes(n.Nodes, nameParts, star, bx)
			if err != nil {
				return nil, err
			}
			sx = append(sx, nodes...)
			continue
		}
		if star {
			keyParts := strings.Split(n.Key, "/")
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
		if err := json.Unmarshal([]byte(n.Value), serv); err != nil {
			return nil, err
		}
		b := msg.Service{Host: serv.Host, Port: serv.Port, Priority: serv.Priority, Weight: serv.Weight, Text: serv.Text, Key: n.Key}
		if _, ok := bx[b]; ok {
			continue
		}
		bx[b] = true

		serv.Key = n.Key
		serv.Ttl = g.Ttl(n, serv)
		if serv.Priority == 0 {
			serv.Priority = priority
		}
		sx = append(sx, *serv)
	}
	return sx, nil
}

// Ttl returns the smaller of the kubernetes TTL and the service's
// TTL. If neither of these are set (have a zero value), a default is used.
func (g Kubernetes) Ttl(node *etcdc.Node, serv *msg.Service) uint32 {
	kubernetesTtl := uint32(node.TTL)

	if kubernetesTtl == 0 && serv.Ttl == 0 {
		return ttl
	}
	if kubernetesTtl == 0 {
		return serv.Ttl
	}
	if serv.Ttl == 0 {
		return kubernetesTtl
	}
	if kubernetesTtl < serv.Ttl {
		return kubernetesTtl
	}
	return serv.Ttl
}
*/

// kubernetesNameError checks if the error is ErrorCodeKeyNotFound from kubernetes.
func isKubernetesNameError(err error) bool {
	return false
}

const (
	priority    = 10  // default priority when nothing is set
	ttl         = 300 // default ttl when nothing is set
	minTtl      = 60
	hostmaster  = "hostmaster"
	k8sTimeout = 5 * time.Second
)
