package dsosession

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/coredns/coredns/plugin/dso/internal/dsomessage"
	"github.com/google/go-cmp/cmp"
	"github.com/miekg/dns"
)

// testPush is utility wrapper over [Push].
//
// Use with [testing/synctest].
type testPush struct {
	*Push

	zone    sync.Map
	mu      sync.Mutex
	changes [][]dns.RR

	writeErr    error
	writeDelay  time.Duration
	lookupDelay time.Duration
}

func newTestPush(classes, types []uint16) *testPush {
	if classes == nil {
		classes = []uint16{dns.ClassINET}
	}
	if types == nil {
		types = []uint16{dns.TypeA}
	}
	return &testPush{
		Push: NewPush(classes, types),
	}
}

func cloneRRSet(rrs []dns.RR) (rrs1 []dns.RR) {
	if rrs == nil {
		return nil
	}
	rrs1 = make([]dns.RR, len(rrs))
	for i := range rrs {
		rrs1[i] = dns.Copy(rrs[i])
	}
	return rrs1
}

// Write implements [io.Writer].
func (push *testPush) Write(msg []byte) (int, error) {
	time.Sleep(push.writeDelay)
	if push.writeErr != nil {
		return 0, push.writeErr
	}
	m, err := dsomessage.UnpackMsg(msg[dsomessage.LengthPrefixLen:], dsomessage.OriginServer)
	if err != nil {
		panic("TestPushSession.WriteDSO: failed to unpack")
	}
	tlv := m.TLV[0].(*dsomessage.Push)

	push.mu.Lock()
	defer push.mu.Unlock()

	push.changes = append(push.changes, cloneRRSet(tlv.Change))
	return len(msg), nil
}

// Lookup implements [PushLookuper.LookupPushSubscription].
func (push *testPush) LookupPushSubscription(ctx context.Context, tlv dsomessage.Subscribe) (rrs []dns.RR, ok bool) {
	select {
	case <-time.After(push.lookupDelay):
	case <-ctx.Done():
	}
	if ctx.Err() != nil {
		return nil, false
	}
	q := dns.Question{Name: tlv.Name, Qtype: tlv.RRType, Qclass: tlv.Class}
	if rrs, ok := push.zone.Load(q); ok {
		return rrs.([]dns.RR), true
	}
	return nil, true
}

func (push *testPush) start(tb testing.TB, ctx context.Context, debounceDelay, refreshInterval time.Duration) <-chan error { //nolint:revive
	tb.Helper()

	doneC := make(chan error, 1)
	go func() {
		defer close(doneC)
		doneC <- push.Serve(ctx, push, push, debounceDelay, refreshInterval)
	}()
	tb.Cleanup(func() {
		<-doneC
	})
	return doneC
}

// assertStart is like start but checks returned error during test cleanup.
func (push *testPush) assertStart(tb testing.TB, debounceDelay, refreshInterval time.Duration) {
	tb.Helper()

	doneC := push.start(tb, tb.Context(), debounceDelay, refreshInterval)
	tb.Cleanup(func() {
		if err := <-doneC; err != context.Canceled {
			tb.Errorf("Got Serve()=%v, want %v", err, context.Canceled)
		}
	})
}

func (push *testPush) assertAdd(tb testing.TB, id uint16, tlv dsomessage.Subscribe) {
	tb.Helper()

	sub, rcode, err := push.CanAdd(id, tlv)
	if err != nil {
		tb.Fatalf("Got CanAdd(%v, \"%v\")=%v", id, tlv, err)
	}
	if rcode != dns.RcodeSuccess {
		tb.Fatalf("Got CanAdd(%v, \"%v\")=%v", id, tlv, rcode)
	}

	push.Add(id, sub)

	var b strings.Builder
	for key1 := range sub.key.expand(push.classes, push.types) {
		if _, ok := push.expanded[key1]; !ok {
			b.WriteString("- ")
			b.WriteString(dsomessage.Subscribe(key1).String())
			b.WriteString("\n")
		}
	}
	if b.Len() > 0 {
		tb.Fatal(b.String())
	}
}

