package sazu

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/miekg/dns"
	_ "modernc.org/sqlite" // pure-Go driver, registers as "sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS zones (
	origin     TEXT PRIMARY KEY,
	created_at INTEGER NOT NULL
);

-- One row per key currently trusted for a zone: exactly one role='KSK'
-- row (first contact and every §10.4 rollover replace it, never add a
-- second) plus zero or more role='ZSK' rows (see keys.go's KeyRole doc
-- comment for what the optional ZSK split is for). can_auth_tx mirrors
-- ManagedKey.CanAuthenticateTx -- always 1 for a KSK, customer's choice
-- for a ZSK. A DB created before ZSKs existed has an older-shaped
-- version of this table (zone as its sole primary key, no keytag/role/
-- can_auth_tx columns); see migrateKeysTableIfNeeded for how that gets
-- upgraded in place the first time such a database is opened.
CREATE TABLE IF NOT EXISTS keys (
	zone        TEXT NOT NULL REFERENCES zones(origin),
	keytag      INTEGER NOT NULL,
	role        TEXT NOT NULL,
	flags       INTEGER NOT NULL,
	protocol    INTEGER NOT NULL,
	algorithm   INTEGER NOT NULL,
	public_key  TEXT NOT NULL,
	can_auth_tx INTEGER NOT NULL,
	pinned_at   INTEGER NOT NULL,
	PRIMARY KEY (zone, keytag)
);
-- keys_zone_role is deliberately NOT created here: on a database
-- created before ZSK support existed, the keys table above is a no-op
-- (IF NOT EXISTS -- the old-shape table already exists) and has no
-- role column yet for an index to reference, which would make this
-- entire schema script fail before migrateKeysTableIfNeeded ever gets a
-- chance to run. Open creates this index separately, after migration
-- has guaranteed the column exists either way.

-- §10.6 registration record: a zone's registered contact address(es),
-- newline-joined when there is more than one (see ContactUpdate/
-- splitContactOps in contact.go for the wire-side convention).
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

-- §12 audit trail: one row per UPDATE transaction this server decided on,
-- accepted or rejected. zone is NOT a foreign key into zones(origin) --
-- unlike every other table here, an audit entry is written for a zone
-- that was refused at first contact and so never got a zones row at all,
-- which is exactly the kind of attempt an audit trail exists to remember.
CREATE TABLE IF NOT EXISTS audit_log (
	id          TEXT PRIMARY KEY,
	zone        TEXT NOT NULL,
	remote_addr TEXT NOT NULL,
	rcode       TEXT NOT NULL,
	status      TEXT NOT NULL,
	at          INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS audit_log_zone_at ON audit_log(zone, at);
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
	db := &DB{sql: sqlDB}
	if err := db.migrateKeysTableIfNeeded(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("migrating keys table in %s: %w", path, err)
	}
	// See the schema constant's comment on why this index is created
	// here rather than as part of schema itself: by this point the keys
	// table is guaranteed to have a role column either way (a fresh
	// table always did; migrateKeysTableIfNeeded just added it to an
	// old one), so this is always safe.
	if _, err := sqlDB.Exec(`CREATE INDEX IF NOT EXISTS keys_zone_role ON keys(zone, role)`); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("creating keys_zone_role index in %s: %w", path, err)
	}
	return db, nil
}

