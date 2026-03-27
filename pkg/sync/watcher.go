package sync

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/coredns/coredns/pkg/cloudflare"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// Config for the watcher.
type Config struct {
	DatabaseURL     string
	CoreDNSEndpoint string // e.g. "http://dns-api.hanzo.svc:80"
	CoreDNSAPIKey   string // optional, for HANZO_DNS_API_KEY auth
	CFAPIToken      string // optional, enables CF sync
	SyncInterval    time.Duration
}

// Watcher polls PostgreSQL for DNS changes and syncs to CoreDNS + Cloudflare.
type Watcher struct {
	db              *sql.DB
	corednsEndpoint string
	corednsAPIKey   string
	cfClient        *cloudflare.Client
	interval        time.Duration
	logger          *slog.Logger
	lastSync        time.Time
}

// NewWatcher creates a new sync watcher.
func NewWatcher(cfg Config) (*Watcher, error) {
	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	interval := cfg.SyncInterval
	if interval == 0 {
		interval = 30 * time.Second
	}

	w := &Watcher{
		db:              db,
		corednsEndpoint: cfg.CoreDNSEndpoint,
		corednsAPIKey:   cfg.CoreDNSAPIKey,
		interval:        interval,
		logger:          slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}

	if cfg.CFAPIToken != "" {
		w.cfClient = cloudflare.NewClient(cfg.CFAPIToken)
	}

	return w, nil
}

// Run starts the polling loop. Blocks until ctx is cancelled.
func (w *Watcher) Run(ctx context.Context) error {
	w.logger.Info("starting dns sync watcher",
		"coredns", w.corednsEndpoint,
		"interval", w.interval,
		"cloudflare", w.cfClient != nil,
	)

	// Initial full sync.
	if err := w.SyncAll(ctx); err != nil {
		w.logger.Error("initial sync failed", "error", err)
	}

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("sync watcher stopping")
			return w.db.Close()
		case <-ticker.C:
			if err := w.syncChanged(ctx); err != nil {
				w.logger.Error("sync cycle failed", "error", err)
			}
		}
	}
}

// SyncAll fetches all zones+records from PG and pushes to CoreDNS + CF.
func (w *Watcher) SyncAll(ctx context.Context) error {
	zones, err := w.fetchZones(ctx)
	if err != nil {
		return fmt.Errorf("fetch zones: %w", err)
	}

	w.logger.Info("full sync", "zones", len(zones))

	var allZones []zoneWithRecords
	for _, z := range zones {
		records, err := w.fetchRecords(ctx, z.ID)
		if err != nil {
			w.logger.Error("fetch records failed", "zone", z.Name, "error", err)
			continue
		}
		allZones = append(allZones, zoneWithRecords{Zone: z, Records: records})
	}

	// Push to CoreDNS.
	if err := w.syncToCoreDNS(ctx, allZones); err != nil {
		w.logger.Error("coredns sync failed", "error", err)
	}

	// Push to Cloudflare.
	for _, zwr := range allZones {
		if err := w.syncToCloudflare(ctx, zwr.Zone, zwr.Records); err != nil {
			w.logger.Error("cloudflare sync failed", "zone", zwr.Zone.Name, "error", err)
		}
	}

	w.lastSync = time.Now()
	return nil
}

// syncChanged syncs only zones that have changed since last sync.
func (w *Watcher) syncChanged(ctx context.Context) error {
	if w.lastSync.IsZero() {
		return w.SyncAll(ctx)
	}

	zones, err := w.fetchChangedZones(ctx, w.lastSync)
	if err != nil {
		return fmt.Errorf("fetch changed zones: %w", err)
	}

	if len(zones) == 0 {
		return nil
	}

	w.logger.Info("incremental sync", "changed_zones", len(zones))

	var changedZones []zoneWithRecords
	for _, z := range zones {
		records, err := w.fetchRecords(ctx, z.ID)
		if err != nil {
			w.logger.Error("fetch records failed", "zone", z.Name, "error", err)
			continue
		}
		changedZones = append(changedZones, zoneWithRecords{Zone: z, Records: records})
	}

	if err := w.syncToCoreDNS(ctx, changedZones); err != nil {
		w.logger.Error("coredns sync failed", "error", err)
	}

	for _, zwr := range changedZones {
		if err := w.syncToCloudflare(ctx, zwr.Zone, zwr.Records); err != nil {
			w.logger.Error("cloudflare sync failed", "zone", zwr.Zone.Name, "error", err)
		}
	}

	w.lastSync = time.Now()
	return nil
}

// SyncZone syncs a single zone by ID.
func (w *Watcher) SyncZone(ctx context.Context, zoneID string) error {
	zones, err := w.fetchZones(ctx)
	if err != nil {
		return err
	}

	var zone *DBZone
	for _, z := range zones {
		if z.ID == zoneID {
			zone = &z
			break
		}
	}
	if zone == nil {
		return fmt.Errorf("zone %s not found", zoneID)
	}

	records, err := w.fetchRecords(ctx, zone.ID)
	if err != nil {
		return err
	}

	zwr := []zoneWithRecords{{Zone: *zone, Records: records}}

	if err := w.syncToCoreDNS(ctx, zwr); err != nil {
		return err
	}

	return w.syncToCloudflare(ctx, *zone, records)
}