func (push *testPush) assertRemove(tb testing.TB, id uint16) {
	tb.Helper()

	key, exists := push.allByID[id]

	var mustRemove []pushSubscribeKey
	if exists {
		for key1 := range key.expand(push.classes, push.types) {
			if rrs, ok := push.expanded[key1]; ok && rrs.refcount == 1 {
				mustRemove = append(mustRemove, key1)
			}
		}
	}

	push.Remove(dsomessage.Unsubscribe{SubscribeID: id})

	if exists {
		if _, ok := push.allByID[id]; ok {
			tb.Fatalf("+ %v", key)
		}
		if _, ok := push.all[key]; ok {
			tb.Fatalf("+ %v", key)
		}
	}

	var b strings.Builder
	for _, key1 := range mustRemove {
		if _, ok := push.expanded[key1]; ok {
			b.WriteString("+ ")
			b.WriteString(dsomessage.Subscribe(key1).String())
			b.WriteString("\n")
		}
	}
	if b.Len() > 0 {
		tb.Fatal(b.String())
	}
}

func (push *testPush) assertChanges(tb testing.TB, changes ...[]dns.RR) {
	tb.Helper()

	synctest.Wait()

	push.mu.Lock()
	defer push.mu.Unlock()

	var (
		b    strings.Builder
		i    int
		diff string
	)
	for ; i < len(changes); i++ {
		if i < len(push.changes) {
			diff = cmp.Diff(changes[i], push.changes[i], cmp.Comparer(func(a, b dns.RR) bool {
				return a.Header().Ttl == b.Header().Ttl && a.Header().Name == b.Header().Name && dns.IsDuplicate(a, b)
			}))
		} else {
			diff = cmp.Diff(changes[i], []dns.RR{})
		}
		if len(diff) > 0 {
			fmt.Fprintf(&b, "%d:\n%s", i, diff)
		}
	}
	for ; i < len(push.changes); i++ {
		diff = cmp.Diff([]dns.RR{}, push.changes[i])
		fmt.Fprintf(&b, "%d:\n%s", i, diff)
	}
	if b.Len() > 0 {
		tb.Errorf("Bad changes:\n%v", b.String())
	}
}

// newRRf formats and creates RR with corresponding [dns.Question] and [dsomessage.Subscribe].
func newRRf(format string, a ...any) (q dns.Question, rr dns.RR, tlv dsomessage.Subscribe) {
	rr, err := dns.NewRR(fmt.Sprintf(format, a...))
	if err != nil {
		panic(err)
	}
	return rrToQuestion(rr), rr, rrToSubscribe(rr)
}

func rrToPushRemoval(rr dns.RR) (rr1 dns.RR) {
	rr1 = dns.Copy(rr)
	rr1.Header().Ttl = dsomessage.PushTTLRemove
	return rr1
}

func rrToQuestion(rr dns.RR) dns.Question {
	h := rr.Header()
	return dns.Question{Name: strings.ToLower(h.Name), Qtype: h.Rrtype, Qclass: h.Class}
}

func rrToSubscribe(rr dns.RR) dsomessage.Subscribe {
	h := rr.Header()
	return dsomessage.Subscribe{Name: h.Name, RRType: h.Rrtype, Class: h.Class}
}

func rrToReconfirm(rr dns.RR) dsomessage.Reconfirm {
	return dsomessage.Reconfirm{RR: dns.Copy(rr)}
}

func TestPushAdd(t *testing.T) {
	t.Parallel()

	push := newTestPush(nil, nil)

	tlv := dsomessage.Subscribe{Name: "a.test.", RRType: dns.TypeA, Class: dns.ClassINET}
	push.assertAdd(t, 1, tlv)

	tlv1 := dsomessage.Subscribe{Name: "b.test.", RRType: dns.TypeA, Class: dns.ClassINET}
	sub := PushSubscribe{Subscribe: tlv1}
	push.Add(2, sub)
	if _, ok := push.allByID[2]; ok {
		t.Errorf("Got subscription, want none")
	}
}

func TestPushAddWildcard(t *testing.T) {
	t.Parallel()

	push := newTestPush([]uint16{dns.ClassINET, dns.ClassCHAOS}, []uint16{dns.TypeA, dns.TypeAAAA})

	tlv := dsomessage.Subscribe{Name: "test.", RRType: dns.TypeANY, Class: dns.ClassANY}
	push.assertAdd(t, 1, tlv)
}

