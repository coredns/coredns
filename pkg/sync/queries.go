package sync

import (
	"context"
	"time"
)

// fetchZones returns all active zones.
func (w *Watcher) fetchZones(ctx context.Context) ([]DBZone, error) {
	rows, err := w.db.QueryContext(ctx, `
		SELECT id, org_id, name, status, cloudflare_zone_id, updated_at
		FROM dns_zones
		WHERE status = 'active'
		ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var zones []DBZone
	for rows.Next() {
		var z DBZone
		if err := rows.Scan(&z.ID, &z.OrgID, &z.Name, &z.Status, &z.CloudflareZoneID, &z.UpdatedAt); err != nil {
			return nil, err
		}
		zones = append(zones, z)
	}
	return zones, rows.Err()
}

// fetchChangedZones returns zones updated since the given time.
func (w *Watcher) fetchChangedZones(ctx context.Context, since time.Time) ([]DBZone, error) {
	rows, err := w.db.QueryContext(ctx, `
		SELECT id, org_id, name, status, cloudflare_zone_id, updated_at
		FROM dns_zones
		WHERE status = 'active' AND updated_at > $1
		ORDER BY name
	`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var zones []DBZone
	for rows.Next() {
		var z DBZone
		if err := rows.Scan(&z.ID, &z.OrgID, &z.Name, &z.Status, &z.CloudflareZoneID, &z.UpdatedAt); err != nil {
			return nil, err
		}
		zones = append(zones, z)
	}
	return zones, rows.Err()
}

// fetchRecords returns all records for a zone.
func (w *Watcher) fetchRecords(ctx context.Context, zoneID string) ([]DBRecord, error) {
	rows, err := w.db.QueryContext(ctx, `
		SELECT id, zone_id, name, type, content, ttl, priority, proxied,
		       sync_to_cloudflare, cloudflare_record_id, updated_at
		FROM dns_records
		WHERE zone_id = $1
		ORDER BY name, type
	`, zoneID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []DBRecord
	for rows.Next() {
		var r DBRecord
		if err := rows.Scan(&r.ID, &r.ZoneID, &r.Name, &r.Type, &r.Content, &r.TTL,
			&r.Priority, &r.Proxied, &r.SyncToCloudflare, &r.CloudflareRecordID, &r.UpdatedAt); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// updateCFRecordID stores the Cloudflare record ID back in PostgreSQL.
func (w *Watcher) updateCFRecordID(ctx context.Context, recordID, cfRecordID string) error {
	_, err := w.db.ExecContext(ctx, `
		UPDATE dns_records SET cloudflare_record_id = $1 WHERE id = $2
	`, cfRecordID, recordID)
	return err
}
