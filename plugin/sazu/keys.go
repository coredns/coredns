package sazu

import (
	"fmt"
	"strings"
	"sync"

	"github.com/miekg/dns"
)

// KeyRole distinguishes a zone's key-signing key (KSK) from an optional
// zone-signing key (ZSK). Every onboarded zone has exactly one KSK --
// it is what the §10.2 chain-of-trust walk anchors to a parent DS
// record, and there is no other way a key becomes trusted for a zone in
// the first place, so SAZU never supports a ZSK-only zone. A ZSK is a
// purely optional addition a customer may register later (see AddZSK):
// it is never DS-anchored itself, and is trusted transitively -- solely
// because an already-trusted key's SIG(0) authenticated the push that
// introduced it. See SAZU-PLAN.md's KSK/ZSK section for why this split
// exists at all: §9.1's single-key model (one Ed25519 key doing both
// SIG(0) authentication and DNSSEC signing) remains the default and
// fully supported for a zone that never registers a ZSK, but every
// rotation of that lone key is necessarily a KSK rotation, which means
// a registrar DS update every time. A ZSK exists so a customer who signs
// zone content often can rotate that key on their own schedule, with no
// registrar involvement and no outbound chain-of-trust network walk on
// this server's side -- at the cost of the DS-anchored guarantee a KSK
// carries, which is exactly why a ZSK is optional and a KSK is not.
type KeyRole int

const (
	RoleKSK KeyRole = iota
	RoleZSK
)

func (r KeyRole) String() string {
	if r == RoleZSK {
		return "ZSK"
	}
	return "KSK"
}

// ManagedKey is one key SAZU currently trusts for a zone, together with
// the two things that determine what it may be used for.
type ManagedKey struct {
	DNSKEY *dns.DNSKEY
	Role   KeyRole

	// CanAuthenticateTx reports whether this key's own SIG(0) signature
	// may authenticate an UPDATE transaction, as opposed to only being
	// trusted to sign zone *content* once some other key has already
	// authenticated the push carrying it. A KSK is always
	// CanAuthenticateTx=true -- there is no other way it becomes
	// trusted. A ZSK defaults to true too (see AddZSK), so a customer
	// who wants their day-to-day automation to push routine content
	// updates without ever touching the KSK can do so; an operator MAY
	// register a ZSK with this false instead, if it should only ever be
	// trusted to sign content pushed under the KSK's own authentication.
	CanAuthenticateTx bool
}

// KeyTag is a small convenience wrapper around dns.DNSKEY.KeyTag().
func (m *ManagedKey) KeyTag() uint16 { return m.DNSKEY.KeyTag() }

// ZoneKeys is everything KeyRegistry tracks for one zone.
type ZoneKeys struct {
	KSK  *ManagedKey
	ZSKs []*ManagedKey
}

// ContentSigners returns every key currently trusted to sign zone
// content for this zone -- the KSK plus every registered ZSK -- in the
// order VerifySignedRRsets should try them. Safe on a zero-value/nil
// receiver only in the sense that it panics the same way any nil-KSK
// ZoneKeys would; callers only ever hold a ZoneKeys obtained from
// KeyRegistry.Get, which never returns one without a KSK.
func (zk *ZoneKeys) ContentSigners() []*dns.DNSKEY {
	out := make([]*dns.DNSKEY, 0, 1+len(zk.ZSKs))
	out = append(out, zk.KSK.DNSKEY)
	for _, zsk := range zk.ZSKs {
		out = append(out, zsk.DNSKEY)
	}
	return out
}

// Authenticators returns every key currently trusted to authenticate an
// UPDATE transaction via SIG(0) for this zone -- the KSK, plus any ZSK
// registered with CanAuthenticateTx.
func (zk *ZoneKeys) Authenticators() []*ManagedKey {
	out := make([]*ManagedKey, 0, 1+len(zk.ZSKs))
	out = append(out, &ManagedKey{DNSKEY: zk.KSK.DNSKEY, Role: RoleKSK, CanAuthenticateTx: true})
	for _, zsk := range zk.ZSKs {
		if zsk.CanAuthenticateTx {
			out = append(out, zsk)
		}
	}
	return out
}

// FindZSK returns the registered ZSK with the given key tag, if any.
func (zk *ZoneKeys) FindZSK(keytag uint16) (*ManagedKey, bool) {
	for _, zsk := range zk.ZSKs {
		if zsk.KeyTag() == keytag {
			return zsk, true
		}
	}
	return nil, false
}

