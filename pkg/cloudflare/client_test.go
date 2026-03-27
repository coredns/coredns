package cloudflare

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func boolPtr(b bool) *bool { return &b }
func intPtr(i int) *int    { return &i }

// mockCFServer creates a test server that simulates the Cloudflare API.
func mockCFServer(records map[string][]Record) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		// List records: GET /zones/{id}/dns_records
		if r.Method == http.MethodGet && strings.HasSuffix(path, "/dns_records") {
			parts := strings.Split(path, "/")
			zoneID := parts[len(parts)-2]
			recs := records[zoneID]
			json.NewEncoder(w).Encode(apiResponse[[]Record]{
				Success:    true,
				Result:     recs,
				ResultInfo: &pageInfo{Page: 1, TotalPages: 1, Count: len(recs), TotalCount: len(recs)},
			})
			return
		}

		// Create record: POST /zones/{id}/dns_records
		if r.Method == http.MethodPost && strings.HasSuffix(path, "/dns_records") {
			parts := strings.Split(path, "/")
			zoneID := parts[len(parts)-2]
			var rec Record
			json.NewDecoder(r.Body).Decode(&rec)
			rec.ID = "new-" + rec.Type + "-" + rec.Name
			records[zoneID] = append(records[zoneID], rec)
			json.NewEncoder(w).Encode(apiResponse[Record]{Success: true, Result: rec})
			return
		}

		// Update record: PATCH /zones/{id}/dns_records/{rid}
		if r.Method == http.MethodPatch {
			parts := strings.Split(path, "/")
			zoneID := parts[len(parts)-3]
			recordID := parts[len(parts)-1]
			var patch Record
			json.NewDecoder(r.Body).Decode(&patch)
			for i, rec := range records[zoneID] {
				if rec.ID == recordID {
					if patch.Content != "" {
						records[zoneID][i].Content = patch.Content
					}
					if patch.TTL != 0 {
						records[zoneID][i].TTL = patch.TTL
					}
					json.NewEncoder(w).Encode(apiResponse[Record]{Success: true, Result: records[zoneID][i]})
					return
				}
			}
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(apiResponse[json.RawMessage]{Success: false, Errors: []apiError{{Code: 1, Message: "not found"}}})
			return
		}

		// Delete record: DELETE /zones/{id}/dns_records/{rid}
		if r.Method == http.MethodDelete {
			parts := strings.Split(path, "/")
			zoneID := parts[len(parts)-3]
			recordID := parts[len(parts)-1]
			newRecs := records[zoneID][:0]
			for _, rec := range records[zoneID] {
				if rec.ID != recordID {
					newRecs = append(newRecs, rec)
				}
			}
			records[zoneID] = newRecs
			json.NewEncoder(w).Encode(apiResponse[map[string]string]{Success: true, Result: map[string]string{"id": recordID}})
			return
		}

		w.WriteHeader(404)
	}))
}

func TestSyncCreatesNewRecords(t *testing.T) {
	records := map[string][]Record{"zone1": {}}
	srv := mockCFServer(records)
	defer srv.Close()

	c := NewClient("test-token")
	c.baseURL = srv.URL

	desired := []Record{
		{Type: "A", Name: "www.example.com", Content: "1.2.3.4", TTL: 300},
		{Type: "MX", Name: "example.com", Content: "mail.example.com", TTL: 3600, Priority: intPtr(10)},
	}

	result, err := c.SyncZone(context.Background(), "zone1", desired)
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 2 {
		t.Fatalf("expected 2 created, got %d", result.Created)
	}
	if result.Updated != 0 || result.Deleted != 0 {
		t.Fatalf("expected 0 updated/deleted, got %d/%d", result.Updated, result.Deleted)
	}
	if len(records["zone1"]) != 2 {
		t.Fatalf("expected 2 records in mock, got %d", len(records["zone1"]))
	}
}

func TestSyncUpdatesChangedRecords(t *testing.T) {
	records := map[string][]Record{
		"zone1": {
			{ID: "r1", Type: "A", Name: "www.example.com", Content: "1.2.3.4", TTL: 300},
		},
	}
	srv := mockCFServer(records)
	defer srv.Close()

	c := NewClient("test-token")
	c.baseURL = srv.URL

	desired := []Record{
		{Type: "A", Name: "www.example.com", Content: "5.6.7.8", TTL: 300},
	}

	result, err := c.SyncZone(context.Background(), "zone1", desired)
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated != 1 {
		t.Fatalf("expected 1 updated, got %d", result.Updated)
	}
	if records["zone1"][0].Content != "5.6.7.8" {
		t.Fatalf("expected content 5.6.7.8, got %s", records["zone1"][0].Content)
	}
}

func TestSyncDeletesRemovedRecords(t *testing.T) {
	records := map[string][]Record{
		"zone1": {
			{ID: "r1", Type: "A", Name: "www.example.com", Content: "1.2.3.4", TTL: 300},
			{ID: "r2", Type: "A", Name: "old.example.com", Content: "9.9.9.9", TTL: 300},
		},
	}
	srv := mockCFServer(records)
	defer srv.Close()

	c := NewClient("test-token")
	c.baseURL = srv.URL

	desired := []Record{
		{Type: "A", Name: "www.example.com", Content: "1.2.3.4", TTL: 300},
	}

	result, err := c.SyncZone(context.Background(), "zone1", desired)
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 1 {
		t.Fatalf("expected 1 deleted, got %d", result.Deleted)
	}
	if len(records["zone1"]) != 1 {
		t.Fatalf("expected 1 record remaining, got %d", len(records["zone1"]))
	}
}

func TestSyncPreservesNSAndSOA(t *testing.T) {
	records := map[string][]Record{
		"zone1": {
			{ID: "ns1", Type: "NS", Name: "example.com", Content: "ns1.cloudflare.com", TTL: 86400},
			{ID: "soa1", Type: "SOA", Name: "example.com", Content: "ns1.cloudflare.com admin.example.com", TTL: 3600},
			{ID: "a1", Type: "A", Name: "old.example.com", Content: "1.1.1.1", TTL: 300},
		},
	}
	srv := mockCFServer(records)
	defer srv.Close()

	c := NewClient("test-token")
	c.baseURL = srv.URL

	// Desired has no records — should delete A but preserve NS and SOA.
	result, err := c.SyncZone(context.Background(), "zone1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 1 {
		t.Fatalf("expected 1 deleted (A only), got %d", result.Deleted)
	}
	// NS and SOA should remain.
	if len(records["zone1"]) != 2 {
		t.Fatalf("expected 2 records (NS+SOA), got %d", len(records["zone1"]))
	}
}

func TestSyncHandlesEmptyDesired(t *testing.T) {
	records := map[string][]Record{"zone1": {}}
	srv := mockCFServer(records)
	defer srv.Close()

	c := NewClient("test-token")
	c.baseURL = srv.URL

	result, err := c.SyncZone(context.Background(), "zone1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 0 && result.Updated != 0 && result.Deleted != 0 {
		t.Fatal("expected no operations on empty sync")
	}
}

func TestSyncHandlesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(apiResponse[json.RawMessage]{
			Success: false,
			Errors:  []apiError{{Code: 1000, Message: "internal error"}},
		})
	}))
	defer srv.Close()

	c := NewClient("test-token")
	c.baseURL = srv.URL

	_, err := c.SyncZone(context.Background(), "zone1", []Record{
		{Type: "A", Name: "test.com", Content: "1.1.1.1"},
	})
	if err == nil {
		t.Fatal("expected error on API failure")
	}
}
