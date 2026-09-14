package sazu

import (
	"strings"
	"sync"

	"github.com/miekg/dns"
)

// KeyRegistry tracks the one pinned SIG(0)/zone key per zone -- the
// server's memory of "which key was established as authoritative for
// this zone at first contact" (§10.2). Once pinned, a key closes to
// ordinary pushes: only a §10.4 key rollover (serveUpdate's own
// isRollover path, gated on the new candidate independently passing the
// same chain-of-trust check first contact requires) may change it.
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

// Pin records key as the pinned key for zone, replacing any previous
// one. Called at successful first contact, and again on a successful
// §10.4 key rollover -- KeyRegistry itself enforces nothing about when
// or how often this may legitimately happen; that judgment (first
// contact vs. a properly chain-of-trust-verified rollover vs. neither)
// belongs to serveUpdate, the only caller.
func (r *KeyRegistry) Pin(zone string, key *dns.DNSKEY) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.keys[normalizeZone(zone)] = key
}

func normalizeZone(zone string) string {
	return strings.ToLower(dns.Fqdn(zone))
}