// KeyRegistry tracks, per zone, the KSK first contact pinned (or a
// rollover replaced) plus any optional ZSKs a customer has since
// registered on top of it (see ZoneKeys/KeyRole's doc comments for what
// that split is for). It is the server's whole memory of "which key(s)
// are currently trusted for this zone."
type KeyRegistry struct {
	mu    sync.RWMutex
	zones map[string]*ZoneKeys
}

// NewKeyRegistry returns an empty KeyRegistry.
func NewKeyRegistry() *KeyRegistry {
	return &KeyRegistry{zones: make(map[string]*ZoneKeys)}
}

// Get returns the current key set for zone, if any. The returned
// *ZoneKeys is a defensive copy (a fresh struct and a fresh ZSKs slice,
// though the *ManagedKey elements themselves are shared and must be
// treated as immutable) -- safe to read without holding any lock, and
// unaffected by a concurrent PinKSK/AddZSK/RetireZSK on the original.
func (r *KeyRegistry) Get(zone string) (*ZoneKeys, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	zk, ok := r.zones[normalizeZone(zone)]
	if !ok {
		return nil, false
	}
	cp := &ZoneKeys{KSK: zk.KSK, ZSKs: make([]*ManagedKey, len(zk.ZSKs))}
	copy(cp.ZSKs, zk.ZSKs)
	return cp, true
}

// PinKSK records key as zone's KSK, replacing any previous one --
// called at successful first contact, and again on a successful §10.4
// KSK rollover. Deliberately leaves any already-registered ZSKs
// untouched: a KSK rollover does not invalidate them. That mirrors real
// DNSSEC practice (RFC 6781) -- a ZSK's trust never actually rested on
// chain-of-trust against a *specific* KSK, only on having been
// registered under whichever key was trusted at the time, so rotating
// the KSK on top of it changes nothing about keys that are already
// there. KeyRegistry itself enforces nothing about when or how often
// this may legitimately happen; that judgment belongs to serveUpdate,
// the only caller.
func (r *KeyRegistry) PinKSK(zone string, key *dns.DNSKEY) {
	r.mu.Lock()
	defer r.mu.Unlock()
	z := normalizeZone(zone)
	existing := r.zones[z]
	zk := &ZoneKeys{KSK: &ManagedKey{DNSKEY: key, Role: RoleKSK, CanAuthenticateTx: true}}
	if existing != nil {
		zk.ZSKs = existing.ZSKs
	}
	r.zones[z] = zk
}

// AddZSK registers a new ZSK for zone, on top of its existing KSK.
// Returns an error if zone has no KSK yet (AddZSK is only ever reached,
// in practice, after first contact already succeeded) or if a ZSK with
// the same key tag is already registered.
func (r *KeyRegistry) AddZSK(zone string, key *dns.DNSKEY, canAuthenticateTx bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	z := normalizeZone(zone)
	zk, ok := r.zones[z]
	if !ok {
		return fmt.Errorf("sazu: cannot register a ZSK for %s: no KSK pinned yet", zone)
	}
	tag := key.KeyTag()
	for _, existing := range zk.ZSKs {
		if existing.KeyTag() == tag {
			return fmt.Errorf("sazu: a ZSK with key tag %d is already registered for %s", tag, zone)
		}
	}
	zk.ZSKs = append(zk.ZSKs, &ManagedKey{DNSKEY: key, Role: RoleZSK, CanAuthenticateTx: canAuthenticateTx})
	return nil
}

// RetireZSK removes the ZSK with the given key tag from zone's key set,
// reporting whether one was actually found and removed. Retiring a ZSK
// that doesn't exist is a no-op, not an error -- the caller (serveUpdate)
// only reaches this after a DNSKEY delete op already named this exact
// key tag, so "already gone" is a legitimate, idempotent outcome, not a
// sign of anything wrong.
func (r *KeyRegistry) RetireZSK(zone string, keytag uint16) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	zk, ok := r.zones[normalizeZone(zone)]
	if !ok {
		return false
	}
	for i, zsk := range zk.ZSKs {
		if zsk.KeyTag() == keytag {
			zk.ZSKs = append(zk.ZSKs[:i], zk.ZSKs[i+1:]...)
			return true
		}
	}
	return false
}

func normalizeZone(zone string) string {
	return strings.ToLower(dns.Fqdn(zone))
}
