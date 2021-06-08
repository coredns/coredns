package forwardcrd

import (
	"context"
	"fmt"
	"strings"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/forward"
	corednsv1alpha1 "github.com/coredns/coredns/plugin/forwardcrd/apis/coredns/v1alpha1"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// ForwardCRD represents a plugin instance that can watch Forward CRDs
// within a Kubernetes clusters to dynamically configure stub-domains to proxy
// requests to an upstream resolver.
type ForwardCRD struct {
	Zones             []string
	APIServerEndpoint string
	APIClientCert     string
	APIClientKey      string
	APICertAuth       string
	Namespace         string
	ClientConfig      clientcmd.ClientConfig
	APIConn           forwardCRDController
	Next              plugin.Handler

	pluginInstanceMap *PluginInstanceMap
}

// New returns a new ForwardCRD instance.
func New() *ForwardCRD {
	return &ForwardCRD{
		Namespace: "kube-system",

		pluginInstanceMap: NewPluginInstanceMap(),
	}
}

// Name implements plugin.Handler.
func (k *ForwardCRD) Name() string { return "forwardcrd" }

// ServeDNS implements plugin.Handler.
func (k *ForwardCRD) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	question := strings.ToLower(r.Question[0].Name)

	state := request.Request{W: w, Req: r}
	if !k.match(state) {
		return plugin.NextOrFailure(k.Name(), k.Next, ctx, w, r)
	}

	var (
		offset int
		end    bool
	)

	for {
		p, ok := k.pluginInstanceMap.Get(question[offset:])
		if ok {
			a, b := p.ServeDNS(ctx, w, r)
			return a, b
		}

		offset, end = dns.NextLabel(question, offset)
		if end {
			break
		}
	}

	return plugin.NextOrFailure(k.Name(), k.Next, ctx, w, r)
}

// Ready implements the ready.Readiness interface
func (k *ForwardCRD) Ready() bool {
	return k.APIConn.HasSynced()
}

// InitKubeCache initializes a new Kubernetes cache.
func (k *ForwardCRD) InitKubeCache(ctx context.Context) error {
	config, err := k.getClientConfig()
	if err != nil {
		return err
	}

	dynamicKubeClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create forwardcrd controller: %q", err)
	}

	scheme := runtime.NewScheme()
	err = corednsv1alpha1.AddToScheme(scheme)
	if err != nil {
		return fmt.Errorf("failed to create forwardcrd controller: %q", err)
	}

	k.APIConn = newForwardCRDController(ctx, dynamicKubeClient, scheme, k.Namespace, k.pluginInstanceMap, func(cfg forward.ForwardConfig) (lifecyclePluginHandler, error) {
		return forward.NewWithConfig(cfg)
	})

	return nil
}

func (k *ForwardCRD) getClientConfig() (*rest.Config, error) {
	if k.ClientConfig != nil {
		return k.ClientConfig.ClientConfig()
	}
	loadingRules := &clientcmd.ClientConfigLoadingRules{}
	overrides := &clientcmd.ConfigOverrides{}
	clusterinfo := clientcmdapi.Cluster{}
	authinfo := clientcmdapi.AuthInfo{}

	// Connect to API from in cluster
	if k.APIServerEndpoint == "" {
		cc, err := rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
		cc.ContentType = "application/vnd.kubernetes.protobuf"
		return cc, err
	}

	// Connect to API from out of cluster
	clusterinfo.Server = k.APIServerEndpoint

	if len(k.APICertAuth) > 0 {
		clusterinfo.CertificateAuthority = k.APICertAuth
	}
	if len(k.APIClientCert) > 0 {
		authinfo.ClientCertificate = k.APIClientCert
	}
	if len(k.APIClientKey) > 0 {
		authinfo.ClientKey = k.APIClientKey
	}

	overrides.ClusterInfo = clusterinfo
	overrides.AuthInfo = authinfo
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)

	cc, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	cc.ContentType = "application/vnd.kubernetes.protobuf"
	return cc, err
}

func (k *ForwardCRD) match(state request.Request) bool {
	for _, zone := range k.Zones {
		if plugin.Name(zone).Matches(state.Name()) || dns.Name(state.Name()) == dns.Name(zone) {
			return true
		}
	}

	return false
}
