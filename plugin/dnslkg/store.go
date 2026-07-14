package dnslkg

import (
	"database/sql"
	"fmt"
	"net/url"
	"time"

	"github.com/miekg/dns"
	_ "modernc.org/sqlite"
)

// store is a small persistent key/value store, keyed by (name, qtype), that
// holds the last known good DNS answer (as a packed wire-format message) for
// each tracked name. It is backed by an on-disk SQLite database opened in WAL
// mode to allow concurrent readers alongside a single writer.
type store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS lkg (
    name      TEXT    NOT NULL,
    qtype     INTEGER NOT NULL,
    msg       BLOB    NOT NULL,
    stored_at INTEGER NOT NULL,
    PRIMARY KEY (name, qtype)
);`

// newStore opens (creating if necessary) the SQLite database at path and
// ensures the schema exists.
func newStore(path string) (*store, error) {
	// Enable WAL journaling and a busy timeout so concurrent access from the
	// (potentially many) CoreDNS request goroutines does not immediately fail
	// with "database is locked".
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)", url.PathEscape(path))

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}

	return &store{db: db}, nil
}

// put records m as the last known good answer for name/qtype, replacing any
// previous entry.
func (s *store) put(name string, qtype uint16, m *dns.Msg) error {
	packed, err := m.Pack()
	if err != nil {
		return fmt.Errorf("packing message: %w", err)
	}
	_, err = s.db.Exec(
		`INSERT INTO lkg (name, qtype, msg, stored_at) VALUES (?, ?, ?, ?)
         ON CONFLICT(name, qtype) DO UPDATE SET msg = excluded.msg, stored_at = excluded.stored_at`,
		name, int(qtype), packed, time.Now().Unix(),
	)
	return err
}

// get returns the stored last known good answer for name/qtype together with
// the time it was stored. It returns (nil, zero, nil) when no entry exists.
func (s *store) get(name string, qtype uint16) (*dns.Msg, time.Time, error) {
	var (
		packed   []byte
		storedAt int64
	)
	err := s.db.QueryRow(
		`SELECT msg, stored_at FROM lkg WHERE name = ? AND qtype = ?`,
		name, int(qtype),
	).Scan(&packed, &storedAt)
	if err == sql.ErrNoRows {
		return nil, time.Time{}, nil
	}
	if err != nil {
		return nil, time.Time{}, err
	}

	m := new(dns.Msg)
	if err := m.Unpack(packed); err != nil {
		return nil, time.Time{}, fmt.Errorf("unpacking message: %w", err)
	}
	return m, time.Unix(storedAt, 0), nil
}

// delete removes the entry for name/qtype if present.
func (s *store) delete(name string, qtype uint16) error {
	_, err := s.db.Exec(`DELETE FROM lkg WHERE name = ? AND qtype = ?`, name, int(qtype))
	return err
}

// close releases the underlying database handle.
func (s *store) close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}
