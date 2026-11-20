package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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