func TestPushRemove(t *testing.T) {
	t.Parallel()

	push := newTestPush(nil, nil)

	tlv := dsomessage.Subscribe{Name: "test.", RRType: dns.TypeA, Class: dns.ClassINET}
	push.assertAdd(t, 1, tlv)
	push.assertRemove(t, 1)
	push.assertRemove(t, 1)
}

func TestPushIsActive(t *testing.T) {
	t.Parallel()

	push := newTestPush(nil, nil)

	tlv := dsomessage.Subscribe{Name: "test.", RRType: dns.TypeA, Class: dns.ClassINET}
	push.assertAdd(t, 1, tlv)
	if !push.IsActive() {
		t.Error("Got IsActive()=false, want true")
	}

	push.assertRemove(t, 1)
	if push.IsActive() {
		t.Error("Got IsActive()=true, want false")
	}
}

func TestPushCanAdd(t *testing.T) {
	t.Parallel()

	push := newTestPush([]uint16{dns.ClassINET}, []uint16{dns.TypeA, dns.TypeAAAA})
	tlv := dsomessage.Subscribe{Name: "test.", RRType: dns.TypeA, Class: dns.ClassINET}

	_, rcode, _ := push.CanAdd(1, tlv)
	if rcode != dns.RcodeSuccess {
		t.Fatalf("Got CanAdd(%v, %v)=%v, want RcodeSuccess", 1, tlv, rcode)
	}
	push.assertAdd(t, 1, tlv)

	_, _, err := push.CanAdd(2, tlv)
	if err != ErrDuplicateSub {
		t.Errorf("Got CanAdd(%v, %v)=%v, want ErrDuplicateSub", 2, tlv, err)
	}

	tlv.RRType = dns.TypeAAAA
	_, _, err = push.CanAdd(1, tlv)
	if err != ErrDuplicateSub {
		t.Errorf("Got CanAdd(%v, %v)=%v, want ErrDuplicateSub", 1, tlv, err)
	}

	tlv.RRType = dns.TypeTXT
	_, rcode, _ = push.CanAdd(3, tlv)
	if rcode != dns.RcodeRefused {
		t.Errorf("Got CanAdd(%v, %v)=%v, want RcodeRefused", 3, tlv, rcode)
	}

	tlv.Class = dns.ClassCHAOS
	tlv.RRType = dns.TypeA
	_, rcode, _ = push.CanAdd(3, tlv)
	if rcode != dns.RcodeRefused {
		t.Errorf("Got CanAdd(%v, %v)=%v, want RcodeRefused", 3, tlv, rcode)
	}
}

func TestPushRefcount(t *testing.T) {
	t.Parallel()

	push := newTestPush(nil, nil)

	tlv := dsomessage.Subscribe{Name: "test.", RRType: dns.TypeA, Class: dns.ClassINET}
	push.assertAdd(t, 1, tlv)
	tlvAny := dsomessage.Subscribe{Name: "tEsT.", RRType: dns.TypeANY, Class: dns.ClassANY}
	push.assertAdd(t, 2, tlvAny)

	key := push.allByID[1]
	if rrs := push.expanded[key]; rrs.refcount != 2 {
		t.Errorf("Got refcount=%v, want 2", rrs.refcount)
	}

	push.assertRemove(t, 1)
	if _, ok := push.expanded[key]; !ok {
		t.Fatalf("Got refcount=0, want 1")
	}
	push.assertRemove(t, 2)
	if _, ok := push.expanded[key]; ok {
		t.Errorf("Got refcount>0, want 0")
	}
}

func TestPushRefcountNameFolding(t *testing.T) {
	t.Parallel()

	push := newTestPush(nil, nil)

	push.assertAdd(t, 1, dsomessage.Subscribe{Name: "test.", RRType: dns.TypeANY, Class: dns.ClassANY})
	push.assertAdd(t, 2, dsomessage.Subscribe{Name: "TEST.", RRType: dns.TypeA, Class: dns.ClassANY})
	push.assertAdd(t, 3, dsomessage.Subscribe{Name: "tEsT.", RRType: dns.TypeANY, Class: dns.ClassINET})
	push.assertAdd(t, 4, dsomessage.Subscribe{Name: "TeSt.", RRType: dns.TypeA, Class: dns.ClassINET})

	if rrs := push.expanded[pushSubscribeKey{Name: "test.", RRType: dns.TypeA, Class: dns.ClassINET}]; rrs.refcount != 4 {
		t.Errorf("Got refcount=%v, want 4", rrs.refcount)
	}
}

