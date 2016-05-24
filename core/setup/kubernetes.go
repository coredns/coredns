package setup

import (
//	"crypto/tls"
//	"crypto/x509"
    "fmt"
//	"io/ioutil"
//	"net"
//	"net/http"
//	"time"

	"github.com/miekg/coredns/middleware"
	"github.com/miekg/coredns/middleware/kubernetes"
	"github.com/miekg/coredns/middleware/proxy"
//	"github.com/miekg/coredns/middleware/singleflight"

	"golang.org/x/net/context"
)

const defaultK8sEndpoint = "http://localhost:8080"

// Kubernetes sets up the kubernetes middleware.
func Kubernetes(c *Controller) (middleware.Middleware, error) {
    fmt.Println("controller %v", c)
    // TODO: Determine if subzone support required

	kubernetes, err := kubernetesParse(c)

	if err != nil {
		return nil, err
	}

	return func(next middleware.Handler) middleware.Handler {
		kubernetes.Next = next
		return kubernetes
	}, nil
}

func kubernetesParse(c *Controller) (kubernetes.Kubernetes, error) {
	k8s := kubernetes.Kubernetes{
		Proxy:      proxy.New([]string{"8.8.8.8:53", "8.8.4.4:53"}),
		PathPrefix: "skydns",
		Ctx:        context.Background(),
//		Inflight:   &singleflight.Group{},
	}
	var (
		endpoints     = []string{defaultK8sEndpoint}
	)
	for c.Next() {
		if c.Val() == "kubernetes" {
			k8s.Zones = c.RemainingArgs()
			if len(k8s.Zones) == 0 {
				k8s.Zones = c.ServerBlockHosts
			}
			middleware.Zones(k8s.Zones).FullyQualify()
			if c.NextBlock() {
				// TODO(miek): 2 switches?
				switch c.Val() {
				case "path":
					if !c.NextArg() {
						return kubernetes.Kubernetes{}, c.ArgErr()
					}
					k8s.PathPrefix = c.Val()
				case "endpoint":
					args := c.RemainingArgs()
					if len(args) == 0 {
						return kubernetes.Kubernetes{}, c.ArgErr()
					}
					endpoints = args
/*
				case "upstream":
					args := c.RemainingArgs()
					if len(args) == 0 {
						return kubernetes.Kubernetes{}, c.ArgErr()
					}
					for i := 0; i < len(args); i++ {
						h, p, e := net.SplitHostPort(args[i])
						if e != nil && p == "" {
							args[i] = h + ":53"
						}
					}
					endpoints = args
					k8s.Proxy = proxy.New(args)
*/
				}
				for c.Next() {
					switch c.Val() {
					case "path":
						if !c.NextArg() {
							return kubernetes.Kubernetes{}, c.ArgErr()
						}
						k8s.PathPrefix = c.Val()
					case "endpoint":
						args := c.RemainingArgs()
						if len(args) == 0 {
							return kubernetes.Kubernetes{}, c.ArgErr()
						}
						endpoints = args
/*
					case "upstream":
						args := c.RemainingArgs()
						if len(args) == 0 {
							return kubernetes.Kubernetes{}, c.ArgErr()
						}
						for i := 0; i < len(args); i++ {
							h, p, e := net.SplitHostPort(args[i])
							if e != nil && p == "" {
								args[i] = h + ":53"
							}
						}
						k8s.Proxy = proxy.New(args)
*/
					}
				}
			}
			return k8s, nil
		}
        fmt.Println("endpoints='%v'", endpoints)
	}
	return kubernetes.Kubernetes{}, nil
}
