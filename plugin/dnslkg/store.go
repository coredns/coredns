package dnslkg

import (
	"container/list"
	"fmt"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// Store holds the last known good DNS answer for each tracked (name, qtype)
// pair. It is the abstraction the plugin depends on: the default implementation
// (memStore) keeps everything in memory, but the interface deliberately leaves
// room for a persistent backend to be added later without touching the request
// path.
//
// Implementations must be safe for concurrent use.
type Store interface {
	// Put records m as the last known good answer for name/qtype.
	Put(name string, qtype uint16, m *dns.Msg) error
	// Get returns the stored answer for name/qtype and the time it was stored,
	// or (nil, zero, nil) when no entry exists.
	Get(name string, qtype uint16) (*dns.Msg, time.Time, error)
	// Delete removes the entry for name/qtype if present.
	Delete(name string, qtype uint16) error
	// Close releases any resources held by the store.
	Close() error
}

// defaultMaxEntries bounds the in-memory store when no limit is configured.
const defaultMaxEntries = 10000

// memStore is the default Store: a bounded in-memory map. When the number of
// entries would exceed max, the least-recently-written entry is evicted, so
// memory use is capped regardless of how many distinct names are queried.
//
// It is intentionally simple. A single RWMutex guards the map and the ordering
// list; reads take the (shared) read lock and never mutate shared state, so
// concurrent look-ups on the failure path do not contend with each other.
// There is no background goroutine, no disk I/O and no channels.
type memStore struct {
	max    int
	maxAge time.Duration

	mu      sync.RWMutex
	entries map[string]*list.Element // key -> element in order
	order   *list.List               // front = oldest write, back = newest
}

// entry is a single stored answer.
type entry struct {
	key      string
	packed   []byte    // packed wire-format DNS message
	storedAt time.Time // when the answer was last observed as good
}

// Ensure memStore satisfies the Store interface.
var _ Store = (*memStore)(nil)

// newMemStore returns an in-memory store bounded to max entries. A non-positive
// max falls back to defaultMaxEntries. maxAge, when > 0, is the maximum age of a
// served entry; older entries are treated as absent and reclaimed on the next
// write.
func newMemStore(max int, maxAge time.Duration) *memStore {
	if max <= 0 {
		max = defaultMaxEntries
	}
	return &memStore{
		max:     max,
		maxAge:  maxAge,
		entries: make(map[string]*list.Element),
		order:   list.New(),
	}
}

// storeKey derives the map key for a name/qtype pair. The name is already a
// short, lower-cased FQDN, so it is used directly with the 2-byte qtype
// appended - no hashing is needed for an in-memory map.
func storeKey(name string, qtype uint16) string {
	return name + string([]byte{byte(qtype >> 8), byte(qtype)})
}

// Put records m as the last known good answer for name/qtype. Overwriting an
// existing key refreshes its value and marks it most-recently-written so it is
// the last to be evicted.
func (s *memStore) Put(name string, qtype uint16, m *dns.Msg) error {
	packed, err := m.Pack()
	if err != nil {
		return fmt.Errorf("packing message: %w", err)
	}
	k := storeKey(name, qtype)
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	if el, ok := s.entries[k]; ok {
		e := el.Value.(*entry)
		e.packed = packed
		e.storedAt = now
		s.order.MoveToBack(el)
		return nil
	}

	s.entries[k] = s.order.PushBack(&entry{key: k, packed: packed, storedAt: now})

	// The order list is sorted by write time (front = oldest), so age-based
	// eviction just trims expired entries from the front.
	if s.maxAge > 0 {
		cutoff := now.Add(-s.maxAge)
		for {
			front := s.order.Front()
			if front == nil {
				break
			}
			e := front.Value.(*entry)
			if e.storedAt.After(cutoff) {
				break
			}
			s.order.Remove(front)
			delete(s.entries, e.key)
		}
	}

	if s.order.Len() > s.max {
		if oldest := s.order.Front(); oldest != nil {
			s.order.Remove(oldest)
			delete(s.entries, oldest.Value.(*entry).key)
		}
	}
	return nil
}

// Get returns the stored last known good answer for name/qtype together with
// the time it was stored. It returns (nil, zero, nil) when no entry exists.
//
// Only the (shared) read lock is held, and just long enough to copy the entry's
// immutable packed bytes; the unpack happens after the lock is released.
func (s *memStore) Get(name string, qtype uint16) (*dns.Msg, time.Time, error) {
	k := storeKey(name, qtype)

	s.mu.RLock()
	el, ok := s.entries[k]
	var packed []byte
	var storedAt time.Time
	if ok {
		e := el.Value.(*entry)
		packed = e.packed
		storedAt = e.storedAt
	}
	s.mu.RUnlock()

	if !ok {
		return nil, time.Time{}, nil
	}

	// Entries past max_age are treated as absent; their memory is reclaimed by
	// the front-eviction on the next write.
	if s.maxAge > 0 && time.Since(storedAt) > s.maxAge {
		return nil, time.Time{}, nil
	}

	m := new(dns.Msg)
	if err := m.Unpack(packed); err != nil {
		return nil, time.Time{}, fmt.Errorf("unpacking message: %w", err)
	}
	return m, storedAt, nil
}

// Delete removes the entry for name/qtype if present.
func (s *memStore) Delete(name string, qtype uint16) error {
	k := storeKey(name, qtype)

	s.mu.Lock()
	if el, ok := s.entries[k]; ok {
		s.order.Remove(el)
		delete(s.entries, k)
	}
	s.mu.Unlock()
	return nil
}

// Close releases resources held by the store. The in-memory store holds none,
// so this is a no-op; it exists to satisfy the Store interface and to give a
// future persistent backend a shutdown hook.
func (s *memStore) Close() error { return nil }