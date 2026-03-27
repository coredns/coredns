package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// syncRequest matches the hanzodns POST /api/v1/sync payload.
type syncRequest struct {
	Zones []syncZone `json:"zones"`
}

type syncZone struct {
	Zone    string       `json:"zone"`
	Records []syncRecord `json:"records"`
}

type syncRecord struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Content  string `json:"content"`
	TTL      uint32 `json:"ttl"`
	Priority uint16 `json:"priority,omitempty"`
	Proxied  bool   `json:"proxied"`
}

// syncToCoreDNS pushes zone records to CoreDNS via POST /api/v1/sync.
func (w *Watcher) syncToCoreDNS(ctx context.Context, zones []zoneWithRecords) error {
	if len(zones) == 0 {
		return nil
	}

	payload := syncRequest{
		Zones: make([]syncZone, 0, len(zones)),
	}

	for _, zwr := range zones {
		zoneName := zwr.Zone.Name
		if !strings.HasSuffix(zoneName, ".") {
			zoneName += "."
		}

		sz := syncZone{
			Zone:    zoneName,
			Records: make([]syncRecord, 0, len(zwr.Records)),
		}

		for _, r := range zwr.Records {
			sr := syncRecord{
				ID:      r.ID,
				Name:    r.Name,
				Type:    r.Type,
				Content: r.Content,
				TTL:     uint32(r.TTL),
				Proxied: r.Proxied,
			}
			if r.Priority.Valid {
				sr.Priority = uint16(r.Priority.Int32)
			}
			sz.Records = append(sz.Records, sr)
		}

		payload.Zones = append(payload.Zones, sz)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal sync payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.corednsEndpoint+"/api/v1/sync", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create sync request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if w.corednsAPIKey != "" {
		req.Header.Set("Authorization", "Bearer "+w.corednsAPIKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("sync to coredns: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("coredns sync returned %d", resp.StatusCode)
	}

	return nil
}