func TestPushUpdate(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		push := newTestPush(nil, nil)
		push.assertStart(t, 0, 0)

		q, rr, tlv := newRRf("test. IN A 192.0.2.1")
		push.zone.Store(q, []dns.RR{rr})
		push.assertAdd(t, 1, tlv)

		synctest.Wait()

		push.zone.Store(q, []dns.RR{})
		push.Refresh()

		synctest.Wait()

		_, rr1, _ := newRRf("test. IN A 192.0.2.2")
		push.zone.Store(q, []dns.RR{rr1})
		push.Refresh()

		push.assertChanges(t,
			[]dns.RR{rr},
			[]dns.RR{rrToPushRemoval(rr)},
			[]dns.RR{rr1},
		)
	})
}

func TestPushUpdateUsesSubscriptionName(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		push := newTestPush(nil, nil)
		push.assertStart(t, 0, 0)

		q, _, tlv := newRRf("tEsT. IN A 192.0.2.1")
		_, rr1, _ := newRRf("test. IN A 192.0.2.1")
		_, rr2, _ := newRRf("TEST. IN A 192.0.2.2")
		push.zone.Store(q, []dns.RR{rr1, rr2})

		_, wantRR1, _ := newRRf("tEsT. IN A 192.0.2.1")
		_, wantRR2, _ := newRRf("tEsT. IN A 192.0.2.2")
		push.assertAdd(t, 1, tlv)
		push.assertChanges(t,
			[]dns.RR{wantRR1, wantRR2},
		)
	})
}

func TestPushReconfirm(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		push := newTestPush(nil, nil)
		push.assertStart(t, 0, 0)

		q, rr, tlv := newRRf("test. IN A 192.0.2.1")
		push.zone.Store(q, []dns.RR{rr})
		push.assertAdd(t, 1, tlv)

		synctest.Wait()

		_, rr1, _ := newRRf("test. IN A 192.0.2.2")
		push.zone.Store(q, []dns.RR{rr1})
		push.Reconfirm(rrToReconfirm(rr))

		synctest.Wait()

		q2, rr2, _ := newRRf("b.test IN A 192.0.2.2")
		push.zone.Store(q2, []dns.RR{rr2})
		push.Reconfirm(rrToReconfirm(rr2))

		push.assertChanges(t,
			[]dns.RR{rr},
			[]dns.RR{rrToPushRemoval(rr), rr1},
		)
	})
}

func TestPushReconfirmNoBlock(t *testing.T) {
	t.Parallel()

	push := newTestPush(nil, nil)
	_, rr, tlv := newRRf("test. IN A 192.0.2.1")
	push.assertAdd(t, 1, tlv)

	push.Reconfirm(rrToReconfirm(rr))
	push.Reconfirm(rrToReconfirm(rr))
}

func TestPushDebounce(t *testing.T) {
	t.Parallel()

	const debounceDelay = time.Minute

	synctest.Test(t, func(t *testing.T) {
		push := newTestPush(nil, nil)
		push.assertStart(t, debounceDelay, 0)

		q, rr, tlv := newRRf("test. IN A 192.0.2.1")
		push.zone.Store(q, []dns.RR{rr})
		push.assertAdd(t, 1, tlv)

		time.Sleep(debounceDelay / 2)
		push.assertChanges(t)

		time.Sleep(debounceDelay / 2)
		push.assertChanges(t,
			[]dns.RR{rr},
		)
	})
}

func TestPushRefresh(t *testing.T) {
	t.Parallel()

	const refreshInterval = time.Minute

	tcs := []struct {
		name        string
		triggerFunc func(push *testPush)
	}{
		{
			"manual",
			func(push *testPush) {
				push.Refresh()
			},
		},
		{
			"auto",
			func(*testPush) {
				time.Sleep(refreshInterval)
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				push := newTestPush(nil, nil)
				push.assertStart(t, 0, refreshInterval)

				q, rr, tlv := newRRf("test. IN A 192.0.2.1")
				push.zone.Store(q, []dns.RR{rr})
				push.assertAdd(t, 1, tlv)

				synctest.Wait()

				_, rr1, _ := newRRf("test. IN A 192.0.2.2")
				push.zone.Store(q, []dns.RR{rr, rr1})
				tc.triggerFunc(push)

				push.assertChanges(t,
					[]dns.RR{rr},
					[]dns.RR{rr1},
				)
			})
		})
	}
}

