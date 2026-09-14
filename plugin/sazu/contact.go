package sazu

import (
	"fmt"
	"strings"
	"sync"

	"github.com/miekg/dns"
)

// contactOwnerPrefix names the reserved owner SAZU treats as its §10.6
// registration-contact record: a TXT RRset at "_sazu-contact.<zone>"
// rather than a new field bolted onto the wire format. Piggybacking on an
// ordinary Add/Delete RRset op lets a contact address ride the exact same
// authenticated UPDATE as everything else -- SIG(0) already authenticates
// the whole message, so no separate signature, transaction, or transport
// is needed just for this. It is deliberately stripped out of the ops
// before they ever reach zone content (prerequisites, RRSIG verification,
// ApplyUpdateOps, the served zone): unlike a DNSKEY, a customer's
// registered contact address has no reason to be public, queryable DNS
// data, and unlike zone content it is never itself DNSSEC-signed.
const contactOwnerPrefix = "_sazu-contact."

func contactOwnerName(zone string) string {
	return contactOwnerPrefix + normalizeZone(zone)
}

// ContactOwnerName returns the reserved owner name a client (sazuctl,
// sazu-watchd) addresses a §10.6 registration-contact op to for zone.
// Exported so a client can construct the op itself with dns.Msg.Insert/
// Remove -- see BuildContactOp for the common case.
func ContactOwnerName(zone string) string { return contactOwnerName(zone) }

// BuildContactOp builds the TXT record a client sends, as an ordinary
// Insert-shaped RFC 2136 Add op, to register addrs as zone's §10.6
// contact. It is deliberately NOT run through SignZoneContent: a contact
// registration is not zone content and is never itself DNSSEC-signed
// (SIG(0) on the containing UPDATE already authenticates it) -- a client
// that instead hand-built and signed this RRset would just have its
// RRSIG silently dropped by the server (see splitContactOps), so this
// exists to make the signature-free path the easy, obvious one.
func BuildContactOp(zone string, addrs []string) (*dns.TXT, error) {
	if _, err := validateContactAddresses(addrs); err != nil {
		return nil, err
	}
	return &dns.TXT{
		Hdr: dns.RR_Header{Name: contactOwnerName(zone), Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 3600},
		Txt: addrs,
	}, nil
}

// ContactUpdate is the effect a push's contact-record op should have on a
// zone's registered contact: Addresses non-empty sets it, nil/empty
// clears it.
type ContactUpdate struct {
	Addresses []string
}

// splitContactOps separates ops into the zone-content ops the rest of
// this package already knows how to handle and, if present, the
// registration-contact directive for zone. At most one contact directive
// is meaningful per push; more than one is treated as malformed rather
// than silently picking one, the same posture findCandidateKey takes for
// a push carrying more than one candidate DNSKEY.
func splitContactOps(ops []dns.RR, zone string) (rest []dns.RR, update *ContactUpdate, err error) {
	owner := contactOwnerName(zone)
	rest = make([]dns.RR, 0, len(ops))
	seen := false
	for _, rr := range ops {
		h := rr.Header()
		if strings.EqualFold(normalizeZone(h.Name), owner) && h.Rrtype == dns.TypeRRSIG {
			// A naively-built client might run the contact TXT through the
			// same signing path as real zone content (SignZoneContent
			// signs whatever RRset it's handed, with no way to know this
			// one is special) and send its RRSIG along too. Drop it
			// silently rather than let an orphan signature for a record
			// that will never be served slip into zone content -- it isn't
			// zone content, so it never needs, or gets, a signature of its
			// own; SIG(0) already authenticates the whole push.
			if sig, ok := rr.(*dns.RRSIG); ok && sig.TypeCovered == dns.TypeTXT {
				continue
			}
		}
		if !strings.EqualFold(normalizeZone(h.Name), owner) || h.Rrtype != dns.TypeTXT {
			rest = append(rest, rr)
			continue
		}
		if seen {
			return nil, nil, fmt.Errorf("more than one contact directive in update")
		}
		seen = true

		switch {
		case h.Class == dns.ClassINET && h.Rdlength > 0:
			txt, ok := rr.(*dns.TXT)
			if !ok {
				return nil, nil, fmt.Errorf("malformed contact record")
			}
			addrs, verr := validateContactAddresses(txt.Txt)
			if verr != nil {
				return nil, nil, verr
			}
			update = &ContactUpdate{Addresses: addrs}
		case h.Class == dns.ClassNONE, (h.Class == dns.ClassANY && h.Rdlength == 0):
			// §2.5.2/§2.5.4-shaped delete: clear the registered contact.
			update = &ContactUpdate{}
		default:
			return nil, nil, fmt.Errorf("malformed contact directive")
		}
	}
	return rest, update, nil
}

// validateContactAddresses checks each address carries a scheme
// sazu-watchd (§11) knows how to alert through: "mailto:" (RFC 6068) for
// email, or "http"/"https" for a webhook POST. Keeping this a closed set,
// rather than accepting an opaque string, is what lets sazu-watchd
// dispatch on scheme alone with no further per-zone configuration.
func validateContactAddresses(addrs []string) ([]string, error) {
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if !hasNonEmptySchemePrefix(a, "mailto:") &&
			!hasNonEmptySchemePrefix(a, "http://") &&
			!hasNonEmptySchemePrefix(a, "https://") {
			return nil, fmt.Errorf("contact address %q: must start with mailto:, http://, or https://", a)
		}
		out = append(out, a)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("contact record carries no non-empty addresses")
	}
	return out, nil
}

func hasNonEmptySchemePrefix(s, scheme string) bool {
	return strings.HasPrefix(s, scheme) && len(s) > len(scheme)
}

// ContactRegistry tracks each zone's registered contact addresses (§10.6)
// in memory -- read by sazu-watchd, via the same DB that backs this, to
// know where to send delegation-change alerts (§11).
type ContactRegistry struct {
	mu       sync.RWMutex
	contacts map[string][]string
}

// NewContactRegistry returns an empty ContactRegistry.
func NewContactRegistry() *ContactRegistry {
	return &ContactRegistry{contacts: make(map[string][]string)}
}

// Get returns the registered contact addresses for zone, if any.
func (r *ContactRegistry) Get(zone string) ([]string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.contacts[normalizeZone(zone)]
	return a, ok
}

// Set replaces zone's registered contact addresses. Passing a nil/empty
// addrs clears it.
func (r *ContactRegistry) Set(zone string, addrs []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(addrs) == 0 {
		delete(r.contacts, normalizeZone(zone))
		return
	}
	r.contacts[normalizeZone(zone)] = addrs
}
