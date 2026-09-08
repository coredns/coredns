package azure

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/file"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	publicdns "github.com/Azure/azure-sdk-for-go/profiles/latest/dns/mgmt/dns"
	privatedns "github.com/Azure/azure-sdk-for-go/profiles/latest/privatedns/mgmt/privatedns"
	"github.com/miekg/dns"
)

type azureAPIHandler func(http.ResponseWriter, *http.Request) bool

// Use the real SDK's HTTP decoding, pagination and cancellation paths.
func newTestAzure(t *testing.T, private bool, handle azureAPIHandler) (*Azure, *atomic.Int32) {
	t.Helper()
	requests := new(atomic.Int32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if handle != nil && handle(w, r) {
			return
		}
		name := "healthy.example"
		if strings.Contains(r.URL.Path, "/missing.example/") {
			name = "missing.example"
		} else if !strings.Contains(r.URL.Path, "/healthy.example/") {
			t.Errorf("unexpected API path %q", r.URL.Path)
			http.Error(w, "unexpected API path", http.StatusBadRequest)
			return
		}
		writeAzureRecords(t, w, private, name, "", true, true)
	}))
	t.Cleanup(server.Close)
	client := server.Client()
	client.Timeout = 5 * time.Second
	publicClient := publicdns.NewRecordSetsClientWithBaseURI(server.URL, "test-subscription")
	privateClient := privatedns.NewRecordSetsClientWithBaseURI(server.URL, "test-subscription")
	publicClient.Sender, privateClient.Sender = client, client
	// Zero skips the request entirely in the legacy registration wrapper.
	publicClient.RetryAttempts, privateClient.RetryAttempts = 1, 1
	publicClient.RetryDuration, privateClient.RetryDuration = time.Millisecond, time.Millisecond
	access := "public"
	if private {
		access = "private"
	}
	h, err := New(context.Background(), publicClient, privateClient,
		map[string][]string{"rg": {"healthy.example", "missing.example"}},
		map[string]string{"rghealthy.example": access, "rgmissing.example": access})
	if err != nil {
		t.Fatal(err)
	}
	return h, requests
}

func writeAzureRecords(t *testing.T, w http.ResponseWriter, private bool, name, next string, soa, address bool) {
	t.Helper()
	ttlKey, soaKey, aKey, minimumKey := "TTL", "SOARecord", "ARecords", "minimumTTL"
	if private {
		ttlKey, soaKey, aKey, minimumKey = "ttl", "soaRecord", "aRecords", "minimumTtl"
	}
	values := make([]any, 0, 2)
	if soa {
		values = append(values, map[string]any{"name": "@", "properties": map[string]any{
			"fqdn": name + ".", ttlKey: 60, soaKey: map[string]any{
				"host": "ns." + name + ".", "email": "hostmaster." + name + ".",
				"serialNumber": 1, "refreshTime": 3600, "retryTime": 300,
				"expireTime": 86400, minimumKey: 60,
			},
		}})
	}
	if address {
		values = append(values, map[string]any{"name": "www", "properties": map[string]any{
			"fqdn": "www." + name + ".", ttlKey: 60,
			aKey: []any{map[string]string{"ipv4Address": "192.0.2.10"}},
		}})
	}
	body := map[string]any{"value": values}
	if next != "" {
		body["nextLink"] = next
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(body); err != nil {
		t.Errorf("encode records: %v", err)
	}
}

func writeAzureError(t *testing.T, w http.ResponseWriter, status int) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"code": http.StatusText(status), "message": "zone unavailable in test"},
	}); err != nil {
		t.Errorf("encode error: %v", err)
	}
}

func azureQuery(h *Azure, name string) (int, *dns.Msg, error) {
	r := new(dns.Msg)
	r.SetQuestion("www."+name+".", dns.TypeA)
	w := dnstest.NewRecorder(&test.ResponseWriter{})
	code, err := h.ServeDNS(context.Background(), w, r)
	return code, w.Msg, err
}

func hasAzureAnswer(h *Azure, name string) bool {
	code, m, err := azureQuery(h, name)
	if err != nil || code != dns.RcodeSuccess || m == nil || len(m.Answer) != 1 {
		return false
	}
	a, ok := m.Answer[0].(*dns.A)
	return ok && a.A.String() == "192.0.2.10"
}

