package sazu

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/miekg/dns"
	_ "modernc.org/sqlite" // pure-Go driver, registers as "sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS zones (
	origin     TEXT PRIMARY KEY,
	created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS keys (
	zone       TEXT PRIMARY KEY REFERENCES zones(origin),
	flags      INTEGER NOT NULL,
	protocol   INTEGER NOT NULL,
	algorithm  INTEGER NOT NULL,
	public_key TEXT NOT NULL,
	pinned_at  INTEGER NOT NULL
);

-- Ready for the §10.6 registration record, not yet written to or read from:
-- the wire format a customer uses to actually submit a contact address is
-- still an open design question (see SAZU-PLAN.md), so this table exists
-- but CommitUpdate never touches it yet.
CREATE TABLE IF NOT EXISTS contacts (
	zone          TEXT PRIMARY KEY REFERENCES zones(origin),
	address       TEXT NOT NULL,
	registered_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS rrs (
	id     INTEGER PRIMARY KEY AUTOINCREMENT,
	zone   TEXT NOT NULL REFERENCES zones(origin),
	name   TEXT NOT NULL,
	rrtype INTEGER NOT NULL,
	rr     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS rrs_zone_name_type ON rrs(zone, name, rrtype);
`

// DB is SAZU's SQLite persistence backend, via modernc.org/sqlite -- a
// pure-Go driver, no cgo, keeping this in line with the rest of the tree
// (CoreDNS has no cgo dependencies today; a cgo-based driver like
// mattn/go-sqlite3 would be a real departure from that, affecting
// cross-compilation and static builds).
//
// Store/ZoneData/KeyRegistry stay pure in-memory and untouched by this
// file -- DB is a durability layer handler.go/setup.go add on top: every
// successful UPDATE writes through to it (commit-then-apply-to-memory, so
// a persistence failure can't leave memory and disk disagreeing), and
// LoadAll hydrates memory from it once at startup. Nothing about DB is
// required: a plugin instance configured without a `db` directive never
// constructs one, and behaves exactly as it did before persistence
// existed.
type DB struct {
	sql *sql.DB
}

// Open creates or opens a SQLite database at path and ensures its schema
// exists.
func Open(path string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	// SQLite handles one writer at a time; a single connection avoids
	// SQLITE_BUSY under this plugin's own already-serialized update path
	// (Sazu.updateMu) without needing WAL-mode tuning for a first cut.
	sqlDB.SetMaxOpenConns(1)
	if _, err := sqlDB.Exec(schema); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("creating schema in %s: %w", path, err)
	}
	return &DB{sql: sqlDB}, nil
}

// Close closes the underlying database connection.
func (db *DB) Close() error { return db.sql.Close() }

// CommitUpdate persists one accepted UPDATE transactionally: optionally
// pinning newKey (first contact only -- pass nil for an ordinary push to
// an already-pinned zone) and applying every op in ops, all in one SQLite
// transaction so a failure partway through leaves no partial state on
// disk. Mirrors ApplyUpdateOps' RFC 2136 §2.5 classification exactly, so
// the two stay in lockstep for the same input.
func (db *DB) CommitUpdate(zone string, newKey *dns.DNSKEY, ops []dns.RR, zclass uint16) error {
	tx, err := db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after a successful Commit

	now := time.Now().Unix()
	if _, err := tx.Exec(`INSERT OR IGNORE INTO zones (origin, created_at) VALUES (?, ?)`, zone, now); err != nil {
		return fmt.Errorf("ensuring zone row: %w", err)
	}

	if newKey != nil {
		if _, err := tx.Exec(
			`INSERT OR REPLACE INTO keys (zone, flags, protocol, algorithm, public_key, pinned_at) VALUES (?, ?, ?, ?, ?, ?)`,
			zone, newKey.Flags, newKey.Protocol, newKey.Algorithm, newKey.PublicKey, now,
		); err != nil {
			return fmt.Errorf("pinning key: %w", err)
		}
	}

	// Invalidate any existing NSEC chain before applying this update's own
	// ops -- mirrors ZoneData.PurgeNSEC exactly, and for the same reason
	// (see its doc comment): only a freshly, completely recomputed chain
	// from a full push can be trusted, so an existing one is invalidated
	// up front rather than risked going stale once this row set no longer
	// matches what LoadAll would reconstruct from it. A full push's own
	// NSEC rows, added by the loop below immediately after this, repopulate
	// it in the same transaction.
	if _, err := tx.Exec(`DELETE FROM rrs WHERE zone = ? AND rrtype = ?`, zone, dns.TypeNSEC); err != nil {
		return fmt.Errorf("purging stale NSEC records: %w", err)
	}
	sigRows, err := tx.Query(`SELECT id, rr FROM rrs WHERE zone = ? AND rrtype = ?`, zone, dns.TypeRRSIG)
	if err != nil {
		return fmt.Errorf("finding RRSIGs to check for stale NSEC coverage: %w", err)
	}
	var staleSigIDs []int64
	for sigRows.Next() {
		var id int64
		var text string
		if err := sigRows.Scan(&id, &text); err != nil {
			sigRows.Close()
			return err
		}
		if rr, err := dns.NewRR(text); err == nil {
			if sig, ok := rr.(*dns.RRSIG); ok && sig.TypeCovered == dns.TypeNSEC {
				staleSigIDs = append(staleSigIDs, id)
			}
		}
	}
	sigRows.Close()
	for _, id := range staleSigIDs {
		if _, err := tx.Exec(`DELETE FROM rrs WHERE id = ?`, id); err != nil {
			return fmt.Errorf("purging stale NSEC RRSIG: %w", err)
		}
	}

	for _, rr := range ops {
		h := rr.Header()
		switch {
		case h.Class == zclass:
			// §2.5.1 Add to an RRset. A SOA at the apex replaces any
			// existing one, mirroring ZoneData.insertLocked exactly.
			if _, ok := rr.(*dns.SOA); ok && normalizeZone(h.Name) == normalizeZone(zone) {
				if _, err := tx.Exec(`DELETE FROM rrs WHERE zone = ? AND name = ? AND rrtype = ?`,
					zone, normalizeZone(h.Name), dns.TypeSOA); err != nil {
					return fmt.Errorf("replacing SOA: %w", err)
				}
			}
			if _, err := tx.Exec(`INSERT INTO rrs (zone, name, rrtype, rr) VALUES (?, ?, ?, ?)`,
				zone, normalizeZone(h.Name), h.Rrtype, rr.String()); err != nil {
				return fmt.Errorf("adding record: %w", err)
			}
		case h.Class == dns.ClassANY && h.Rrtype == dns.TypeANY && h.Rdlength == 0:
			// §2.5.3 Delete all RRsets from a name (apex excepted).
			if normalizeZone(h.Name) == normalizeZone(zone) {
				continue
			}
			if _, err := tx.Exec(`DELETE FROM rrs WHERE zone = ? AND name = ?`, zone, normalizeZone(h.Name)); err != nil {
				return fmt.Errorf("deleting name: %w", err)
			}
		case h.Class == dns.ClassANY && h.Rdlength == 0:
			// §2.5.2 Delete an RRset (apex SOA excepted).
			if h.Rrtype == dns.TypeSOA && normalizeZone(h.Name) == normalizeZone(zone) {
				continue
			}
			if _, err := tx.Exec(`DELETE FROM rrs WHERE zone = ? AND name = ? AND rrtype = ?`,
				zone, normalizeZone(h.Name), h.Rrtype); err != nil {
				return fmt.Errorf("deleting rrset: %w", err)
			}
		case h.Class == dns.ClassNONE:
			// §2.5.4 Delete one RR -- fetch candidates and match by
			// content (ignoring TTL/Class) the same way ZoneData.DeleteRR
			// does, since matching that in SQL would need the same
			// normalization logic duplicated as a query.
			rows, err := tx.Query(`SELECT id, rr FROM rrs WHERE zone = ? AND name = ? AND rrtype = ?`,
				zone, normalizeZone(h.Name), h.Rrtype)
			if err != nil {
				return fmt.Errorf("finding record to delete: %w", err)
			}
			var toDelete []int64
			for rows.Next() {
				var id int64
				var text string
				if err := rows.Scan(&id, &text); err != nil {
					rows.Close()
					return err
				}
				existing, err := dns.NewRR(text)
				if err != nil {
					continue // shouldn't happen; skip rather than fail the whole transaction
				}
				if rrEqualContent(existing, rr) {
					toDelete = append(toDelete, id)
				}
			}
			rows.Close()
			for _, id := range toDelete {
				if _, err := tx.Exec(`DELETE FROM rrs WHERE id = ?`, id); err != nil {
					return fmt.Errorf("deleting record: %w", err)
				}
			}
		default:
			return fmt.Errorf("malformed update op for %s", h.Name)
		}
	}

	return tx.Commit()
}

// LoadAll reads every persisted zone and key back into fresh in-memory
// Store/KeyRegistry instances, for hydrating a plugin instance at startup.
func (db *DB) LoadAll() (*Store, *KeyRegistry, error) {
	store := NewStore()
	keys := NewKeyRegistry()

	zoneRows, err := db.sql.Query(`SELECT origin FROM zones`)
	if err != nil {
		return nil, nil, fmt.Errorf("loading zones: %w", err)
	}
	var origins []string
	for zoneRows.Next() {
		var origin string
		if err := zoneRows.Scan(&origin); err != nil {
			zoneRows.Close()
			return nil, nil, err
		}
		origins = append(origins, origin)
	}
	zoneRows.Close()

	for _, origin := range origins {
		z := store.GetOrCreate(origin)

		rrRows, err := db.sql.Query(`SELECT rr FROM rrs WHERE zone = ? ORDER BY id ASC`, origin)
		if err != nil {
			return nil, nil, fmt.Errorf("loading records for %s: %w", origin, err)
		}
		for rrRows.Next() {
			var text string
			if err := rrRows.Scan(&text); err != nil {
				rrRows.Close()
				return nil, nil, err
			}
			rr, err := dns.NewRR(text)
			if err != nil {
				rrRows.Close()
				return nil, nil, fmt.Errorf("parsing stored record %q for %s: %w", text, origin, err)
			}
			z.Insert(rr)
		}
		rrRows.Close()

		var flags, protocol, algorithm int64
		var publicKey string
		row := db.sql.QueryRow(`SELECT flags, protocol, algorithm, public_key FROM keys WHERE zone = ?`, origin)
		switch err := row.Scan(&flags, &protocol, &algorithm, &publicKey); err {
		case nil:
			keys.Pin(origin, &dns.DNSKEY{
				Hdr:       dns.RR_Header{Name: dns.Fqdn(origin), Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET},
				Flags:     uint16(flags),
				Protocol:  uint8(protocol),
				Algorithm: uint8(algorithm),
				PublicKey: publicKey,
			})
		case sql.ErrNoRows:
			// A zone row with no pinned key shouldn't normally happen
			// (CommitUpdate always pins one at first contact), but isn't
			// fatal to loading -- the zone just won't accept further
			// pushes until an operator intervenes.
		default:
			return nil, nil, fmt.Errorf("loading key for %s: %w", origin, err)
		}
	}

	return store, keys, nil
}