func TestPushRefreshNoBlock(t *testing.T) {
	t.Parallel()

	push := newTestPush(nil, nil)
	push.Refresh()
	push.Refresh()
}

func TestPushSplitLongChange(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		push := newTestPush([]uint16{dns.ClassINET}, []uint16{dns.TypeTXT})
		push.assertStart(t, 0, 0)

		q, rr, tlv := newRRf("test. IN TXT (%s)", strings.Repeat("\""+strings.Repeat("X", 255)+"\" ", 62))
		push.zone.Store(q, []dns.RR{rr, rr, rr, rr, rr})
		push.assertAdd(t, 1, tlv)
		push.assertChanges(t,
			[]dns.RR{rr},
			[]dns.RR{rr},
			[]dns.RR{rr},
			[]dns.RR{rr},
			[]dns.RR{rr},
		)
	})
}

func TestPushAbortOnTooLongRR(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		push := newTestPush([]uint16{dns.ClassINET}, []uint16{dns.TypeTXT})
		doneC := push.start(t, t.Context(), 0, 0)

		q, rr, tlv := newRRf("test. IN TXT (%s)", strings.Repeat("\""+strings.Repeat("X", 255)+"\" ", 255))
		push.zone.Store(q, []dns.RR{rr})
		push.assertAdd(t, 1, tlv)

		err := <-doneC
		var packingErr *dsomessage.PackingError
		if ok := errors.As(err, &packingErr); !ok {
			t.Errorf("Got Serve()=%v, want PackingError", err)
		}
	})
}

func TestPushAbortOnInvalidRR(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		push := newTestPush([]uint16{dns.ClassINET}, []uint16{dns.TypeAPL})
		doneC := push.start(t, t.Context(), 0, 0)

		invalidRR := &dns.APL{
			Hdr: dns.RR_Header{
				Name:   "test.",
				Rrtype: dns.TypeAPL,
				Class:  dns.ClassINET,
				Ttl:    600,
			},
			Prefixes: []dns.APLPrefix{
				{
					Negation: true,
					Network: net.IPNet{
						IP:   net.ParseIP("2001:db8::1"),
						Mask: net.CIDRMask(24, 32),
					},
				},
			},
		}
		push.zone.Store(rrToQuestion(invalidRR), []dns.RR{invalidRR})
		push.assertAdd(t, 1, rrToSubscribe(invalidRR))

		err := <-doneC
		var packingErr *dsomessage.PackingError
		if ok := errors.As(err, &packingErr); !ok {
			t.Errorf("Got Serve()=%v, want PackingError", err)
		}
	})
}

func TestPushAbortOnWriteError(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		push := newTestPush(nil, nil)
		push.writeErr = errors.New("test")
		doneC := push.start(t, t.Context(), 0, 0)

		q, rr, tlv := newRRf("test. IN A 192.0.2.1")
		push.zone.Store(q, []dns.RR{rr})
		push.assertAdd(t, 1, tlv)

		err := <-doneC
		if err != push.writeErr {
			t.Errorf("Got Serve()=%v, want %v", err, push.writeErr)
		}
	})
}

func TestPushCancelDebounce(t *testing.T) {
	t.Parallel()

	const debounceDelay = time.Minute

	synctest.Test(t, func(t *testing.T) {
		push := newTestPush(nil, nil)

		ctx, cancel := context.WithCancel(t.Context())
		doneC := push.start(t, ctx, debounceDelay, 0)

		q, rr, tlv := newRRf("test. IN A 192.0.2.1")
		push.zone.Store(q, []dns.RR{rr})
		push.assertAdd(t, 1, tlv)

		synctest.Wait()

		time.Sleep(debounceDelay / 2)
		cancel()

		synctest.Wait()

		time.Sleep(debounceDelay / 2)
		push.assertChanges(t)

		err := <-doneC
		if err != context.Canceled {
			t.Errorf("Expected Serve() error %v, got %v", context.Canceled, err)
		}
	})
}

