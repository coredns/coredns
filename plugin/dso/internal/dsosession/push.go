package dsosession

import (
	"context"
	"errors"
	"io"
	"iter"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coredns/coredns/plugin/dso/internal/dsomessage"

	"github.com/miekg/dns"
)

var ErrDuplicateSub = errors.New("duplicate Push subscription")

type (
	// Push stores DSO Push subscriptions.
	Push struct {
		all     map[pushSubscribeKey]struct{} // Subscriptions by TLV, as requested.
		allByID map[uint16]pushSubscribeKey   // Subscriptions by ID, as requested.

		classes []uint16
		types   []uint16

		refreshMu sync.Mutex
		expanded  map[pushSubscribeKey]*rrSet   // Subscriptions and corresponding answers, expanded.
		dirty     map[pushSubscribeKey]struct{} // Dirty subscriptions.
		dirtyC    chan struct{}                 // Signal that dirty is not empty.
		refreshC  chan struct{}                 // Signal that refresh is needed.
	}

	// PushLookuper resolves subscription on behalf of [Push].
	PushLookuper interface {
		// Lookup returns RRset that corresponds to given subscription.
		//
		// Lookup can be abandoned once context is cancelled.
		// Returned RRset must follow RFC 8765, Section 6.3.1
		LookupPushSubscription(ctx context.Context, tlv dsomessage.Subscribe) (rrs []dns.RR, ok bool)
	}

	// PushSubscribe is [dsomessage.Subscribe] vetted by [Push.CanAdd].
	PushSubscribe struct {
		dsomessage.Subscribe
		key pushSubscribeKey
	}

	// pushSubscribeKey is normalized TLV for duplicate detection.
	pushSubscribeKey dsomessage.Subscribe

	rrSet struct {
		name     string
		v        []dns.RR
		refcount int // up to 4 client subscriptions may reference the same rrSet because of wildcards
	}
)

// NewPush returns new [Push] service.
//
// Classes and types, which [Push] takes ownership of, control allowed subscriptions and wildcard expansion.
// Must be sorted in increasing order.
func NewPush(classes, types []uint16) *Push {
	return &Push{
		all:      make(map[pushSubscribeKey]struct{}),
		allByID:  make(map[uint16]pushSubscribeKey),
		expanded: make(map[pushSubscribeKey]*rrSet),

		classes: classes,
		types:   types,

		dirty: make(map[pushSubscribeKey]struct{}),

		dirtyC:   make(chan struct{}, 1),
		refreshC: make(chan struct{}, 1),
	}
}

// IsActive returns whether there are active subscriptions.
func (push *Push) IsActive() bool {
	return len(push.allByID) > 0
}

// CanAdd checks if subscription can be added.
//
// Non-nil error indicates fatal protocol violation. Rcode indicates whether and how
// subscription request should be rejected. Only [dns.RcodeSuccess] indicates that subscription
// is vetted for [Push.Add].
func (push *Push) CanAdd(id uint16, tlv dsomessage.Subscribe) (sub PushSubscribe, rcode uint8, err error) {
	// RFC 8765, Section 6.3.1:
	// For individual additions and removals, if the TYPE in the SUBSCRIBE request was not ANY (255),
	// then the TYPE of the record must either be CNAME or match the TYPE given in the SUBSCRIBE request
	//
	// Only allow RR types for which QTYPE=T query can solicit response with TYPE=T in Answer section.
	switch tlv.RRType {
	case dns.TypeOPT, dns.TypeTSIG, dns.TypeRRSIG:
		fallthrough
	case dns.TypeNone, dns.TypeNXNAME, dns.TypeIXFR, dns.TypeAXFR, dns.TypeMAILB, dns.TypeMAILA:
		return sub, dns.RcodeFormatError, nil
	}

	if tlv.Class != dns.ClassANY {
		if _, ok := slices.BinarySearch(push.classes, tlv.Class); !ok {
			return sub, dns.RcodeRefused, nil
		}
	}
	if tlv.RRType != dns.TypeANY {
		if _, ok := slices.BinarySearch(push.types, tlv.RRType); !ok {
			return sub, dns.RcodeRefused, nil
		}
	}

	if _, ok := push.allByID[id]; ok {
		return sub, dns.RcodeServerFailure, ErrDuplicateSub
	}
	key := pushSubscribeKey{
		strings.ToLower(tlv.Name), // assume [dns.UnpackDomainName]-like unicode escapes
		tlv.RRType,
		tlv.Class,
	}
	if _, ok := push.all[key]; ok {
		return sub, dns.RcodeServerFailure, ErrDuplicateSub
	}

	return PushSubscribe{tlv, key}, dns.RcodeSuccess, nil
}

