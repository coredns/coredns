package sync

import (
	"context"
	"fmt"
	"strings"

	"github.com/coredns/coredns/pkg/cloudflare"
)

// syncToCloudflare syncs records with sync_to_cloudflare=true to Cloudflare.
func (w *Watcher) syncToCloudflare(ctx context.Context, zone DBZone, records []DBRecord) error {
	if w.cfClient == nil {
		return nil
	}
	if !zone.CloudflareZoneID.Valid || zone.CloudflareZoneID.String == "" {
		return nil
	}

	cfZoneID := zone.CloudflareZoneID.String

	// Filter to records that should sync to CF.
	var desired []cloudflare.Record
	for _, r := range records {
		if !r.SyncToCloudflare {
			continue
		}

		// Build FQDN for the record name.
		fqdn := r.Name
		if fqdn == "@" || fqdn == "" {
			fqdn = zone.Name
		} else if !strings.Contains(fqdn, ".") {
			fqdn = fqdn + "." + zone.Name
		}

		cfRec := cloudflare.Record{
			Type:    r.Type,
			Name:    fqdn,
			Content: r.Content,
			TTL:     r.TTL,
			Proxied: &r.Proxied,
		}
		if r.Priority.Valid {
			p := int(r.Priority.Int32)
			cfRec.Priority = &p
		}
		desired = append(desired, cfRec)
	}

	result, err := w.cfClient.SyncZone(ctx, cfZoneID, desired)
	if err != nil {
		return fmt.Errorf("cf sync zone %s: %w", zone.Name, err)
	}

	w.logger.Info("cloudflare sync complete",
		"zone", zone.Name,
		"created", result.Created,
		"updated", result.Updated,
		"deleted", result.Deleted,
		"errors", len(result.Errors),
	)

	if len(result.Errors) > 0 {
		for _, e := range result.Errors {
			w.logger.Error("cf sync error", "zone", zone.Name, "error", e)
		}
	}

	return nil
}