func TestPushCancelRefresh(t *testing.T) {
	t.Parallel()

	const refreshInterval = time.Minute

	synctest.Test(t, func(t *testing.T) {
		push := newTestPush(nil, nil)
		ctx, cancel := context.WithCancel(t.Context())
		doneC := push.start(t, ctx, 0, refreshInterval)

		q, rr, tlv := newRRf("test. IN A 192.0.2.1")
		push.zone.Store(q, []dns.RR{rr})
		push.assertAdd(t, 1, tlv)

		synctest.Wait()

		push.lookupDelay = 1
		_, rr1, _ := newRRf("test. IN A 192.0.2.2")
		push.zone.Store(q, []dns.RR{rr, rr1})

		time.Sleep(refreshInterval) // refresh started but lookup is delayed
		cancel()

		err := <-doneC
		if err != context.Canceled {
			t.Errorf("Got Serve()=%v, want context.Canceled", err)
		}
		push.assertChanges(t,
			[]dns.RR{rr},
		)
	})
}

func TestPushCancelWrite(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		push := newTestPush([]uint16{dns.ClassINET}, []uint16{dns.TypeTXT})
		push.writeDelay = time.Minute
		ctx, cancel := context.WithCancel(t.Context())
		doneC := push.start(t, ctx, 0, 0)

		q, rr, tlv := newRRf("test. IN TXT (%s)", strings.Repeat("\""+strings.Repeat("X", 255)+"\" ", 62))
		push.zone.Store(q, []dns.RR{rr, rr, rr, rr, rr})
		push.assertAdd(t, 1, tlv)

		synctest.Wait()

		time.Sleep(push.writeDelay)
		cancel()

		err := <-doneC
		if err != context.Canceled {
			t.Errorf("Got Serve()=%v, want context.Canceled", err)
		}
		push.assertChanges(t,
			[]dns.RR{rr},
		)
	})
}

func TestPushCancelLookup(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		push := newTestPush(nil, nil)
		push.lookupDelay = time.Minute
		ctx, cancel := context.WithCancel(t.Context())
		doneC := push.start(t, ctx, 0, 0)

		for i, s := range []string{"a.test. IN A 192.0.2.1", "b.test. IN A 192.0.2.1"} {
			q, rr, tlv := newRRf("%s", s)
			push.zone.Store(q, []dns.RR{rr})
			push.assertAdd(t, uint16(i+1), tlv)
		}

		synctest.Wait()

		time.Sleep(push.lookupDelay)
		cancel()

		err := <-doneC
		if err != context.Canceled {
			t.Errorf("Got Serve()=%v, want context.Canceled", err)
		}
		push.assertChanges(t)
	})
}

func TestRRsetUpdate(t *testing.T) {
	t.Parallel()

	var (
		rrs = rrSet{name: "test."}

		rr, _  = dns.NewRR("test. IN A 192.0.2.1")
		rr1, _ = dns.NewRR("test. IN A 192.0.2.2")
		rr2, _ = dns.NewRR("test. IN A 192.0.2.3")
		rr3, _ = dns.NewRR("test. IN A 192.0.2.4")
		rr4, _ = dns.NewRR("test. In A 192.0.2.5")
	)
	tcs := []struct {
		name       string
		new        []dns.RR
		wantChange []dns.RR
	}{
		{
			"add all",
			[]dns.RR{rr, rr1, rr2, rr3, rr4},
			[]dns.RR{rr, rr1, rr2, rr3, rr4},
		},
		{
			"remove none",
			[]dns.RR{rr, rr1, rr2, rr3, rr4},
			nil,
		},
		{
			"remove head",
			[]dns.RR{rr3, rr4},
			[]dns.RR{rrToPushRemoval(rr), rrToPushRemoval(rr1), rrToPushRemoval(rr2)},
		},
		{
			"add head",
			[]dns.RR{rr, rr1, rr2, rr3, rr4},
			[]dns.RR{rr, rr1, rr2},
		},
		{
			"remove middle",
			[]dns.RR{rr, rr4},
			[]dns.RR{rrToPushRemoval(rr1), rrToPushRemoval(rr2), rrToPushRemoval(rr3)},
		},
		{
			"add middle",
			[]dns.RR{rr, rr1, rr2, rr3, rr4},
			[]dns.RR{rr1, rr2, rr3},
		},
		{
			"remove tail",
			[]dns.RR{rr, rr1},
			[]dns.RR{rrToPushRemoval(rr2), rrToPushRemoval(rr3), rrToPushRemoval(rr4)},
		},
		{
			"add tail",
			[]dns.RR{rr, rr1, rr2, rr3, rr4},
			[]dns.RR{rr2, rr3, rr4},
		},
		{
			"remove sparse",
			[]dns.RR{rr, rr2, rr4},
			[]dns.RR{rrToPushRemoval(rr1), rrToPushRemoval(rr3)},
		},
		{
			"add sparse",
			[]dns.RR{rr, rr1, rr2, rr3, rr4},
			[]dns.RR{rr1, rr3},
		},
		{
			"remove all",
			[]dns.RR{},
			[]dns.RR{rrToPushRemoval(rr), rrToPushRemoval(rr1), rrToPushRemoval(rr2), rrToPushRemoval(rr3), rrToPushRemoval(rr4)},
		},
		{
			"remove none",
			[]dns.RR{},
			nil,
		},
	}
	for _, tc := range tcs {
		change := rrs.update(tc.new)
		if diff := cmp.Diff(tc.wantChange, change); len(diff) > 0 {
			t.Fatalf("%q mismatch:\n%s", tc.name, diff)
		}
	}
}

