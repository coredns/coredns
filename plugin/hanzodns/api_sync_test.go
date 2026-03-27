package hanzodns

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBulkSync(t *testing.T) {
	store := NewStore()

	// Sync a zone with records.
	resp, err := store.BulkSync(SyncZoneRequest{
		Zone: "example.com",
		Records: []SyncRecord{
			{Name: "www", Type: TypeA, Content: "1.2.3.4", TTL: 300},
			{Name: "@", Type: TypeMX, Content: "mail.example.com", TTL: 3600, Priority: 10},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "example.com.", resp.Zone)
	assert.Equal(t, 2, resp.RecordCount)
	assert.Equal(t, 2, resp.Created)
	assert.Equal(t, 0, resp.Deleted)

	// Verify records are in store.
	records, err := store.ListRecords("example.com")
	require.NoError(t, err)
	assert.Len(t, records, 2)

	// Re-sync with different records — old ones should be replaced.
	resp, err = store.BulkSync(SyncZoneRequest{
		Zone: "example.com",
		Records: []SyncRecord{
			{Name: "api", Type: TypeA, Content: "5.6.7.8", TTL: 60},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, resp.RecordCount)
	assert.Equal(t, 1, resp.Created)
	assert.Equal(t, 2, resp.Deleted)

	// Verify old records are gone.
	records, err = store.ListRecords("example.com")
	require.NoError(t, err)
	assert.Len(t, records, 1)
	assert.Equal(t, "api", records[0].Name)
}

func TestBulkSyncCreatesZone(t *testing.T) {
	store := NewStore()

	// Zone should not exist yet.
	assert.Nil(t, store.GetZone("newzone.io"))

	resp, err := store.BulkSync(SyncZoneRequest{
		Zone: "newzone.io",
		Records: []SyncRecord{
			{Name: "@", Type: TypeA, Content: "10.0.0.1"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, resp.RecordCount)

	// Zone should now exist.
	z := store.GetZone("newzone.io")
	require.NotNil(t, z)
	assert.Equal(t, "active", z.Status)
}

func TestBulkSyncSkipsInvalidTypes(t *testing.T) {
	store := NewStore()

	resp, err := store.BulkSync(SyncZoneRequest{
		Zone: "test.com",
		Records: []SyncRecord{
			{Name: "www", Type: TypeA, Content: "1.2.3.4"},
			{Name: "bad", Type: RecordType("INVALID"), Content: "x"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, resp.RecordCount) // only the valid one
}

func TestSyncEndpoint(t *testing.T) {
	store := NewStore()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/sync", func(w http.ResponseWriter, r *http.Request) {
		handleSync(w, r, store)
	})

	body := SyncRequest{
		Zones: []SyncZoneRequest{
			{
				Zone: "test.com",
				Records: []SyncRecord{
					{Name: "@", Type: TypeA, Content: "10.0.0.1", TTL: 300},
					{Name: "www", Type: TypeCNAME, Content: "test.com", TTL: 300},
				},
			},
			{
				Zone: "other.com",
				Records: []SyncRecord{
					{Name: "@", Type: TypeA, Content: "10.0.0.2", TTL: 300},
				},
			},
		},
	}

	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, float64(2), result["total"])

	// Verify both zones exist.
	zones := store.ListZones()
	assert.Len(t, zones, 2)
}

func TestSyncEndpointEmptyZones(t *testing.T) {
	store := NewStore()

	body := SyncRequest{Zones: []SyncZoneRequest{}}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync", bytes.NewReader(b))
	w := httptest.NewRecorder()

	handleSync(w, req, store)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSyncEndpointMethodNotAllowed(t *testing.T) {
	store := NewStore()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sync", nil)
	w := httptest.NewRecorder()

	handleSync(w, req, store)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}
