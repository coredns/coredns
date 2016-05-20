package setup

import (
//	"crypto/tls"
//	"crypto/x509"
    "fmt"
//	"io/ioutil"
	"net"
//	"net/http"
//	"time"

	"github.com/miekg/coredns/middleware"
	"github.com/miekg/coredns/middleware/kubernetes"
	"github.com/miekg/coredns/middleware/proxy"
	"github.com/miekg/coredns/middleware/singleflight"

	"golang.org/x/net/context"
)

const defaultK8sEndpoint = "http://localhost:8080"

// Kubernetes sets up the kubernetes middleware.
func Kubernetes(c *Controller) (middleware.Middleware, error) {
    // TODO: Determine if subzone support required
	kubernetes, stubzones, err := kubernetesParse(c)
    fmt.Println("stubzones: %v", stubzones)

	if err != nil {
		return nil, err
	}

	return func(next middleware.Handler) middleware.Handler {
		kubernetes.Next = next
		return kubernetes
	}, nil
}

func kubernetesParse(c *Controller) (kubernetes.Kubernetes, bool, error) {
	stub := make(map[string]proxy.Proxy)
	k8s := kubernetes.Kubernetes{
		Proxy:      proxy.New([]string{"8.8.8.8:53", "8.8.4.4:53"}),
		PathPrefix: "skydns",
		Ctx:        context.Background(),
		Inflight:   &singleflight.Group{},
		Stubmap:    &stub,
	}
	var (
		tlsCertFile   = ""
		tlsKeyFile    = ""
		tlsCAcertFile = ""
		endpoints     = []string{defaultK8sEndpoint}
		stubzones     = false
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
				case "stubzones":
					stubzones = true
				case "path":
					if !c.NextArg() {
						return kubernetes.Kubernetes{}, false, c.ArgErr()
					}
					k8s.PathPrefix = c.Val()
				case "endpoint":
					args := c.RemainingArgs()
					if len(args) == 0 {
						return kubernetes.Kubernetes{}, false, c.ArgErr()
					}
					endpoints = args
				case "upstream":
					args := c.RemainingArgs()
					if len(args) == 0 {
						return kubernetes.Kubernetes{}, false, c.ArgErr()
					}
					for i := 0; i < len(args); i++ {
						h, p, e := net.SplitHostPort(args[i])
						if e != nil && p == "" {
							args[i] = h + ":53"
						}
					}
					endpoints = args
					k8s.Proxy = proxy.New(args)
				case "tls": // cert key cacertfile
					args := c.RemainingArgs()
					if len(args) != 3 {
						return kubernetes.Kubernetes{}, false, c.ArgErr()
					}
					tlsCertFile, tlsKeyFile, tlsCAcertFile = args[0], args[1], args[2]
				}
				for c.Next() {
					switch c.Val() {
					case "stubzones":
						stubzones = true
					case "path":
						if !c.NextArg() {
							return kubernetes.Kubernetes{}, false, c.ArgErr()
						}
						k8s.PathPrefix = c.Val()
					case "endpoint":
						args := c.RemainingArgs()
						if len(args) == 0 {
							return kubernetes.Kubernetes{}, false, c.ArgErr()
						}
						endpoints = args
					case "upstream":
						args := c.RemainingArgs()
						if len(args) == 0 {
							return kubernetes.Kubernetes{}, false, c.ArgErr()
						}
						for i := 0; i < len(args); i++ {
							h, p, e := net.SplitHostPort(args[i])
							if e != nil && p == "" {
								args[i] = h + ":53"
							}
						}
						k8s.Proxy = proxy.New(args)
					case "tls": // cert key cacertfile
						args := c.RemainingArgs()
						if len(args) != 3 {
							return kubernetes.Kubernetes{}, false, c.ArgErr()
						}
						tlsCertFile, tlsKeyFile, tlsCAcertFile = args[0], args[1], args[2]
					}
				}
			}
			return k8s, stubzones, nil
		}
        fmt.Println("[tlsCertFile='%v', tlsKeyFile='%v', tlsCAcertFile='%v', endpoints='%v'", tlsCertFile, tlsKeyFile, tlsCAcertFile, endpoints)
	}
	return kubernetes.Kubernetes{}, false, nil
}