func TestRRsetUpdateReuseRR(t *testing.T) {
	t.Parallel()

	rrs := rrSet{name: "test."}

	rr, _ := dns.NewRR("test. 3600 IN A 192.0.2.1")
	rrs.update([]dns.RR{rr})
	if rrs.v[0] == rr {
		t.Errorf("Want to copy RR")
	}

	rr.Header().Ttl = 42
	if got := rrs.v[0].Header().Ttl; got != 3600 {
		t.Errorf("Got Ttl=%v, want 3600", got)
	}

	rr1, _ := dns.NewRR("test. IN A 192.0.2.1")
	old := rrs.v[0]
	rrs.update([]dns.RR{rr1})
	if rrs.v[0] != old {
		t.Error("Want to reuse RR")
	}
}

func TestPushSubscribeKeyExpand(t *testing.T) {
	t.Parallel()

	types := []uint16{dns.TypeA, dns.TypeAAAA}
	classes := []uint16{dns.ClassINET, dns.ClassCHAOS}

	tcs := []struct {
		name string
		key  pushSubscribeKey
		want []pushSubscribeKey
	}{
		{"neither", pushSubscribeKey{Name: "test.", RRType: dns.TypeA, Class: dns.ClassINET}, []pushSubscribeKey{
			{Name: "test.", RRType: dns.TypeA, Class: dns.ClassINET},
		}},
		{"type", pushSubscribeKey{Name: "test.", RRType: dns.TypeANY, Class: dns.ClassINET}, []pushSubscribeKey{
			{Name: "test.", RRType: dns.TypeA, Class: dns.ClassINET},
			{Name: "test.", RRType: dns.TypeAAAA, Class: dns.ClassINET}},
		},
		{"class", pushSubscribeKey{Name: "test.", RRType: dns.TypeA, Class: dns.ClassANY}, []pushSubscribeKey{
			{Name: "test.", RRType: dns.TypeA, Class: dns.ClassINET},
			{Name: "test.", RRType: dns.TypeA, Class: dns.ClassCHAOS},
		}},
		{"both", pushSubscribeKey{Name: "test.", RRType: dns.TypeANY, Class: dns.ClassANY}, []pushSubscribeKey{
			{Name: "test.", RRType: dns.TypeA, Class: dns.ClassINET},
			{Name: "test.", RRType: dns.TypeAAAA, Class: dns.ClassINET},
			{Name: "test.", RRType: dns.TypeA, Class: dns.ClassCHAOS},
			{Name: "test.", RRType: dns.TypeAAAA, Class: dns.ClassCHAOS},
		}},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := slices.Collect(tc.key.expand(classes, types))
			if diff := cmp.Diff(tc.want, got); len(diff) > 0 {
				t.Errorf("Expansion mismatch:\n%s", diff)
			}
		})
	}
}