func waitAzure(t *testing.T, check func() bool) {
	t.Helper()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for !check() {
		select {
		case <-timeout.C:
			t.Fatal("timed out waiting for Azure synchronization")
		case <-tick.C:
		}
	}
}

func testAzureModes(t *testing.T, test func(*testing.T, bool)) {
	t.Helper()
	t.Run("private", func(t *testing.T) { test(t, true) })
	t.Run("public", func(t *testing.T) { test(t, false) })
}

func TestNewDoesNotContactAzure(t *testing.T) {
	testAzureModes(t, func(t *testing.T, private bool) {
		t.Helper()
		h, requests := newTestAzure(t, private, func(w http.ResponseWriter, _ *http.Request) bool {
			writeAzureError(t, w, http.StatusNotFound)
			return true
		})
		if requests.Load() != 0 || len(h.zoneNames) != 2 {
			t.Fatalf("New must configure both zones without HTTP requests: requests=%d zones=%v", requests.Load(), h.zoneNames)
		}
	})
}

func TestRunWithUnavailableZone(t *testing.T) {
	testAzureModes(t, func(t *testing.T, private bool) {
		t.Helper()
		h, _ := newTestAzure(t, private, func(w http.ResponseWriter, r *http.Request) bool {
			if strings.Contains(r.URL.Path, "/missing.example/") {
				writeAzureError(t, w, http.StatusNotFound)
				return true
			}
			return false
		})
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(func() { cancel(); h.updates.Wait() })
		if err := h.Run(ctx); err != nil {
			t.Fatal(err)
		}
		waitAzure(t, func() bool { return hasAzureAnswer(h, "healthy.example") })
		if code, m, err := azureQuery(h, "missing.example"); code != dns.RcodeServerFailure || m != nil || err != nil {
			t.Fatalf("unloaded zone: code=%d msg=%v err=%v", code, m, err)
		}
	})
}

func TestRunDoesNotWaitForAzure(t *testing.T) {
	testAzureModes(t, func(t *testing.T, private bool) {
		t.Helper()
		entered := make(chan struct{})
		var once sync.Once
		h, requests := newTestAzure(t, private, func(_ http.ResponseWriter, r *http.Request) bool {
			once.Do(func() { close(entered) })
			<-r.Context().Done()
			return true
		})
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(func() { cancel(); h.updates.Wait() })
		started := make(chan error, 1)
		go func() { started <- h.Run(ctx) }()
		select {
		case err := <-started:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Run blocked on Azure")
		}
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("background sync never contacted Azure")
		}
		cancel()
		stopped := make(chan struct{})
		go func() { h.updates.Wait(); close(stopped) }()
		select {
		case <-stopped:
		case <-time.After(5 * time.Second):
			t.Fatal("cancellation did not stop the blocked synchronization")
		}
		if requests.Load() != 1 {
			t.Fatalf("canceled sync contacted another zone: %d requests", requests.Load())
		}
	})
}

func TestZoneRecovery(t *testing.T) {
	testAzureModes(t, func(t *testing.T, private bool) {
		t.Helper()
		var missing atomic.Bool
		missing.Store(true)
		var failures atomic.Int32
		h, _ := newTestAzure(t, private, func(w http.ResponseWriter, r *http.Request) bool {
			if strings.Contains(r.URL.Path, "/missing.example/") && missing.Load() {
				failures.Add(1)
				writeAzureError(t, w, http.StatusNotFound)
				return true
			}
			return false
		})
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { defer close(done); h.run(ctx, 5*time.Millisecond) }()
		t.Cleanup(func() { cancel(); <-done })
		waitAzure(t, func() bool { return failures.Load() > 0 && hasAzureAnswer(h, "healthy.example") })
		missing.Store(false)
		waitAzure(t, func() bool { return hasAzureAnswer(h, "missing.example") })
	})
}

