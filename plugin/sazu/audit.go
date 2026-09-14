package sazu

import (
	"crypto/rand"
	"fmt"
	"time"
)

// AuditEntry is one row of §12's audit trail: the record of what
// serveUpdate decided about one UPDATE transaction, kept regardless of
// whether it was accepted or rejected -- an operator investigating "why
// did my push fail" or "who touched this zone and when" needs the
// rejected attempts at least as much as the accepted ones.
type AuditEntry struct {
	// ID is a fresh, server-generated identifier for this transaction --
	// not something the client supplies or sees on the wire today (see
	// SAZU-PLAN.md for why this is deliberately server-side-only for
	// now); it exists purely to let one accepted or rejected attempt be
	// found again later in the audit log.
	ID         string
	Zone       string
	RemoteAddr string
	Rcode      string // dns.RcodeToString[...], e.g. "NOERROR", "REFUSED"
	Status     string // a §12 status code (e.g. ERR_NO_DS_PUBLISHED), or "" if none applies
	At         time.Time
}

// newTransactionID returns a fresh random UUID (v4, RFC 4122), used to
// identify one audit-log entry. Hand-rolled from crypto/rand rather than
// pulling in a UUID library: the format is simple enough (16 random
// bytes, two fixed nibbles, hyphen placement) that a dependency buys
// nothing here.
func newTransactionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failing is not a condition this package can usefully
		// recover from or is ever expected to hit in practice; a
		// non-random-but-still-unique fallback (current time) keeps
		// serveUpdate itself from having to handle an error here for a
		// purely advisory audit-trail ID.
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
