package controller

import (
	"context"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

// PostgresWatcher polls a PostgreSQL database for DNS zone/record changes
// and creates/updates DnsZone and DnsRecord CRDs accordingly.
// This bridges the console's Prisma-managed database to the K8s operator.
type PostgresWatcher struct {
	// DatabaseURL is the PostgreSQL connection string.
	DatabaseURL string
	// PollInterval is how often to check for changes.
	PollInterval time.Duration
}

// Start begins the polling loop. Implements manager.Runnable.
func (w *PostgresWatcher) Start(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("postgres-watcher")

	if w.DatabaseURL == "" {
		logger.Info("no DATABASE_URL configured, postgres watcher disabled")
		return nil
	}

	interval := w.PollInterval
	if interval == 0 {
		interval = 30 * time.Second
	}

	logger.Info("starting postgres watcher", "interval", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("postgres watcher stopping")
			return nil
		case <-ticker.C:
			// TODO(phase2): Connect to PG, query dns_zones/dns_records,
			// diff against current CRDs, create/update/delete as needed.
			logger.V(1).Info("polling postgres for DNS changes")
		}
	}
}

// NeedLeaderElection implements manager.LeaderElectionRunnable.
func (w *PostgresWatcher) NeedLeaderElection() bool {
	return true
}