func TestFailedUpdatePreservesZone(t *testing.T) {
	testAzureModes(t, func(t *testing.T, private bool) {
		t.Helper()
		for _, status := range []int{http.StatusNotFound, http.StatusForbidden, http.StatusServiceUnavailable} {
			t.Run(http.StatusText(status), func(t *testing.T) {
				var fail atomic.Bool
				h, _ := newTestAzure(t, private, func(w http.ResponseWriter, r *http.Request) bool {
					if fail.Load() && strings.Contains(r.URL.Path, "/missing.example/") {
						writeAzureError(t, w, status)
						return true
					}
					return false
				})
				if err := h.updateZones(context.Background()); err != nil {
					t.Fatal(err)
				}
				old := h.zones["missing.example."][0].z
				healthy := h.zones["healthy.example."][0].z
				fail.Store(true)
				if err := h.updateZones(context.Background()); err == nil {
					t.Fatal("expected update error")
				}
				if h.zones["missing.example."][0].z != old || !hasAzureAnswer(h, "missing.example") || !hasAzureAnswer(h, "healthy.example") {
					t.Fatal("failed refresh lost valid zone data")
				}
				if h.zones["healthy.example."][0].z == healthy {
					t.Fatal("failed zone prevented healthy zone refresh")
				}
			})
		}
	})
}

func TestZonePagination(t *testing.T) {
	testAzureModes(t, func(t *testing.T, private bool) {
		t.Helper()
		for _, status := range []int{http.StatusOK, http.StatusNotFound, http.StatusServiceUnavailable} {
			t.Run(http.StatusText(status), func(t *testing.T) {
				var paginated atomic.Bool
				var pageRequests atomic.Int32
				h, _ := newTestAzure(t, private, func(w http.ResponseWriter, r *http.Request) bool {
					if !paginated.Load() || !strings.Contains(r.URL.Path, "/missing.example/") {
						return false
					}
					if r.URL.Query().Get("page") == "2" {
						// Bound a regression that retries the unchanged previous page forever.
						if pageRequests.Add(1) <= 4 && status != http.StatusOK {
							writeAzureError(t, w, status)
						} else {
							writeAzureRecords(t, w, private, "missing.example", "", false, true)
						}
					} else {
						next := "http://" + r.Host + r.URL.Path + "?page=2"
						writeAzureRecords(t, w, private, "missing.example", next, true, false)
					}
					return true
				})
				if err := h.updateZones(context.Background()); err != nil {
					t.Fatal(err)
				}
				old := h.zones["missing.example."][0].z
				healthy := h.zones["healthy.example."][0].z
				paginated.Store(true)
				err := h.updateZones(context.Background())
				if (err == nil) != (status == http.StatusOK) {
					t.Fatalf("pagination status %d: err=%v", status, err)
				}
				if pageRequests.Load() == 0 || pageRequests.Load() > 2 {
					t.Fatalf("unexpected next-page attempts: %d", pageRequests.Load())
				}
				if status != http.StatusOK && h.zones["missing.example."][0].z != old {
					t.Fatal("published a partial zone after a pagination error")
				}
				if !hasAzureAnswer(h, "missing.example") || !hasAzureAnswer(h, "healthy.example") {
					t.Fatal("lost an A answer after pagination")
				}
				if h.zones["healthy.example."][0].z == healthy {
					t.Fatal("pagination failure prevented healthy zone refresh")
				}
			})
		}
	})
}

func TestCanceledPaginationPreservesZone(t *testing.T) {
	testAzureModes(t, func(t *testing.T, private bool) {
		t.Helper()
		var paginated atomic.Bool
		entered := make(chan struct{})
		var once sync.Once
		h, _ := newTestAzure(t, private, func(w http.ResponseWriter, r *http.Request) bool {
			if !paginated.Load() || !strings.Contains(r.URL.Path, "/missing.example/") {
				return false
			}
			if r.URL.Query().Get("page") == "2" {
				once.Do(func() { close(entered) })
				<-r.Context().Done()
			} else {
				next := "http://" + r.Host + r.URL.Path + "?page=2"
				writeAzureRecords(t, w, private, "missing.example", next, true, false)
			}
			return true
		})
		if err := h.updateZones(context.Background()); err != nil {
			t.Fatal(err)
		}
		old := h.zones["missing.example."][0].z
		paginated.Store(true)
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { defer close(done); done <- h.updateZones(ctx) }()
		t.Cleanup(func() { cancel(); <-done })
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("second page was not requested")
		}
		cancel()
		select {
		case err := <-done:
			if err == nil {
				t.Fatal("expected canceled pagination error")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("pagination did not stop on cancellation")
		}
		if h.zones["missing.example."][0].z != old || !hasAzureAnswer(h, "missing.example") {
			t.Fatal("canceled pagination replaced valid zone data")
		}
	})
}

