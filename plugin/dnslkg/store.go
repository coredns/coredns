package dnslkg

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// Persistence design (implemented by snapshotStore, the default Store below):
// a small persistent key/value store, keyed by (name, qtype), that holds the
// last known good DNS answer (as a packed wire-format message) for each tracked
// name. It is the default Store implementation.
//
// It is deliberately simple and dependency-free (standard library only). The
// design is a periodically-snapshotted in-memory map:
//
//   - The map is the single source of truth. Reads (only on the upstream-failure
//     path) and writes are plain, lock-cheap map operations - the request path
//     never touches the disk.
//   - A write just flips a dirty flag. A background goroutine flushes the whole
//     map to one on-disk file at most once every flushInterval, and only when
//     something actually changed. Disk-write frequency is therefore bounded and
//     independent of the query rate.
//   - Each flush writes to a temp file that is fsync'd and atomically renamed
//     into place, so the on-disk snapshot is always a complete, consistent copy
//     - a crash can never leave a half-written file, so no journaling, CRC-tail
//     recovery or compaction machinery is needed.
//
// Because the whole (bounded) live set already lives in memory, the snapshot is
// the state: loading it on startup fully restores the store. The trade-off is
// that answers observed since the last snapshot are lost on an unclean crash,
// which is acceptable for a best-effort last-known-good cache.

// Store persists last known good DNS answers, keyed by (name, qtype). It is the
// abstraction the plugin depends on, so alternative backends (e.g. a bounded
// LRU, or an external store) can be added without touching the request path.
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
	// Close flushes any pending state and releases resources.
	Close() error
}

// snapshotStore is the default Store implementation: an in-memory map that is
// periodically snapshotted to a single on-disk file. See the package overview
// for the design rationale.
type snapshotStore struct {
	path string

	mu      sync.RWMutex
	entries map[string]entry
	dirty   bool

	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

// Ensure snapshotStore satisfies the Store interface.
var _ Store = (*snapshotStore)(nil)

// entry is a single stored answer held in memory.
type entry struct {
	packed   []byte    // packed wire-format DNS message
	storedAt time.Time // when the answer was last observed as good
}

const (
	// keyLen is the fixed length of a hashed key (SHA-256).
	keyLen = sha256.Size
	// flushInterval bounds how often the snapshot is written to disk.
	flushInterval = 10 * time.Second
)

// snapMagic identifies (and versions) the snapshot file format.
var snapMagic = [4]byte{'L', 'K', 'G', '2'}

var crcTable = crc32.MakeTable(crc32.IEEE)

// newSnapshotStore opens the snapshot file at path (ensuring its parent
// directory exists), loads any existing entries, and starts the background
// flusher.
func newSnapshotStore(path string) (*snapshotStore, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("creating store directory %q: %w", dir, err)
		}
	}

	s := &snapshotStore{
		path:    path,
		entries: make(map[string]entry),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	s.load()

	go s.flushLoop()
	return s, nil
}

// keyBytes derives the fixed-length key for a name/qtype pair. A hash keeps keys
// a constant size regardless of the (arbitrary length) query name.
func keyBytes(name string, qtype uint16) [keyLen]byte {
	var buf [2]byte
	binary.BigEndian.PutUint16(buf[:], qtype)
	h := sha256.New()
	h.Write([]byte(name))
	h.Write(buf[:])
	var k [keyLen]byte
	copy(k[:], h.Sum(nil))
	return k
}

// Put records m as the last known good answer for name/qtype. Identical repeat
// answers only refresh the in-memory timestamp and do not dirty the store, so a
// name that keeps resolving to the same value never triggers a disk write.
func (s *snapshotStore) Put(name string, qtype uint16, m *dns.Msg) error {
	packed, err := m.Pack()
	if err != nil {
		return fmt.Errorf("packing message: %w", err)
	}
	k := keyBytes(name, qtype)
	ks := string(k[:])
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	if cur, ok := s.entries[ks]; ok && bytes.Equal(cur.packed, packed) {
		cur.storedAt = now
		s.entries[ks] = cur
		return nil
	}
	s.entries[ks] = entry{packed: packed, storedAt: now}
	s.dirty = true
	return nil
}

// Get returns the stored last known good answer for name/qtype together with
// the time it was stored. It returns (nil, zero, nil) when no entry exists.
func (s *snapshotStore) Get(name string, qtype uint16) (*dns.Msg, time.Time, error) {
	k := keyBytes(name, qtype)

	s.mu.RLock()
	e, ok := s.entries[string(k[:])]
	s.mu.RUnlock()
	if !ok {
		return nil, time.Time{}, nil
	}

	m := new(dns.Msg)
	if err := m.Unpack(e.packed); err != nil {
		return nil, time.Time{}, fmt.Errorf("unpacking message: %w", err)
	}
	return m, e.storedAt, nil
}

// Delete removes the entry for name/qtype if present.
func (s *snapshotStore) Delete(name string, qtype uint16) error {
	k := keyBytes(name, qtype)
	ks := string(k[:])

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.entries[ks]; ok {
		delete(s.entries, ks)
		s.dirty = true
	}
	return nil
}