// Add adds subscription.
func (push *Push) Add(id uint16, sub PushSubscribe) {
	if sub.key == (pushSubscribeKey{}) { // force user to call [Push.CanAdd]
		return
	}
	push.allByID[id] = sub.key
	push.all[sub.key] = struct{}{}
	if push.add(sub.Name, sub.key.expand(push.classes, push.types)) {
		select {
		case push.dirtyC <- struct{}{}:
		default:
		}
	}
}

func (push *Push) add(name string, keys iter.Seq[pushSubscribeKey]) (dirty bool) {
	push.refreshMu.Lock()
	defer push.refreshMu.Unlock()

	for key := range keys {
		rrs, ok := push.expanded[key]
		if !ok {
			rrs = &rrSet{name: name}
			push.expanded[key] = rrs
			push.dirty[key] = struct{}{}
			dirty = true
		}
		rrs.refcount++
	}
	return dirty
}

// Remove stops refreshing subscription. It's permitted to remove non-existent subscriptions.
func (push *Push) Remove(tlv dsomessage.Unsubscribe) (dsomessage.Subscribe, bool) {
	// RFC 8765, Section 6.4.1: Consequently, it is possible for a server to receive an UNSUBSCRIBE
	// message that does not match any currently active subscription ... servers MUST silently
	// ignore UNSUBSCRIBE messages that do not match any currently active subscription.
	key, ok := push.allByID[tlv.SubscribeID]
	if ok {
		delete(push.allByID, tlv.SubscribeID)
		delete(push.all, key)
		push.remove(key.expand(push.classes, push.types))
	}
	return dsomessage.Subscribe(key), false
}

func (push *Push) remove(keys iter.Seq[pushSubscribeKey]) {
	push.refreshMu.Lock()
	defer push.refreshMu.Unlock()

	for key := range keys {
		if rrs, ok := push.expanded[key]; ok {
			rrs.refcount--
			if rrs.refcount <= 0 {
				delete(push.expanded, key)
				delete(push.dirty, key)
			}
		}
	}
}

// Reconfirm schedules check that confirms whether RDATA is accurate.
func (push *Push) Reconfirm(tlv dsomessage.Reconfirm) {
	h := tlv.RR.Header()
	key := pushSubscribeKey{
		Name:   strings.ToLower(h.Name),
		RRType: h.Rrtype,
		Class:  h.Class,
	}

	push.refreshMu.Lock()
	_, ok := push.expanded[key]
	if !ok {
		push.refreshMu.Unlock()
		return // not subscribed or stopped
	}
	push.dirty[key] = struct{}{}
	push.refreshMu.Unlock()

	select {
	case push.dirtyC <- struct{}{}:
	default:
	}
}

// Refresh subscriptions ahead of periodic interval.
func (push *Push) Refresh() {
	select {
	case push.refreshC <- struct{}{}:
	default:
	}
}

// Subscriptions returns currently active subscriptions.
func (push *Push) Subscriptions() iter.Seq[dsomessage.Subscribe] {
	return func(yield func(dsomessage.Subscribe) bool) {
		for _, key := range push.allByID {
			if !yield(dsomessage.Subscribe(key)) {
				return
			}
		}
	}
}

