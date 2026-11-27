package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestInitKubeCacheRequestLimits(t *testing.T) {
	for _, tc := range []struct {
		name  string
		qps   float32
		burst int
	}{
		{"defaults", 0, 0},
		{"qps", 1, 0},
		{"burst", 0, 2},
		{"qps and burst", 1, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			k := New([]string{"cluster.local."})
			k.ClientConfig = clientcmd.NewDefaultClientConfig(clientcmdapi.Config{}, &clientcmd.ConfigOverrides{
				ClusterInfo: clientcmdapi.Cluster{Server: "http://localhost"},
			})
			k.apiQPS, k.apiBurst = tc.qps, tc.burst
			k.opts.multiclusterZones = []string{"clusterset.local."}
			_, stop, err := k.InitKubeCache(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = stop() })
			controller := k.APIConn.(*dnsControl)
			wantQPS, wantBurst := tc.qps, tc.burst
			if wantQPS == 0 {
				wantQPS = rest.DefaultQPS
			}
			if wantBurst == 0 {
				wantBurst = rest.DefaultBurst
			}
			for name, client := range map[string]rest.Interface{
				"core": controller.client.CoreV1().RESTClient(),
				"mcs":  controller.mcsClient.RESTClient(),
			} {
				limiter := client.GetRateLimiter()
				if got := limiter.QPS(); got != wantQPS {
					t.Errorf("%s QPS = %v, want %v", name, got, wantQPS)
				}
				for i := range wantBurst {
					if !limiter.TryAccept() {
						t.Errorf("%s burst exhausted at %d, want %d", name, i, wantBurst)
						break
					}
				}
				if limiter.TryAccept() {
					t.Errorf("%s burst exceeds %d", name, wantBurst)
				}
			}
		})
	}
	for _, max := range []int{0, 1} {
		t.Run(fmt.Sprintf("inflight=%d", max), func(t *testing.T) {
			var calls atomic.Int32
			entered, release := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					close(entered)
					<-release
				}
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Query().Get("watch") != "true" {
					fmt.Fprint(w, `{"apiVersion":"v1","kind":"ServiceList","items":[]}`)
				}
			}))
			defer server.Close()
			k := New([]string{"cluster.local."})
			k.APIServerList = []string{server.URL}
			k.apiMaxInflight = max
			k.opts.multiclusterZones = []string{"clusterset.local."}
			_, stop, err := k.InitKubeCache(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer stop()
			d := k.APIConn.(*dnsControl)
			done := make(chan error, 1)
			go func() {
				_, err := serviceListFunc(d.client, "", nil)(t.Context(), meta.ListOptions{})
				done <- err
			}()
			<-entered
			ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
			defer cancel()
			watch, err := serviceImportWatchFunc(d.mcsClient, "", nil)(ctx, meta.ListOptions{})
			if watch != nil {
				watch.Stop()
			}
			close(release)
			if firstErr := <-done; firstErr != nil {
				t.Fatal(firstErr)
			}
			if max > 0 {
				if !errors.Is(err, context.DeadlineExceeded) || calls.Load() != 1 {
					t.Errorf("in-flight limit: second request error = %v, transport calls = %d; want deadline exceeded and 1 call", err, calls.Load())
				}
			} else if err != nil || calls.Load() != 2 {
				t.Errorf("default in-flight behavior: second request error = %v, transport calls = %d; want success and 2 calls", err, calls.Load())
			}
			if _, err := serviceListFunc(d.client, "", nil)(t.Context(), meta.ListOptions{}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestInitKubeCacheWatchRateLimit(t *testing.T) {
	for _, tc := range []struct {
		name  string
		qps   float32
		burst int
	}{
		{"defaults", 0, 0},
		{"qps", 1, 0},
		{"burst", 0, 1},
		{"qps and burst", 1, 1},
	} {
		for _, resource := range []string{"services", "serviceimports"} {
			t.Run(tc.name+"/"+resource, func(t *testing.T) {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					w.Header().Set("Content-Type", "application/json")
					if r.URL.Query().Get("watch") == "true" {
						return
					}
					kind, version := "ServiceList", "v1"
					if resource == "serviceimports" {
						kind, version = "ServiceImportList", "multicluster.x-k8s.io/v1alpha1"
					}
					fmt.Fprintf(w, `{"apiVersion":%q,"kind":%q,"items":[]}`, version, kind)
				}))
				defer server.Close()
				k := New([]string{"cluster.local."})
				k.APIServerList = []string{server.URL}
				k.apiQPS, k.apiBurst = tc.qps, tc.burst
				k.opts.multiclusterZones = []string{"clusterset.local."}
				_, stop, err := k.InitKubeCache(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				defer stop()
				d := k.APIConn.(*dnsControl)
				list := serviceListFunc(d.client, "ns", nil)
				watchRequest := serviceWatchFunc(d.client, "ns", nil)
				if resource == "serviceimports" {
					list = serviceImportListFunc(d.mcsClient, "ns", nil)
					watchRequest = serviceImportWatchFunc(d.mcsClient, "ns", nil)
				}
				burst := tc.burst
				if burst == 0 {
					burst = rest.DefaultBurst
				}
				for range burst {
					if _, err := list(t.Context(), meta.ListOptions{}); err != nil {
						t.Fatal(err)
					}
				}
				before := calls.Load()
				ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
				defer cancel()
				w, err := watchRequest(ctx, meta.ListOptions{})
				if w != nil {
					w.Stop()
				}
				if tc.qps > 0 || tc.burst > 0 {
					if err == nil || calls.Load() != before {
						t.Errorf("watch rate limit: error = %v, dispatched = %d; want limiter rejection without dispatch", err, calls.Load()-before)
					} else if !strings.Contains(err.Error(), "would exceed context deadline") && !errors.Is(err, context.DeadlineExceeded) {
						t.Fatal(err)
					}
				} else if err != nil || calls.Load() != before+1 {
					t.Errorf("default watch behavior: error = %v, dispatched = %d; want success and one dispatch", err, calls.Load()-before)
				}
			})
		}
	}
}