// Close stops the background flusher and writes a final snapshot.
func (s *snapshotStore) Close() error {
	s.closeOnce.Do(func() {
		close(s.stop)
		<-s.done
	})
	return s.flush()
}

// flushLoop periodically flushes the snapshot until the store is closed.
func (s *snapshotStore) flushLoop() {
	defer close(s.done)

	t := time.NewTicker(flushInterval)
	defer t.Stop()

	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			if err := s.flush(); err != nil {
				log.Warningf("Failed to flush LKG store: %v", err)
			}
		}
	}
}

// flush writes the current map to disk if it has changed since the last flush.
// The (potentially large) file write happens outside the lock; only the quick
// in-memory serialisation is done while holding it.
func (s *snapshotStore) flush() error {
	s.mu.Lock()
	if !s.dirty {
		s.mu.Unlock()
		return nil
	}
	buf := s.encodeLocked()
	s.dirty = false
	s.mu.Unlock()

	if err := s.writeFile(buf); err != nil {
		// Re-mark dirty so the next flush retries.
		s.mu.Lock()
		s.dirty = true
		s.mu.Unlock()
		return err
	}
	return nil
}

// encodeLocked serialises the whole map into the snapshot file format. The
// caller must hold s.mu.
//
// Layout:
//
//	magic[4] | crc32[4] | payload
//	payload  = count:uint32 | count * ( storedAt:int64 | key[keyLen] | vlen:uint32 | value )
//
// The CRC covers payload only and guards against on-disk corruption (atomic
// renames already prevent partially-written snapshots).
func (s *snapshotStore) encodeLocked() []byte {
	size := 4 // count
	for _, e := range s.entries {
		size += 8 + keyLen + 4 + len(e.packed)
	}
	payload := make([]byte, 0, size)

	var num [8]byte
	binary.BigEndian.PutUint32(num[:4], uint32(len(s.entries)))
	payload = append(payload, num[:4]...)

	for ks, e := range s.entries {
		binary.BigEndian.PutUint64(num[:8], uint64(e.storedAt.Unix()))
		payload = append(payload, num[:8]...)
		payload = append(payload, ks...) // ks is exactly keyLen bytes
		binary.BigEndian.PutUint32(num[:4], uint32(len(e.packed)))
		payload = append(payload, num[:4]...)
		payload = append(payload, e.packed...)
	}

	out := make([]byte, 0, len(snapMagic)+4+len(payload))
	out = append(out, snapMagic[:]...)
	var crc [4]byte
	binary.BigEndian.PutUint32(crc[:], crc32.Checksum(payload, crcTable))
	out = append(out, crc[:]...)
	out = append(out, payload...)
	return out
}

// writeFile atomically replaces the snapshot file with buf.
func (s *snapshotStore) writeFile(buf []byte) error {
	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, filepath.Base(s.path)+"-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp snapshot: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(buf); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("writing snapshot: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("syncing snapshot: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("closing snapshot: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("renaming snapshot into place: %w", err)
	}
	return nil
}

// load reads the snapshot file (if any) into the in-memory map. A missing file
// is not an error; a corrupt file is logged and ignored, leaving an empty store.
func (s *snapshotStore) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Warningf("Failed to read LKG store %q: %v", s.path, err)
		}
		return
	}

	entries, err := decodeSnapshot(data)
	if err != nil {
		log.Warningf("Ignoring malformed LKG store %q: %v", s.path, err)
		return
	}
	s.entries = entries
}

// decodeSnapshot parses the snapshot format produced by encodeLocked.
func decodeSnapshot(data []byte) (map[string]entry, error) {
	const headerLen = 4 + 4 // magic + crc
	if len(data) < headerLen {
		return nil, fmt.Errorf("snapshot too short (%d bytes)", len(data))
	}
	var magic [4]byte
	copy(magic[:], data[:4])
	if magic != snapMagic {
		return nil, fmt.Errorf("bad magic")
	}
	crc := binary.BigEndian.Uint32(data[4:8])
	payload := data[headerLen:]
	if crc32.Checksum(payload, crcTable) != crc {
		return nil, fmt.Errorf("bad checksum")
	}
	if len(payload) < 4 {
		return nil, fmt.Errorf("truncated header")
	}

	count := binary.BigEndian.Uint32(payload[:4])
	payload = payload[4:]

	entries := make(map[string]entry, count)
	for i := uint32(0); i < count; i++ {
		if len(payload) < 8+keyLen+4 {
			return nil, fmt.Errorf("truncated entry %d", i)
		}
		storedAt := time.Unix(int64(binary.BigEndian.Uint64(payload[:8])), 0)
		key := string(payload[8 : 8+keyLen])
		vlen := binary.BigEndian.Uint32(payload[8+keyLen : 8+keyLen+4])
		payload = payload[8+keyLen+4:]

		if uint32(len(payload)) < vlen {
			return nil, fmt.Errorf("truncated value for entry %d", i)
		}
		val := make([]byte, vlen)
		copy(val, payload[:vlen])
		payload = payload[vlen:]

		entries[key] = entry{packed: val, storedAt: storedAt}
	}
	return entries, nil
}
