package hanzodns

import (
	"encoding/json"
	"net/http"
)

// SyncRequest is the top-level request body for POST /api/v1/sync.
type SyncRequest struct {
	Zones []SyncZoneRequest `json:"zones"`
}

// handleSync handles POST /api/v1/sync — bulk pushes zones + records.
func handleSync(w http.ResponseWriter, r *http.Request, store *Store) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST is allowed")
		return
	}

	var body SyncRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	if len(body.Zones) == 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "at least one zone is required")
		return
	}

	results := make([]SyncResponse, 0, len(body.Zones))
	for _, zr := range body.Zones {
		if zr.Zone == "" {
			writeError(w, http.StatusBadRequest, "bad_request", "zone name is required in each entry")
			return
		}
		resp, err := store.BulkSync(zr)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		results = append(results, *resp)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"synced": results,
		"total":  len(results),
	})
}
