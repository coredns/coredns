package test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/dnstest"

	"github.com/miekg/dns"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// Background cache refreshes must not lose the Pod-dependent search path while
// refreshing an expanded external name. Keep AAAA fresh while A is refreshed to
// detect partial results from a dual-stack lookup as well as outright failures.
func TestKubernetesAutopathCacheRefresh(t *testing.T) {
	for _, options := range []struct{ name, corefile string }{
		{"prefetch-stale", "prefetch 20\nserve_stale"},
		{"stale", "serve_stale"},
	} {
		for _, network := range []string{"udp", "tcp"} {
			t.Run(options.name+"/"+network, func(t *testing.T) {
				// Kubernetes setup installs a process-global klog logger. Isolate
				// each startup from other instances' informer goroutines.
				const childEnv = "COREDNS_AUTOPATH_CACHE_TEST"
				if os.Getenv(childEnv) != t.Name() {
					pattern := "^" + strings.ReplaceAll(t.Name(), "/", "$/^") + "$"
					cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run="+pattern, "-test.v", "-test.timeout=30s")
					cmd.Env = append(os.Environ(), childEnv+"="+t.Name())
					if output, err := cmd.CombinedOutput(); err != nil {
						t.Fatalf("isolated test failed: %v\n%s", err, output)
					}
					return
				}
				kubeconfig := autopathKubeconfig(t)
				const base = "external.example."
				const expanded = base + "client.svc.cluster.local."
				var aQueries atomic.Uint32
				upstream := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
					m := new(dns.Msg)
					m.SetReply(r)
					m.RecursionAvailable = true
					q := r.Question[0]
					h := dns.RR_Header{Name: q.Name, Rrtype: q.Qtype, Class: dns.ClassINET, Ttl: 300}
					switch {
					case q.Name == base && q.Qtype == dns.TypeA:
						ip := net.ParseIP("192.0.2.2")
						if aQueries.Add(1) == 1 {
							h.Ttl = 1
							ip = net.ParseIP("192.0.2.1")
						}
						m.Answer = []dns.RR{&dns.A{Hdr: h, A: ip}}
					case q.Name == base && q.Qtype == dns.TypeAAAA:
						m.Answer = []dns.RR{&dns.AAAA{Hdr: h, AAAA: net.ParseIP("2001:db8::1")}}
					default:
						// Kubernetes also appends the host's resolv.conf search domains.
						m.Rcode = dns.RcodeNameError
						m.Ns = []dns.RR{&dns.SOA{
							Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: 30},
							Ns:  "ns.example.", Mbox: "hostmaster.example.", Serial: 1, Minttl: 30,
						}}
					}
					if err := w.WriteMsg(m); err != nil {
						t.Error(err)
					}
				})
				t.Cleanup(upstream.Close)
				corefile := fmt.Sprintf(`.:0 {
    kubernetes cluster.local {
        pods verified
        kubeconfig %q
    }
    autopath @kubernetes
    cache {
        success 1024 3600 0
        denial 1024 1800 0
        %s
    }
    forward . %s
    loadbalance
}`, filepath.ToSlash(kubeconfig), options.corefile, upstream.Addr)
				server, udp, tcp, err := CoreDNSServerAndPorts(corefile)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					for _, err := range server.ShutdownCallbacks() {
						t.Error(err)
					}
					if err := server.Stop(); err != nil {
						t.Error(err)
					}
					server.Wait()
				})
				addr := udp
				if network == "tcp" {
					addr = tcp
				}
				_, port, err := net.SplitHostPort(addr)
				if err != nil {
					t.Fatal(err)
				}
				addr = net.JoinHostPort("127.0.0.1", port)
				query := func(name string, qtype uint16) *dns.Msg {
					t.Helper()
					m := new(dns.Msg)
					m.SetQuestion(name, qtype)
					m.SetEdns0(1232, false)
					c := &dns.Client{Net: network, Timeout: 2 * time.Second}
					r, _, err := c.Exchange(m, addr)
					if err != nil {
						t.Fatal(err)
					}
					return r
				}
				hasAddress := func(m *dns.Msg, ip string) bool {
					return m.Rcode == dns.RcodeSuccess && slices.ContainsFunc(m.Answer, func(rr dns.RR) bool {
						switch rr := rr.(type) {
						case *dns.A:
							return rr.A.String() == ip
						case *dns.AAAA:
							return rr.AAAA.String() == ip
						}
						return false
					})
				}
				// Verify that the real Kubernetes informers populated both indexes.
				for name, ip := range map[string]string{
					"internal.client.svc.cluster.local.":  "10.96.0.10",
					"127-0-0-1.client.pod.cluster.local.": "127.0.0.1",
				} {
					if r := query(name, dns.TypeA); !hasAddress(r, ip) {
						t.Fatalf("Kubernetes index check for %s failed: %s", name, r)
					}
				}
				if r := query(expanded, dns.TypeAAAA); !hasAddress(r, "2001:db8::1") {
					t.Fatalf("initial expanded AAAA lookup failed: %s", r)
				}
				if r := query(expanded, dns.TypeA); !hasAddress(r, "192.0.2.1") {
					t.Fatalf("initial expanded A lookup failed: %s", r)
				}

				// A changed address proves refresh completed. serve_stale ensures a
				// delayed test still exercises background refresh, not a cache miss.
				refreshed := false
				for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
					r := query(expanded, dns.TypeA)
					if hasAddress(r, "192.0.2.2") {
						refreshed = true
						break
					}
					if !hasAddress(r, "192.0.2.1") {
						t.Errorf("expanded A lookup lost its answer during refresh: %s", r)
						break
					}
					time.Sleep(10 * time.Millisecond)
				}
				if !refreshed {
					t.Errorf("expanded A lookup did not receive the refreshed address; upstream A queries: %d", aQueries.Load())
				}
				if r := query(base, dns.TypeA); !hasAddress(r, "192.0.2.2") {
					t.Errorf("absolute A control lookup failed: %s", r)
				}
				if r := query(expanded, dns.TypeAAAA); !hasAddress(r, "2001:db8::1") {
					t.Errorf("expanded AAAA control lookup failed: %s", r)
				}
				resolver := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
					d := net.Dialer{}
					return d.DialContext(ctx, network, addr)
				}}
				ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
				defer cancel()
				ips, err := resolver.LookupIPAddr(ctx, expanded)
				if err != nil || !slices.ContainsFunc(ips, func(ip net.IPAddr) bool { return ip.IP.String() == "192.0.2.2" }) ||
					!slices.ContainsFunc(ips, func(ip net.IPAddr) bool { return ip.IP.String() == "2001:db8::1" }) {
					t.Errorf("expected both address families after refresh, got %v, err: %v", ips, err)
				}
			})
		}
	}
}

