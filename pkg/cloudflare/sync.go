package cloudflare

import (
	"context"
	"fmt"
	"log/slog"
)

// protectedTypes are never deleted from Cloudflare by sync.
var protectedTypes = map[string]bool{
	"NS":  true,
	"SOA": true,
}

// SyncZone takes a desired set of records and reconciles the zone to match.
// It diffs against current CF state: creates missing, updates changed, deletes extra.
// NS and SOA records are never deleted.
func (c *Client) SyncZone(ctx context.Context, zoneID string, desired []Record) (*SyncResult, error) {
	current, err := c.ListRecords(ctx, zoneID)
	if err != nil {
		return nil, fmt.Errorf("list current records: %w", err)
	}

	result := &SyncResult{}

	// Index current records by (type, name).
	type key struct{ typ, name string }
	currentByKey := make(map[key]Record, len(current))
	for _, r := range current {
		k := key{r.Type, r.Name}
		currentByKey[k] = r
	}

	// Index desired records by (type, name).
	desiredByKey := make(map[key]Record, len(desired))
	for _, r := range desired {
		k := key{r.Type, r.Name}
		desiredByKey[k] = r
	}

	// Create or update records.
	for k, want := range desiredByKey {
		have, exists := currentByKey[k]
		if !exists {
			// Create.
			slog.Info("cf sync: creating record", "type", want.Type, "name", want.Name)
			_, err := c.CreateRecord(ctx, zoneID, want)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("create %s %s: %w", want.Type, want.Name, err))
				continue
			}
			result.Created++
		} else if recordChanged(have, want) {
			// Update.
			slog.Info("cf sync: updating record", "type", want.Type, "name", want.Name)
			want.ID = have.ID
			_, err := c.UpdateRecord(ctx, zoneID, have.ID, want)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("update %s %s: %w", want.Type, want.Name, err))
				continue
			}
			result.Updated++
		}
	}

	// Delete records not in desired set (skip NS/SOA).
	for k, have := range currentByKey {
		if protectedTypes[k.typ] {
			continue
		}
		if _, wanted := desiredByKey[k]; !wanted {
			slog.Info("cf sync: deleting record", "type", have.Type, "name", have.Name)
			if err := c.DeleteRecord(ctx, zoneID, have.ID); err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("delete %s %s: %w", have.Type, have.Name, err))
				continue
			}
			result.Deleted++
		}
	}

	return result, nil
}

// recordChanged returns true if the desired record differs from current.
func recordChanged(current, desired Record) bool {
	if current.Content != desired.Content {
		return true
	}
	if current.TTL != desired.TTL && desired.TTL != 0 {
		return true
	}
	if desired.Proxied != nil && (current.Proxied == nil || *current.Proxied != *desired.Proxied) {
		return true
	}
	if desired.Priority != nil && (current.Priority == nil || *current.Priority != *desired.Priority) {
		return true
	}
	return false
}
