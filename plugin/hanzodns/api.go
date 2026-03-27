package hanzodns

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
)

// apiError is the standard error response body.
type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, apiError{Code: code, Message: msg})
}

// authMiddleware validates the Bearer token against HANZO_DNS_API_KEY.
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := os.Getenv("HANZO_DNS_API_KEY")
		if key == "" {
			// No key configured -- allow all requests (development mode).
			next.ServeHTTP(w, r)
			return
		}

		auth := r.Header.Get("Authorization")
		if auth == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing Authorization header")
			return
		}

		parts := strings.SplitN(auth, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") || parts[1] != key {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid bearer token")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// registerRoutes wires up all REST API handlers on the given mux.
func registerRoutes(mux *http.ServeMux, store *Store) {
	// Health -- no auth required.
	mux.HandleFunc("GET /api/v1/health", handleHealth)

	// All other routes require auth.
	authed := authMiddleware(apiRouter(store))
	mux.Handle("/api/v1/zones", authed)
	mux.Handle("/api/v1/zones/", authed)

	// Bulk sync endpoint -- requires auth.
	mux.Handle("/api/v1/sync", authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleSync(w, r, store)
	})))
}

// apiRouter returns a handler that dispatches /api/v1/zones* requests.
func apiRouter(store *Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v1")
		path = strings.TrimSuffix(path, "/")
		parts := strings.Split(strings.TrimPrefix(path, "/"), "/")

		// /zones
		if len(parts) == 1 && parts[0] == "zones" {
			switch r.Method {
			case http.MethodGet:
				handleListZones(w, r, store)
			case http.MethodPost:
				handleCreateZone(w, r, store)
			default:
				writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
			}
			return
		}

		// /zones/{zone}
		if len(parts) == 2 && parts[0] == "zones" {
			zoneName := parts[1]
			switch r.Method {
			case http.MethodGet:
				handleGetZone(w, r, store, zoneName)
			case http.MethodDelete:
				handleDeleteZone(w, r, store, zoneName)
			default:
				writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
			}
			return
		}

		// /zones/{zone}/records
		if len(parts) == 3 && parts[0] == "zones" && parts[2] == "records" {
			zoneName := parts[1]
			switch r.Method {
			case http.MethodGet:
				handleListRecords(w, r, store, zoneName)
			case http.MethodPost:
				handleCreateRecord(w, r, store, zoneName)
			default:
				writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
			}
			return
		}

		// /zones/{zone}/records/{id}
		if len(parts) == 4 && parts[0] == "zones" && parts[2] == "records" {
			zoneName := parts[1]
			recordID := parts[3]
			switch r.Method {
			case http.MethodGet:
				handleGetRecord(w, r, store, zoneName, recordID)
			case http.MethodPatch:
				handleUpdateRecord(w, r, store, zoneName, recordID)
			case http.MethodPut:
				handleUpdateRecord(w, r, store, zoneName, recordID)
			case http.MethodDelete:
				handleDeleteRecord(w, r, store, zoneName, recordID)
			default:
				writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
			}
			return
		}

		writeError(w, http.StatusNotFound, "not_found", "endpoint not found")
	})
}

// --- Handlers ---

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleListZones(w http.ResponseWriter, r *http.Request, store *Store) {
	zones := store.ListZones()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"zones": zones,
		"total": len(zones),
	})
}

func handleGetZone(w http.ResponseWriter, r *http.Request, store *Store, zoneName string) {
	z := store.GetZone(zoneName)
	if z == nil {
		writeError(w, http.StatusNotFound, "not_found", "zone not found")
		return
	}
	writeJSON(w, http.StatusOK, z)
}

func handleCreateZone(w http.ResponseWriter, r *http.Request, store *Store) {
	var body struct {
		Zone string `json:"zone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if body.Zone == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "zone name is required")
		return
	}

	z, err := store.CreateZone(body.Zone)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			writeError(w, http.StatusConflict, "conflict", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, z)
}

func handleDeleteZone(w http.ResponseWriter, r *http.Request, store *Store, zoneName string) {
	if err := store.DeleteZone(zoneName); err != nil {
		writeError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{})
}

func handleListRecords(w http.ResponseWriter, r *http.Request, store *Store, zoneName string) {
	records, err := store.ListRecords(zoneName)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"records": records,
		"total":   len(records),
	})
}

func handleGetRecord(w http.ResponseWriter, r *http.Request, store *Store, zoneName, recordID string) {
	rec, err := store.GetRecord(zoneName, recordID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

func handleCreateRecord(w http.ResponseWriter, r *http.Request, store *Store, zoneName string) {
	var body struct {
		Name     string     `json:"name"`
		Type     RecordType `json:"type"`
		TTL      uint32     `json:"ttl"`
		Content  string     `json:"content"`
		Priority uint16     `json:"priority"`
		Proxied  bool       `json:"proxied"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "name is required")
		return
	}
	if body.Type == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "type is required")
		return
	}
	if body.Content == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "content is required")
		return
	}
	if body.TTL == 0 {
		body.TTL = 300
	}

	rec, err := store.CreateRecord(zoneName, body.Name, body.Type, body.TTL, body.Content, body.Priority, body.Proxied)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, "not_found", err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, rec)
}

func handleUpdateRecord(w http.ResponseWriter, r *http.Request, store *Store, zoneName, recordID string) {
	var patch RecordPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	rec, err := store.UpdateRecord(zoneName, recordID, patch)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, "not_found", err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

func handleDeleteRecord(w http.ResponseWriter, r *http.Request, store *Store, zoneName, recordID string) {
	if err := store.DeleteRecord(zoneName, recordID); err != nil {
		writeError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{})
}
