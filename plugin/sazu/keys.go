package sazu

import (
	"strings"
	"sync"

	"github.com/miekg/dns"
)

// KeyRegistry tracks the one pinned SIG(0)/zone key per zone -- the
// server's memory of "which key was established as authoritative for
// this zone at first contact" (§10.2). Once pinned, a key closes
// permanently: only a key rollover (§10.4, not yet implemented) may
// change it, never a later ordinary push.
type KeyRegistry struct {
	mu   sync.RWMutex
	keys map[string]*dns.DNSKEY
}

// NewKeyRegistry returns an empty KeyRegistry.
func NewKeyRegistry() *KeyRegistry {
	return &KeyRegistry{keys: make(map[string]*dns.DNSKEY)}
}

// Get returns the pinned key for zone, if any.
func (r *KeyRegistry) Get(zone string) (*dns.DNSKEY, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	k, ok := r.keys[normalizeZone(zone)]
	return k, ok
}

// Pin records key as the pinned key for zone. Callers are responsible for
// only calling this once, at successful first contact -- KeyRegistry
// itself does not enforce "never overwrite," since a future rollover flow
// legitimately needs to.
func (r *KeyRegistry) Pin(zone string, key *dns.DNSKEY) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.keys[normalizeZone(zone)] = key
}

func normalizeZone(zone string) string {
	return strings.ToLower(dns.Fqdn(zone))
}