// Serve serves Push session by updating clients until context is cancelled.
//
// Subscriptions are periodically refreshed (refreshInterval) via
// upstream lookups. Initial subscription burst is countered by debounceDelay.
// The difference is sent to client.
//
// Returns [context.Context.Err] if stopped gracefully. Otherwise returns resolving, packing or writing error.
func (push *Push) Serve(ctx context.Context, writer io.Writer, upstream PushLookuper, debounceDelay, refreshInterval time.Duration) error {
	var (
		doneC    = make(chan struct{})
		dirtyC   chan struct{}
		refreshC chan struct{}
	)
	defer close(doneC)
	if debounceDelay > 0 {
		dirtyC = make(chan struct{})
		go func() {
			defer close(dirtyC)
			t := time.NewTimer(debounceDelay)
			defer t.Stop()
			for {
				select {
				case <-push.dirtyC:
				case <-doneC:
					return
				}
				t.Reset(debounceDelay)
				select {
				case <-t.C:
				case <-doneC:
					return
				}
				// Drain after debounce delay.
				select {
				case <-push.dirtyC:
				default:
				}
				select {
				case dirtyC <- struct{}{}:
				case <-doneC:
					return
				}
			}
		}()
	} else {
		dirtyC = push.dirtyC
	}
	if refreshInterval > 0 {
		refreshC = make(chan struct{})
		go func() {
			defer close(refreshC)
			t := time.NewTicker(refreshInterval)
			defer t.Stop()
			for {
				select {
				case <-t.C:
				case <-push.refreshC:
				case <-doneC:
					return
				}
				select {
				case refreshC <- struct{}{}:
				case <-doneC:
					return
				}
			}
		}()
	} else {
		refreshC = push.refreshC
	}

	for {
		var dirty []pushSubscribeKey
		select {
		case <-dirtyC:
			push.refreshMu.Lock()
			dirty = slices.Collect(maps.Keys(push.dirty))
			clear(push.dirty)
			push.refreshMu.Unlock()
		case <-refreshC:
			push.refreshMu.Lock()
			dirty = slices.Collect(maps.Keys(push.expanded))
			clear(push.dirty) // expanded is superset of dirty
			push.refreshMu.Unlock()
		case <-ctx.Done():
			return ctx.Err()
		}

		change, err := push.refreshDirty(ctx, upstream, dirty)
		if err != nil {
			return err
		}

		err = push.writeChange(ctx, writer, change)
		if err != nil {
			return err
		}
	}
}

// refreshDirty updates RRSet of dirty subscriptions.
func (push *Push) refreshDirty(ctx context.Context, upstream PushLookuper, dirty []pushSubscribeKey) (change []dns.RR, err error) {
	if len(dirty) == 0 {
		return nil, nil
	}

	answers, err := push.resolveDirty(ctx, upstream, dirty)
	if err != nil {
		return nil, err
	}

	func() {
		push.refreshMu.Lock()
		defer push.refreshMu.Unlock()

		for key, newRRs := range answers {
			if rrs, ok := push.expanded[key]; ok {
				change = append(change, rrs.update(newRRs)...)
			}
		}
	}()

	return change, nil
}

// resolveDirty uses upstream to fetch fresh RRSet for each dirty subscription.
func (push *Push) resolveDirty(ctx context.Context, upstream PushLookuper, dirty []pushSubscribeKey) (answers map[pushSubscribeKey][]dns.RR, err error) {
	answers = make(map[pushSubscribeKey][]dns.RR, len(dirty))
	for _, key := range dirty {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if rrs, ok := upstream.LookupPushSubscription(ctx, dsomessage.Subscribe(key)); ok {
			answers[key] = rrs
		}
	}
	return answers, nil
}

// writeChange chunks change, if needed, and writes Push updates to client.
func (push *Push) writeChange(ctx context.Context, writer io.Writer, change []dns.RR) error {
	for msg, err := range buildUpdateMsg(change) {
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		_, err = writer.Write(msg)
		if err != nil {
			return err
		}
	}
	return nil
}

func (k pushSubscribeKey) expand(classes, types []uint16) iter.Seq[pushSubscribeKey] {
	return func(yield func(pushSubscribeKey) bool) {
		if k.Class != dns.ClassANY {
			classes = []uint16{k.Class}
		}
		if k.RRType != dns.TypeANY {
			types = []uint16{k.RRType}
		}
		for _, cl := range classes {
			for _, ty := range types {
				k.Class = cl
				k.RRType = ty
				if !yield(k) {
					return
				}
			}
		}
	}
}

