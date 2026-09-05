package dynupdate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/coredns/coredns/plugin/file"

	"github.com/miekg/dns"
	bolt "go.etcd.io/bbolt"
)

var (
	storeBucket = []byte("dynupdate-v1")
	originKey   = []byte("origin")
	recordsKey  = []byte("records")

	storesMu sync.Mutex
	stores   = make(map[string]*zoneStore)
)

// A store is shared by overlapping instances during a Corefile reload. Its
// mutex covers prerequisite evaluation, durable commit, and snapshot publication.
// The file view has no Next handler; each plugin supplies its own chain on reads.
type zoneStore struct {
	mu      sync.RWMutex
	db      *bolt.DB
	origin  string
	records []dns.RR
	view    *file.File
	refs    int // protected by storesMu
}

func (d *DynUpdate) acquireStore() (*zoneStore, error) {
	storesMu.Lock()
	defer storesMu.Unlock()

	// SameFile also handles case aliases on Windows and symlinks/hard links.
	info, statErr := os.Stat(d.database)
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return nil, statErr
	}
	if seedInfo, err := os.Stat(d.seed); err == nil && info != nil && os.SameFile(info, seedInfo) {
		return nil, errors.New("database and seed must be different files")
	}
	for path, s := range stores {
		same := path == d.database
		if !same && info != nil {
			other, err := os.Stat(path)
			same = err == nil && os.SameFile(info, other)
		}
		if !same {
			continue
		}
		if s.origin != d.Zone {
			return nil, fmt.Errorf("database already serves zone %q", s.origin)
		}
		s.mu.RLock()
		err := d.limits.check(s.records)
		s.mu.RUnlock()
		if err != nil {
			return nil, err
		}
		s.refs++
		return s, nil
	}

	db, err := bolt.Open(d.database, 0600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, fmt.Errorf("opening database %q: %w", d.database, err)
	}
	s := &zoneStore{db: db, origin: d.Zone, refs: 1}
	err = db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(storeBucket)
		if b != nil {
			if string(b.Get(originKey)) != d.Zone {
				return fmt.Errorf("database belongs to a different zone")
			}
			s.records, err = decodeRecords(b.Get(recordsKey), d.limits)
			return err
		}
		// An existing database without our metadata is corrupt or incompatible,
		// not an invitation to silently replace acknowledged updates with a seed.
		if statErr == nil {
			return errors.New("unrecognized database format")
		}
		s.records, err = readZoneLimited(d.seed, d.Zone, d.limits)
		if err != nil {
			return err
		}
		b, err = tx.CreateBucket(storeBucket)
		if err != nil {
			return err
		}
		if err := b.Put(originKey, []byte(d.Zone)); err != nil {
			return err
		}
		return putRecords(b, s.records)
	})
	if err == nil {
		s.view, err = d.build(s.records)
	}
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("loading database %q: %w", d.database, err)
	}
	s.view.Next = nil
	stores[d.database] = s
	return s, nil
}

func releaseStore(s *zoneStore) error {
	storesMu.Lock()
	defer storesMu.Unlock()
	s.refs--
	if s.refs != 0 {
		return nil
	}
	for path, active := range stores {
		if active == s {
			delete(stores, path)
			break
		}
	}
	return s.db.Close()
}

// Called with d.mu held. Opening lazily avoids retaining a database lock when
// a later directive or listener makes startup fail (Caddy does not run shutdown
// callbacks for failed startups). Configuration validation uses a temporary ref.
func (d *DynUpdate) ensureStore() error {
	if d.closed {
		return errors.New("dynamic zone is closed")
	}
	if d.database == "" || d.store != nil {
		return nil
	}
	s, err := d.acquireStore()
	if err != nil {
		return err
	}
	d.store = s
	return nil
}

func (d *DynUpdate) close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	if d.store == nil {
		return nil
	}
	s := d.store
	d.store = nil
	return releaseStore(s)
}

// commit must be called with s.mu held. bbolt's default synchronous transaction
// commits before the new immutable snapshot can become visible to queries.
func (s *zoneStore) commit(records []dns.RR, view *file.File) error {
	if err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(storeBucket)
		if b == nil {
			return errors.New("dynamic zone bucket is missing")
		}
		return putRecords(b, records)
	}); err != nil {
		return err
	}
	s.records = records
	copyView := *view
	copyView.Next = nil
	s.view = &copyView
	return nil
}

// RRs are stored as consecutive, uncompressed wire records, not a DNS message:
// a zone is not limited by the 64 KiB message or 16-bit section-count limits.
func putRecords(b *bolt.Bucket, records []dns.RR) error {
	var data []byte
	for _, rr := range records {
		wire := make([]byte, dns.Len(rr))
		n, err := dns.PackRR(rr, wire, 0, nil, false)
		if err != nil {
			return err
		}
		data = append(data, wire[:n]...)
	}
	return b.Put(recordsKey, data)
}

func decodeRecords(data []byte, bound limits) ([]dns.RR, error) {
	bound = bound.defaults()
	if len(data) == 0 || len(data) > bound.bytes {
		return nil, fmt.Errorf("invalid stored zone size (max_bytes %d)", bound.bytes)
	}
	var records []dns.RR
	for offset := 0; offset < len(data); {
		if len(records) == bound.records {
			return nil, fmt.Errorf("stored zone exceeds max_records (%d)", bound.records)
		}
		rr, next, err := dns.UnpackRR(data, offset)
		if err != nil {
			return nil, err
		}
		if next <= offset {
			return nil, errors.New("invalid stored record length")
		}
		records = append(records, rr)
		offset = next
	}
	return records, bound.check(records)
}

func databasePath(path, root string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	return filepath.Abs(path)
}