func TestIncompleteZoneIsNotPublished(t *testing.T) {
	testAzureModes(t, func(t *testing.T, private bool) {
		t.Helper()
		h, _ := newTestAzure(t, private, func(w http.ResponseWriter, r *http.Request) bool {
			if strings.Contains(r.URL.Path, "/missing.example/") {
				writeAzureRecords(t, w, private, "missing.example", "", false, true)
				return true
			}
			return false
		})
		old := h.zones["missing.example."][0].z
		if err := h.updateZones(context.Background()); err == nil {
			t.Fatal("expected missing SOA error")
		}
		if h.zones["missing.example."][0].z != old || !hasAzureAnswer(h, "healthy.example") {
			t.Fatal("incomplete zone was published or prevented healthy zone update")
		}
	})
}

func TestUnloadedZoneFallthrough(t *testing.T) {
	h, _ := newTestAzure(t, true, nil)
	var calls int
	h.Next = test.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		calls++
		m := new(dns.Msg)
		m.SetRcode(r, dns.RcodeRefused)
		return dns.RcodeSuccess, w.WriteMsg(m)
	})
	h.Fall.SetZonesFromArgs([]string{"missing.example"})
	if code, m, err := azureQuery(h, "healthy.example"); code != dns.RcodeServerFailure || m != nil || err != nil || calls != 0 {
		t.Fatalf("out-of-scope fallthrough: code=%d msg=%v err=%v calls=%d", code, m, err, calls)
	}
	if code, m, err := azureQuery(h, "missing.example"); code != dns.RcodeSuccess || m == nil || m.Rcode != dns.RcodeRefused || err != nil || calls != 1 {
		t.Fatalf("explicit fallthrough: code=%d msg=%v err=%v calls=%d", code, m, err, calls)
	}
	if _, _, err := azureQuery(h, "unrelated.example"); err != nil || calls != 2 {
		t.Fatalf("unrelated query did not reach next plugin: err=%v calls=%d", err, calls)
	}
}

func TestSameZoneInMultipleResourceGroups(t *testing.T) {
	testAzureModes(t, func(t *testing.T, private bool) {
		t.Helper()
		h, _ := newTestAzure(t, private, func(w http.ResponseWriter, r *http.Request) bool {
			if strings.Contains(r.URL.Path, "/missing-rg/") {
				writeAzureError(t, w, http.StatusNotFound)
				return true
			}
			return false
		})
		h.zones["healthy.example."] = append([]*zone{{id: "missing-rg", zone: "healthy.example", private: private, z: file.NewZone("healthy.example.", "")}}, h.zones["healthy.example."]...)
		if err := h.updateZones(context.Background()); err == nil {
			t.Fatal("expected missing resource group error")
		}
		if !hasAzureAnswer(h, "healthy.example") {
			t.Fatal("missing resource group masked the healthy copy of the zone")
		}
	})
}

func TestConcurrentZoneUpdates(t *testing.T) {
	h, _ := newTestAzure(t, true, nil)
	if err := h.updateZones(context.Background()); err != nil {
		t.Fatal(err)
	}
	var readers sync.WaitGroup
	for range 4 {
		readers.Go(func() {
			for range 100 {
				if !hasAzureAnswer(h, "healthy.example") {
					t.Error("concurrent query lost its answer")
					return
				}
			}
		})
	}
	for range 20 {
		if err := h.updateZones(context.Background()); err != nil {
			t.Error(err)
		}
	}
	readers.Wait()
}

func TestRunWithCanceledContext(t *testing.T) {
	h, requests := newTestAzure(t, true, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := h.Run(ctx); err != nil {
		t.Fatal(err)
	}
	h.updates.Wait()
	if requests.Load() != 0 {
		t.Fatalf("canceled synchronization issued %d requests", requests.Load())
	}
}
