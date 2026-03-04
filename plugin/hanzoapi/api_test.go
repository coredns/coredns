package hanzoapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func newTestServer() (*http.ServeMux, *Store) {
	store := NewStore()
	mux := http.NewServeMux()
	registerRoutes(mux, store)
	return mux, store
}

func TestHealthEndpoint(t *testing.T) {
	mux, _ := newTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Fatalf("expected ok, got %s", body["status"])
	}
}

func TestAuthRequired(t *testing.T) {
	os.Setenv("HANZO_DNS_API_KEY", "test-secret-key")
	defer os.Unsetenv("HANZO_DNS_API_KEY")

	mux, _ := newTestServer()

	// No auth header.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/zones", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", w.Code)
	}

	// Wrong token.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/zones", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with wrong token, got %d", w.Code)
	}

	// Correct token.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/zones", nil)
	req.Header.Set("Authorization", "Bearer test-secret-key")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with correct token, got %d", w.Code)
	}
}

func TestNoAuthWhenKeyUnset(t *testing.T) {
	os.Unsetenv("HANZO_DNS_API_KEY")

	mux, _ := newTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/zones", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when no key configured, got %d", w.Code)
	}
}

func TestZoneCRUD(t *testing.T) {
	os.Unsetenv("HANZO_DNS_API_KEY")
	mux, _ := newTestServer()

	// Create zone.
	body := `{"zone":"example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/zones", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create zone: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var zone Zone
	json.NewDecoder(w.Body).Decode(&zone)
	if zone.Name != "example.com." {
		t.Fatalf("expected example.com., got %s", zone.Name)
	}

	// List zones.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/zones", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list zones: expected 200, got %d", w.Code)
	}

	var listResp struct {
		Zones []Zone `json:"zones"`
		Total int    `json:"total"`
	}
	json.NewDecoder(w.Body).Decode(&listResp)
	if listResp.Total != 1 {
		t.Fatalf("expected 1 zone, got %d", listResp.Total)
	}

	// Get zone.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/zones/example.com", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get zone: expected 200, got %d", w.Code)
	}

	// Duplicate zone.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/zones", bytes.NewBufferString(body))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("dup zone: expected 409, got %d", w.Code)
	}

	// Delete zone.
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/zones/example.com", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete zone: expected 200, got %d", w.Code)
	}

	// Delete again -- 404.
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/zones/example.com", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("delete missing zone: expected 404, got %d", w.Code)
	}
}

func TestRecordCRUD(t *testing.T) {
	os.Unsetenv("HANZO_DNS_API_KEY")
	mux, _ := newTestServer()

	// Create zone first.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/zones", bytes.NewBufferString(`{"zone":"example.com"}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create zone: expected 201, got %d", w.Code)
	}

	// Create record.
	recBody := `{"name":"www","type":"A","content":"1.2.3.4","ttl":300}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/zones/example.com/records", bytes.NewBufferString(recBody))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create record: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var rec Record
	json.NewDecoder(w.Body).Decode(&rec)
	if rec.Name != "www" || rec.Content != "1.2.3.4" {
		t.Fatalf("unexpected record: %+v", rec)
	}

	// List records.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/zones/example.com/records", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list records: expected 200, got %d", w.Code)
	}

	// Get record.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/zones/example.com/records/"+rec.ID, nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get record: expected 200, got %d", w.Code)
	}

	// Update record (PATCH).
	patchBody := `{"content":"5.6.7.8"}`
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/zones/example.com/records/"+rec.ID, bytes.NewBufferString(patchBody))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update record: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated Record
	json.NewDecoder(w.Body).Decode(&updated)
	if updated.Content != "5.6.7.8" {
		t.Fatalf("expected 5.6.7.8, got %s", updated.Content)
	}

	// Delete record.
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/zones/example.com/records/"+rec.ID, nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete record: expected 200, got %d", w.Code)
	}

	// Delete again -- 404.
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/zones/example.com/records/"+rec.ID, nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("delete missing record: expected 404, got %d", w.Code)
	}
}

func TestRecordValidation(t *testing.T) {
	os.Unsetenv("HANZO_DNS_API_KEY")
	mux, _ := newTestServer()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/zones", bytes.NewBufferString(`{"zone":"example.com"}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	tests := []struct {
		name string
		body string
		code int
	}{
		{"missing name", `{"type":"A","content":"1.2.3.4"}`, http.StatusBadRequest},
		{"missing type", `{"name":"www","content":"1.2.3.4"}`, http.StatusBadRequest},
		{"missing content", `{"name":"www","type":"A"}`, http.StatusBadRequest},
		{"invalid json", `{bad}`, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/zones/example.com/records", bytes.NewBufferString(tt.body))
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			if w.Code != tt.code {
				t.Fatalf("expected %d, got %d: %s", tt.code, w.Code, w.Body.String())
			}
		})
	}
}

func TestRecordInNonexistentZone(t *testing.T) {
	os.Unsetenv("HANZO_DNS_API_KEY")
	mux, _ := newTestServer()

	// List records for nonexistent zone.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/zones/nope.com/records", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}

	// Create record in nonexistent zone.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/zones/nope.com/records", bytes.NewBufferString(`{"name":"www","type":"A","content":"1.2.3.4"}`))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