// migrateKeysTableIfNeeded upgrades a keys table written before ZSK
// support existed (one row per zone: zone TEXT PRIMARY KEY, no keytag/
// role/can_auth_tx columns) to the current shape (one row per key,
// PRIMARY KEY (zone, keytag)) in place. A fresh database, or one already
// on the current schema, has nothing to do here -- schema's own
// CREATE TABLE IF NOT EXISTS already gave it the current shape, and this
// detects that via the keytag column's presence before touching
// anything. Every pre-existing row becomes that zone's KSK
// (can_auth_tx=1) -- exactly what it always was before ZSKs existed, so
// no existing deployment needs to change anything to keep working.
func (db *DB) migrateKeysTableIfNeeded() error {
	hasKeytag, err := db.columnExists("keys", "keytag")
	if err != nil {
		return fmt.Errorf("inspecting keys table: %w", err)
	}
	if hasKeytag {
		return nil
	}

	rows, err := db.sql.Query(`SELECT zone, flags, protocol, algorithm, public_key, pinned_at FROM keys`)
	if err != nil {
		return fmt.Errorf("reading pre-ZSK keys table: %w", err)
	}
	type oldRow struct {
		zone                       string
		flags, protocol, algorithm int64
		publicKey                  string
		pinnedAt                   int64
	}
	var old []oldRow
	for rows.Next() {
		var r oldRow
		if err := rows.Scan(&r.zone, &r.flags, &r.protocol, &r.algorithm, &r.publicKey, &r.pinnedAt); err != nil {
			rows.Close()
			return err
		}
		old = append(old, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	tx, err := db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after a successful Commit

	if _, err := tx.Exec(`ALTER TABLE keys RENAME TO keys_pre_zsk`); err != nil {
		return fmt.Errorf("renaming old keys table: %w", err)
	}
	if _, err := tx.Exec(`
		CREATE TABLE keys (
			zone        TEXT NOT NULL REFERENCES zones(origin),
			keytag      INTEGER NOT NULL,
			role        TEXT NOT NULL,
			flags       INTEGER NOT NULL,
			protocol    INTEGER NOT NULL,
			algorithm   INTEGER NOT NULL,
			public_key  TEXT NOT NULL,
			can_auth_tx INTEGER NOT NULL,
			pinned_at   INTEGER NOT NULL,
			PRIMARY KEY (zone, keytag)
		)`); err != nil {
		return fmt.Errorf("creating current-shape keys table: %w", err)
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS keys_zone_role ON keys(zone, role)`); err != nil {
		return fmt.Errorf("creating keys_zone_role index: %w", err)
	}
	for _, r := range old {
		dnskey := &dns.DNSKEY{
			Hdr:       dns.RR_Header{Name: dns.Fqdn(r.zone), Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET},
			Flags:     uint16(r.flags),
			Protocol:  uint8(r.protocol),
			Algorithm: uint8(r.algorithm),
			PublicKey: r.publicKey,
		}
		if _, err := tx.Exec(
			`INSERT INTO keys (zone, keytag, role, flags, protocol, algorithm, public_key, can_auth_tx, pinned_at)
			 VALUES (?, ?, 'KSK', ?, ?, ?, ?, 1, ?)`,
			r.zone, dnskey.KeyTag(), r.flags, r.protocol, r.algorithm, r.publicKey, r.pinnedAt,
		); err != nil {
			return fmt.Errorf("migrating key for %s: %w", r.zone, err)
		}
	}
	if _, err := tx.Exec(`DROP TABLE keys_pre_zsk`); err != nil {
		return fmt.Errorf("dropping old keys table: %w", err)
	}
	return tx.Commit()
}

// columnExists reports whether table has a column named column, via
// SQLite's PRAGMA table_info introspection.
func (db *DB) columnExists(table, column string) (bool, error) {
	rows, err := db.sql.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// Close closes the underlying database connection.
func (db *DB) Close() error { return db.sql.Close() }

// KeyChange describes a KeyRegistry mutation for CommitUpdate to persist
// alongside the rest of an UPDATE transaction, atomically -- at most one
// field is normally set per real transaction (that's what serveUpdate's
// own logic guarantees: a single push is either first contact, a KSK
// rollover, a ZSK registration, a ZSK retirement, or none of those), but
// CommitUpdate itself doesn't assume that; it just applies whichever are
// non-nil.
type KeyChange struct {
	// PinKSK is set on first contact or a successful §10.4 KSK rollover.
	PinKSK *dns.DNSKEY
	// AddZSK is set when this push registers a new optional ZSK.
	AddZSK *ManagedKey
	// RetireZSK, if non-nil, is the key tag of a ZSK this push removed.
	RetireZSK *uint16
}

// CommitUpdate persists one accepted UPDATE transactionally: optionally
// applying keyChange (pass nil for an ordinary push that changes no
// key) and applying every op in ops, all in one SQLite transaction so a
// failure partway through leaves no partial state on disk. Mirrors
// ApplyUpdateOps' RFC 2136 §2.5 classification exactly, so the two stay
// in lockstep for the same input.
func (db *DB) CommitUpdate(zone string, keyChange *KeyChange, ops []dns.RR, zclass uint16, contact *ContactUpdate) error {
	tx, err := db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after a successful Commit

	now := time.Now().Unix()
	if _, err := tx.Exec(`INSERT OR IGNORE INTO zones (origin, created_at) VALUES (?, ?)`, zone, now); err != nil {
		return fmt.Errorf("ensuring zone row: %w", err)
	}

	if keyChange != nil {
		if newKSK := keyChange.PinKSK; newKSK != nil {
			// A rollover's new KSK usually has a different key tag than
			// the one it replaces -- PRIMARY KEY (zone, keytag) would
			// leave the old row behind as a second, stale "KSK" unless
			// it's explicitly cleared first. There is always at most one
			// role='KSK' row per zone by construction, so this is safe
			// to do unconditionally.
			if _, err := tx.Exec(`DELETE FROM keys WHERE zone = ? AND role = 'KSK'`, zone); err != nil {
				return fmt.Errorf("clearing previous KSK: %w", err)
			}
			if _, err := tx.Exec(
				`INSERT INTO keys (zone, keytag, role, flags, protocol, algorithm, public_key, can_auth_tx, pinned_at)
				 VALUES (?, ?, 'KSK', ?, ?, ?, ?, 1, ?)`,
				zone, newKSK.KeyTag(), newKSK.Flags, newKSK.Protocol, newKSK.Algorithm, newKSK.PublicKey, now,
			); err != nil {
				return fmt.Errorf("pinning KSK: %w", err)
			}
		}
		if zsk := keyChange.AddZSK; zsk != nil {
			canAuth := 0
			if zsk.CanAuthenticateTx {
				canAuth = 1
			}
			if _, err := tx.Exec(
				`INSERT OR REPLACE INTO keys (zone, keytag, role, flags, protocol, algorithm, public_key, can_auth_tx, pinned_at)
				 VALUES (?, ?, 'ZSK', ?, ?, ?, ?, ?, ?)`,
				zone, zsk.KeyTag(), zsk.DNSKEY.Flags, zsk.DNSKEY.Protocol, zsk.DNSKEY.Algorithm, zsk.DNSKEY.PublicKey, canAuth, now,
			); err != nil {
				return fmt.Errorf("registering ZSK: %w", err)
			}
		}
		if keytag := keyChange.RetireZSK; keytag != nil {
			if _, err := tx.Exec(`DELETE FROM keys WHERE zone = ? AND keytag = ? AND role = 'ZSK'`, zone, *keytag); err != nil {
				return fmt.Errorf("retiring ZSK: %w", err)
			}
		}
	}

	if contact != nil {
		if len(contact.Addresses) == 0 {
			if _, err := tx.Exec(`DELETE FROM contacts WHERE zone = ?`, zone); err != nil {
				return fmt.Errorf("clearing contact: %w", err)
			}
		} else if _, err := tx.Exec(
			`INSERT OR REPLACE INTO contacts (zone, address, registered_at) VALUES (?, ?, ?)`,
			zone, strings.Join(contact.Addresses, "\n"), now,
		); err != nil {
			return fmt.Errorf("registering contact: %w", err)
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

// LoadAll reads every persisted zone, key, and registered contact back
// into fresh in-memory Store/KeyRegistry/ContactRegistry instances, for
// hydrating a plugin instance at startup.
func (db *DB) LoadAll() (*Store, *KeyRegistry, *ContactRegistry, error) {
	store := NewStore()
	keys := NewKeyRegistry()
	contacts := NewContactRegistry()

	origins, err := db.ListZones()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("loading zones: %w", err)
	}

	for _, origin := range origins {
		z := store.GetOrCreate(origin)

		rrRows, err := db.sql.Query(`SELECT rr FROM rrs WHERE zone = ? ORDER BY id ASC`, origin)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("loading records for %s: %w", origin, err)
		}
		for rrRows.Next() {
			var text string
			if err := rrRows.Scan(&text); err != nil {
				rrRows.Close()
				return nil, nil, nil, err
			}
			rr, err := dns.NewRR(text)
			if err != nil {
				rrRows.Close()
				return nil, nil, nil, fmt.Errorf("parsing stored record %q for %s: %w", text, origin, err)
			}
			z.Insert(rr)
		}
		rrRows.Close()

		switch zk, ok, err := db.LoadZoneKeys(origin); {
		case err != nil:
			return nil, nil, nil, fmt.Errorf("loading keys for %s: %w", origin, err)
		case ok:
			keys.PinKSK(origin, zk.KSK.DNSKEY)
			for _, zsk := range zk.ZSKs {
				if err := keys.AddZSK(origin, zsk.DNSKEY, zsk.CanAuthenticateTx); err != nil {
					return nil, nil, nil, fmt.Errorf("restoring ZSK for %s: %w", origin, err)
				}
			}
		default:
			// A zone row with no pinned key shouldn't normally happen
			// (CommitUpdate always pins one at first contact), but isn't
			// fatal to loading -- the zone just won't accept further
			// pushes until an operator intervenes.
		}

		switch addrs, ok, err := db.LoadContact(origin); {
		case err != nil:
			return nil, nil, nil, fmt.Errorf("loading contact for %s: %w", origin, err)
		case ok:
			contacts.Set(origin, addrs)
		default:
			// No contact registered for this zone -- fine, §10.6 is optional.
		}
	}

	return store, keys, contacts, nil
}

// ListZones returns every onboarded zone's origin -- a lighter-weight
// alternative to LoadAll for a caller (like sazu-watchd, §11) that needs
// to enumerate zones without loading their full content.
func (db *DB) ListZones() ([]string, error) {
	rows, err := db.sql.Query(`SELECT origin FROM zones`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var origins []string
	for rows.Next() {
		var origin string
		if err := rows.Scan(&origin); err != nil {
			return nil, err
		}
		origins = append(origins, origin)
	}
	return origins, rows.Err()
}

// LoadKey returns zone's KSK, if any -- a lighter-weight alternative to
// LoadAll/LoadZoneKeys for a caller (like sazu-watchd, §11) that only
// needs the one key its chain-of-trust re-check actually cares about:
// a ZSK is never DS-anchored, so it has nothing for that check to verify
// in the first place.
func (db *DB) LoadKey(zone string) (*dns.DNSKEY, bool, error) {
	var flags, protocol, algorithm int64
	var publicKey string
	switch err := db.sql.QueryRow(`SELECT flags, protocol, algorithm, public_key FROM keys WHERE zone = ? AND role = 'KSK'`, zone).
		Scan(&flags, &protocol, &algorithm, &publicKey); err {
	case nil:
		return &dns.DNSKEY{
			Hdr:       dns.RR_Header{Name: dns.Fqdn(zone), Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET},
			Flags:     uint16(flags),
			Protocol:  uint8(protocol),
			Algorithm: uint8(algorithm),
			PublicKey: publicKey,
		}, true, nil
	case sql.ErrNoRows:
		return nil, false, nil
	default:
		return nil, false, err
	}
}

// LoadZoneKeys returns zone's full key set -- its KSK plus every
// registered ZSK -- for hydrating a KeyRegistry (LoadAll) or for a
// caller that needs more than LoadKey's KSK-only view.
func (db *DB) LoadZoneKeys(zone string) (*ZoneKeys, bool, error) {
	rows, err := db.sql.Query(
		`SELECT keytag, role, flags, protocol, algorithm, public_key, can_auth_tx FROM keys WHERE zone = ?`, zone)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	zk := &ZoneKeys{}
	for rows.Next() {
		var keytag int64
		var role string
		var flags, protocol, algorithm, canAuth int64
		var publicKey string
		if err := rows.Scan(&keytag, &role, &flags, &protocol, &algorithm, &publicKey, &canAuth); err != nil {
			return nil, false, err
		}
		dnskey := &dns.DNSKEY{
			Hdr:       dns.RR_Header{Name: dns.Fqdn(zone), Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET},
			Flags:     uint16(flags),
			Protocol:  uint8(protocol),
			Algorithm: uint8(algorithm),
			PublicKey: publicKey,
		}
		switch role {
		case "KSK":
			zk.KSK = &ManagedKey{DNSKEY: dnskey, Role: RoleKSK, CanAuthenticateTx: true}
		case "ZSK":
			zk.ZSKs = append(zk.ZSKs, &ManagedKey{DNSKEY: dnskey, Role: RoleZSK, CanAuthenticateTx: canAuth != 0})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	if zk.KSK == nil {
		return nil, false, nil
	}
	return zk, true, nil
}

// LoadContact returns the registered §10.6 contact addresses for zone, if
// any -- a lighter-weight alternative to LoadAll for a caller that only
// needs one zone's contact.
func (db *DB) LoadContact(zone string) ([]string, bool, error) {
	var address string
	switch err := db.sql.QueryRow(`SELECT address FROM contacts WHERE zone = ?`, zone).Scan(&address); err {
	case nil:
		return strings.Split(address, "\n"), true, nil
	case sql.ErrNoRows:
		return nil, false, nil
	default:
		return nil, false, err
	}
}

// RecordTransaction appends one row to the §12 audit trail: entry.ID must
// be unique (it's the primary key), which newTransactionID's randomness
// already guarantees in practice. A write here is independent of, and
// never rolled back by, CommitUpdate's own transaction -- the audit
// record is written by the caller (serveUpdate) after that transaction
// has already succeeded or failed, and is deliberately never itself the
// reason an UPDATE fails: see handler.go's own comment where this is
// called for why a logging error here only gets logged, not surfaced to
// the client.
func (db *DB) RecordTransaction(entry AuditEntry) error {
	_, err := db.sql.Exec(
		`INSERT INTO audit_log (id, zone, remote_addr, rcode, status, at) VALUES (?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.Zone, entry.RemoteAddr, entry.Rcode, entry.Status, entry.At.Unix(),
	)
	return err
}

// RecentTransactions returns up to limit audit-log entries for zone,
// newest first -- the read side of the §12 audit trail, for an operator
// (or a future admin surface) asking "what happened to this zone's
// pushes recently."
func (db *DB) RecentTransactions(zone string, limit int) ([]AuditEntry, error) {
	rows, err := db.sql.Query(
		`SELECT id, zone, remote_addr, rcode, status, at FROM audit_log WHERE zone = ? ORDER BY at DESC, rowid DESC LIMIT ?`,
		zone, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		var at int64
		if err := rows.Scan(&e.ID, &e.Zone, &e.RemoteAddr, &e.Rcode, &e.Status, &at); err != nil {
			return nil, err
		}
		e.At = time.Unix(at, 0)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