// update sets newRRs and computes symmetric difference for DSO Push Update.
//
// Slice is consumed, only new RRs are deep copied.
func (s *rrSet) update(newRRs []dns.RR) (change []dns.RR) {
	var (
		oldLen = len(s.v)
		newLen = len(newRRs)

		seen    = make([]bool, oldLen+newLen)
		seenLen = 0 // total # of unchanged RRs
	)

	// Detect all unchanged ("seen") first. Copy "new" RRs only once, reuse "old" RRs on match.
	for oldI, oldRR := range s.v {
		if seenLen == newLen {
			break
		}

		newI := -1
		// High likelyhood of RRsets being identical.
		if oldI < newLen && !seen[oldLen+oldI] && dns.IsDuplicate(oldRR, newRRs[oldI]) {
			newI = oldI
		} else {
			for i, newRR := range newRRs {
				if !seen[oldLen+i] && dns.IsDuplicate(oldRR, newRR) {
					newI = i
					break
				}
			}
		}
		if newI != -1 {
			newRRs[newI] = oldRR // reuse RR that is already owned
			seen[oldLen+newI] = true
			seen[oldI] = true
			seenLen++
		}
	}
	for newI := range newRRs {
		if !seen[oldLen+newI] {
			rr := dns.Copy(newRRs[newI]) // deep copy new RR to own
			rr.Header().Name = s.name
			newRRs[newI] = rr
		}
	}

	// Reuse old slice to store change.
	s.v, change = newRRs, s.v
	changeLen := oldLen - seenLen + newLen - seenLen
	if changeLen == 0 {
		return nil
	}
	if cap(change) < changeLen {
		change1 := make([]dns.RR, oldLen, changeLen) // keep oldLen to catch out-of-bounds
		copy(change1, change)
		change = change1
	}

	// Three iterators: over old, new and change.
	// Fill "seen" with "new" and mark "old" for deletion.
	var oldI, newI, changeI int
	for oldI < oldLen && changeI < changeLen && newI < newLen {
		if seen[oldI] {
			for ; newI < newLen; newI++ {
				if !seen[oldLen+newI] {
					break
				}
			}
			if newI != newLen {
				change[oldI] = newRRs[newI]
				newI++
				oldI++
				changeI++
			}
		} else {
			change[oldI].Header().Ttl = dsomessage.PushTTLRemove
			oldI++
			changeI++
		}
	}
	// Remove remaining "seen" and mark remaining "old" for deletion.
	for oldI < oldLen && changeI < changeLen {
		if seen[oldI] {
			// Batch continuous deletes.
			n := 1
			for ; oldI+n < oldLen; n++ {
				if !seen[oldI+n] {
					break
				}
			}
			change = slices.Delete(change, changeI, changeI+n)
			oldI += n
		} else {
			change[changeI].Header().Ttl = dsomessage.PushTTLRemove
			oldI++
			changeI++
		}
	}
	// Append remaining "new".
	change = change[:changeLen]
	for newI < newLen && changeI < changeLen {
		if !seen[oldLen+newI] {
			change[changeI] = newRRs[newI]
			changeI++
		}
		newI++
	}

	return change
}

// buildUpdateMsg generates sequence of DSO Push Update messages.
//
// Non-nil error signals fatal condition that should abort session.
// Yielded buffer is owned by iterator.
func buildUpdateMsg(change []dns.RR) iter.Seq2[[]byte, error] {
	return func(yield func([]byte, error) bool) {
		if len(change) == 0 {
			return
		}

		poolBuf := updateMsgPool.Get().(*[]byte)
		defer updateMsgPool.Put(poolBuf)

		// Canges are chunked, poolBuf limits chunk size.
		// Buffer is only ever grown in rare event when RR
		// is so large it cannot fit.

		var (
			builder = dsomessage.NewBuilder(*poolBuf).
				EnableLengthPrefix().
				EnableCompression()

			i     int
			retry bool
		)
		for i < len(change) {
			n, err := builder.WritePushChange(change[i:])
			packErr, _ := errors.AsType[*dsomessage.PackingError](err)
			switch {
			case err != nil && packErr == nil: // unexpected error
				yield(nil, err)
				return
			case packErr != nil && n == 0 && retry: // packing failure unrelaed to buffer size
				packErr.Index += i
				yield(nil, packErr)
				return
			case packErr != nil && n == 0: // assume packing error due to short buffer
				tlvLen := dsomessage.TLVHeaderLen + dsomessage.RRLen(change[i])
				if builder.Len()+tlvLen >= dsomessage.MaxPushMsgLen {
					packErr.Index += i
					yield(nil, packErr)
					return
				}
				builder.Grow(tlvLen)
				retry = true
			case packErr != nil: // some RRs were written, but there are more
				msg, _ := builder.Message()
				if !yield(msg, nil) {
					return
				}
				builder.Clear()
				i += packErr.Index
				retry = false
			default: // final chunk
				msg, _ := builder.Message()
				yield(msg, nil)
				return
			}
		}
	}
}

var updateMsgPool = sync.Pool{
	New: func() any {
		b := make([]byte, dsomessage.LengthPrefixLen+5*dsomessage.TLSBlockLen) // must be less than [dsomessage.MaxPushMsgLen]
		return &b
	},
}