// Supply list/watch responses over HTTP, leaving the Kubernetes client,
// informers, Pod index and DNS/autopath handlers in the actual plugin chain.
func autopathKubeconfig(t *testing.T) string {
	t.Helper()
	resources := map[string]struct {
		kind, apiVersion string
		objects          []runtime.Object
	}{
		"/api/v1/namespaces": {"Namespace", "v1", []runtime.Object{&corev1.Namespace{
			TypeMeta:   metav1.TypeMeta{Kind: "Namespace", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{Name: "client", ResourceVersion: "1"},
		}}},
		"/api/v1/pods": {"Pod", "v1", []runtime.Object{&corev1.Pod{
			TypeMeta:   metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{Name: "client", Namespace: "client", ResourceVersion: "1"},
			Status:     corev1.PodStatus{PodIP: "127.0.0.1", PodIPs: []corev1.PodIP{{IP: "127.0.0.1"}}, Phase: corev1.PodRunning},
		}}},
		"/api/v1/services": {"Service", "v1", []runtime.Object{&corev1.Service{
			TypeMeta:   metav1.TypeMeta{Kind: "Service", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{Name: "internal", Namespace: "client", ResourceVersion: "1"},
			Spec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeClusterIP, ClusterIP: "10.96.0.10", ClusterIPs: []string{"10.96.0.10"},
				Ports: []corev1.ServicePort{{Name: "http", Port: 80, Protocol: corev1.ProtocolTCP}},
			},
		}}},
		"/apis/discovery.k8s.io/v1/endpointslices": {"EndpointSlice", "discovery.k8s.io/v1", nil},
	}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resource, ok := resources[r.URL.Path]
		if !ok || r.Method != http.MethodGet {
			t.Errorf("unexpected Kubernetes API request: %s %s", r.Method, r.URL)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		if r.URL.Query().Get("watch") != "true" {
			items := make([]runtime.RawExtension, 0, len(resource.objects))
			for _, obj := range resource.objects {
				items = append(items, runtime.RawExtension{Object: obj})
			}
			if err := enc.Encode(&metav1.List{
				TypeMeta: metav1.TypeMeta{Kind: resource.kind + "List", APIVersion: resource.apiVersion},
				ListMeta: metav1.ListMeta{ResourceVersion: "1"}, Items: items,
			}); err != nil {
				t.Error(err)
			}
			return
		}
		// Support both list-then-watch and client-go's streaming initial list.
		if r.URL.Query().Get("sendInitialEvents") == "true" {
			send := func(eventType string, obj runtime.Object) {
				if err := enc.Encode(struct {
					Type   string         `json:"type"`
					Object runtime.Object `json:"object"`
				}{eventType, obj}); err != nil {
					t.Error(err)
				}
			}
			for _, obj := range resource.objects {
				send("ADDED", obj)
			}
			send("BOOKMARK", &metav1.PartialObjectMetadata{
				TypeMeta: metav1.TypeMeta{Kind: resource.kind, APIVersion: resource.apiVersion},
				ObjectMeta: metav1.ObjectMeta{ResourceVersion: "1", Annotations: map[string]string{
					metav1.InitialEventsAnnotationKey: "true",
				}},
			})
		}
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	t.Cleanup(func() {
		api.CloseClientConnections()
		api.Close()
	})
	config, err := clientcmd.Write(clientcmdapi.Config{
		Clusters:       map[string]*clientcmdapi.Cluster{"test": {Server: api.URL}},
		AuthInfos:      map[string]*clientcmdapi.AuthInfo{"test": {}},
		Contexts:       map[string]*clientcmdapi.Context{"test": {Cluster: "test", AuthInfo: "test"}},
		CurrentContext: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(path, config, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
