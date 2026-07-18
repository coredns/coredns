package secondary

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/file"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/fall"
	plugintest "github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestTransferInCatalog(t *testing.T) {
	const origin = "catalog.example."
	zones := newTestTransferZones(map[string][]dns.RR{
		origin:         catalogZoneRecords(t, true),
		"example.org.": memberZoneRecords(t),
	})

	server := dnstest.NewServer(zones.handler())
	defer server.Close()

	z := file.NewZone(origin, "stdin")
	z.TransferFrom = []string{server.Addr}
	s := newTestSecondary(origin, z, true)
	t.Cleanup(s.stopDynamicZones)

	if err := s.transferIn(origin, z, nil); err != nil {
		t.Fatalf("transferIn returned error: %v", err)
	}

	s.catalogMu.RLock()
	cat := s.catalogs[origin]
	s.catalogMu.RUnlock()
	if cat == nil {
		t.Fatal("expected parsed catalog")
	}
	if len(cat.Members) != 1 {
		t.Fatalf("expected 1 catalog member, got %d", len(cat.Members))
	}
	if cat.Members[0].Zone != "example.org." {
		t.Fatalf("expected member zone example.org., got %q", cat.Members[0].Zone)
	}
	if strings.Join(cat.Members[0].Groups, ",") != "default" {
		t.Fatalf("expected default member group, got %v", cat.Members[0].Groups)
	}
	if z.SOA == nil {
		t.Fatal("expected zone data to be live after valid catalog transfer")
	}

	msg := waitForAnswer(t, s, "www.example.org.", dns.TypeA)
	if len(msg.Answer) != 1 {
		t.Fatalf("expected 1 member zone answer, got %d", len(msg.Answer))
	}
	a, ok := msg.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("expected A answer, got %T", msg.Answer[0])
	}
	if a.A.String() != "192.0.2.1" {
		t.Fatalf("expected 192.0.2.1, got %s", a.A.String())
	}
}

func TestTransferInCatalogRestrictsMemberZones(t *testing.T) {
	const (
		origin  = "catalog.example."
		allowed = "tenant.allowed.example."
		denied  = "tenant.denied.example."
	)
	zones := newTestTransferZones(map[string][]dns.RR{
		origin: catalogZoneRecordsForMembers(t, origin, []catalogRecordMember{
			{id: "a", zone: allowed},
			{id: "b", zone: denied},
		}, 1),
		allowed: memberZoneRecordsFor(t, allowed, 1, "192.0.2.1"),
		denied:  memberZoneRecordsFor(t, denied, 1, "192.0.2.2"),
	})

	server := dnstest.NewServer(zones.handler())
	defer server.Close()

	z := file.NewZone(origin, "stdin")
	z.TransferFrom = []string{server.Addr}
	s := newTestSecondary(origin, z, true)
	s.catalogZones[origin] = plugin.Zones{"allowed.example."}
	t.Cleanup(s.stopDynamicZones)

	if err := s.transferIn(origin, z, nil); err != nil {
		t.Fatalf("transferIn returned error: %v", err)
	}
	waitForAnswer(t, s, "www."+allowed, dns.TypeA)

	s.zoneMu.RLock()
	_, allowedDynamic := s.dynamicZones[allowed]
	_, deniedDynamic := s.dynamicZones[denied]
	_, admittedDenied := s.catalogMemberZones[origin][denied]
	s.zoneMu.RUnlock()
	if !allowedDynamic {
		t.Fatalf("expected member zone %s to match configured suffix", allowed)
	}
	if deniedDynamic || admittedDenied {
		t.Fatalf("expected member zone %s to be rejected", denied)
	}
	if _, _, ok := s.lookupZone("www." + denied); ok {
		t.Fatalf("expected rejected member zone %s not to be served", denied)
	}
}

func TestTransferInCatalogRemovesMemberZone(t *testing.T) {
	const origin = "catalog.example."
	zones := newTestTransferZones(map[string][]dns.RR{
		origin:         catalogZoneRecords(t, true),
		"example.org.": memberZoneRecords(t),
	})

	server := dnstest.NewServer(zones.handler())
	defer server.Close()

	z := file.NewZone(origin, "stdin")
	z.TransferFrom = []string{server.Addr}
	s := newTestSecondary(origin, z, true)
	t.Cleanup(s.stopDynamicZones)

	if err := s.transferIn(origin, z, nil); err != nil {
		t.Fatalf("transferIn returned error: %v", err)
	}
	waitForAnswer(t, s, "www.example.org.", dns.TypeA)

	zones.set(origin, catalogZoneRecordsWithoutMembers(t))
	if err := s.transferIn(origin, z, nil); err != nil {
		t.Fatalf("transferIn returned error: %v", err)
	}
	if _, _, ok := s.lookupZone("www.example.org."); ok {
		t.Fatal("expected member zone to be removed after catalog update")
	}
}

func TestTransferInCatalogResetsMemberZoneOnIDChange(t *testing.T) {
	const origin = "catalog.example."
	zones := newTestTransferZones(map[string][]dns.RR{
		origin:         catalogZoneRecordsWithMemberID(t, "a", 1),
		"example.org.": memberZoneRecordsWithAddress(t, 1, "192.0.2.1"),
	})

	server := dnstest.NewServer(zones.handler())
	defer server.Close()

	z := file.NewZone(origin, "stdin")
	z.TransferFrom = []string{server.Addr}
	s := newTestSecondary(origin, z, true)
	t.Cleanup(s.stopDynamicZones)

	if err := s.transferIn(origin, z, nil); err != nil {
		t.Fatalf("initial transferIn returned error: %v", err)
	}
	waitForAnswer(t, s, "www.example.org.", dns.TypeA)

	s.zoneMu.RLock()
	before := s.Z["example.org."]
	beforeDynamic := s.dynamicZones["example.org."]
	s.zoneMu.RUnlock()
	if before == nil || beforeDynamic == nil {
		t.Fatal("expected initial catalog member zone and dynamic state")
	}

	zones.set(origin, catalogZoneRecordsWithMemberID(t, "a", 2))
	if err := s.transferIn(origin, z, nil); err != nil {
		t.Fatalf("same-ID transferIn returned error: %v", err)
	}
	s.zoneMu.RLock()
	sameIDZone := s.Z["example.org."]
	sameIDDynamic := s.dynamicZones["example.org."]
	s.zoneMu.RUnlock()
	if sameIDZone != before || sameIDDynamic != beforeDynamic {
		t.Fatal("expected unchanged member ID to preserve the dynamic zone")
	}
	select {
	case <-beforeDynamic.shutdown:
		t.Fatal("expected unchanged member ID to preserve the transfer loop")
	default:
	}

	zones.set(origin, catalogZoneRecordsWithMemberID(t, "b", 3))
	zones.set("example.org.", memberZoneRecordsWithAddress(t, 2, "192.0.2.2"))
	if err := s.transferIn(origin, z, nil); err != nil {
		t.Fatalf("updated transferIn returned error: %v", err)
	}

	s.zoneMu.RLock()
	after := s.Z["example.org."]
	afterDynamic := s.dynamicZones["example.org."]
	s.zoneMu.RUnlock()
	if before == after {
		t.Fatal("expected member node ID change to replace the dynamic zone")
	}
	if afterDynamic == nil || afterDynamic.memberID != "b" {
		t.Fatalf("expected active member ID b, got %+v", afterDynamic)
	}
	select {
	case <-beforeDynamic.shutdown:
	default:
		t.Fatal("expected member node ID change to stop the old transfer loop")
	}

	msg := waitForAnswer(t, s, "www.example.org.", dns.TypeA)
	a, ok := msg.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("expected A answer, got %T", msg.Answer[0])
	}
	if a.A.String() != "192.0.2.2" {
		t.Fatalf("expected reset zone answer 192.0.2.2, got %s", a.A.String())
	}
}

func TestTransferInCatalogMigratesMemberZoneWithSameID(t *testing.T) {
	fixture := newCatalogMigrationFixture(t, "a", "a", "new.catalog.example.", true)

	if err := fixture.s.transferIn(fixture.oldOrigin, fixture.oldCatalog, nil); err != nil {
		t.Fatalf("old catalog transferIn returned error: %v", err)
	}
	waitForAddress(t, fixture.s, fixture.member, "192.0.2.1")

	fixture.s.zoneMu.RLock()
	oldDynamic := fixture.s.dynamicZones[fixture.member]
	fixture.s.zoneMu.RUnlock()
	if oldDynamic == nil {
		t.Fatal("expected member zone to belong to the old catalog")
	}

	if err := fixture.s.transferIn(fixture.newOrigin, fixture.newCatalog, nil); err != nil {
		t.Fatalf("new catalog transferIn returned error: %v", err)
	}

	fixture.s.zoneMu.RLock()
	newDynamic := fixture.s.dynamicZones[fixture.member]
	fixture.s.zoneMu.RUnlock()
	if newDynamic == nil || newDynamic.catalog != fixture.newOrigin || newDynamic.memberID != "a" {
		t.Fatalf("expected member zone to migrate to %s with ID a, got %+v", fixture.newOrigin, newDynamic)
	}
	select {
	case <-oldDynamic.shutdown:
	default:
		t.Fatal("expected migration to stop the old catalog transfer loop")
	}

	// The target transfer is paused, so this answer can only come from state
	// carried over from the old catalog.
	waitForAddress(t, fixture.s, fixture.member, "192.0.2.1")
	fixture.releaseTarget()
	waitForAddress(t, fixture.s, fixture.member, "192.0.2.2")
}

func TestTransferInCatalogRejectsOwnershipMigrationOutsideMemberZones(t *testing.T) {
	fixture := newCatalogMigrationFixture(t, "a", "a", "new.catalog.example.", false)
	fixture.s.catalogZones[fixture.newOrigin] = plugin.Zones{"other.example."}

	if err := fixture.s.transferIn(fixture.oldOrigin, fixture.oldCatalog, nil); err != nil {
		t.Fatalf("old catalog transferIn returned error: %v", err)
	}
	waitForAddress(t, fixture.s, fixture.member, "192.0.2.1")

	fixture.s.zoneMu.RLock()
	before := fixture.s.dynamicZones[fixture.member]
	fixture.s.zoneMu.RUnlock()
	if before == nil || before.catalog != fixture.oldOrigin {
		t.Fatal("expected member zone to belong to the old catalog")
	}

	if err := fixture.s.transferIn(fixture.newOrigin, fixture.newCatalog, nil); err != nil {
		t.Fatalf("new catalog transferIn returned error: %v", err)
	}

	fixture.s.zoneMu.RLock()
	after := fixture.s.dynamicZones[fixture.member]
	_, admitted := fixture.s.catalogMemberZones[fixture.newOrigin][fixture.member]
	fixture.s.zoneMu.RUnlock()
	if after == nil || after != before || after.catalog != fixture.oldOrigin {
		t.Fatalf("expected rejected migration to preserve ownership by %s, got %+v", fixture.oldOrigin, after)
	}
	if admitted {
		t.Fatal("expected target catalog not to admit an out-of-scope member")
	}
	select {
	case <-before.shutdown:
		t.Fatal("expected rejected migration to preserve the old transfer loop")
	default:
	}
}

func TestTransferInCatalogWaitsForTargetUpdateAfterSourceAddsCOO(t *testing.T) {
	fixture := newCatalogMigrationFixture(t, "a", "a", "", true)

	if err := fixture.s.transferIn(fixture.oldOrigin, fixture.oldCatalog, nil); err != nil {
		t.Fatalf("old catalog transferIn returned error: %v", err)
	}
	waitForAddress(t, fixture.s, fixture.member, "192.0.2.1")

	if err := fixture.s.transferIn(fixture.newOrigin, fixture.newCatalog, nil); err != nil {
		t.Fatalf("new catalog transferIn returned error: %v", err)
	}
	fixture.s.zoneMu.RLock()
	before := fixture.s.dynamicZones[fixture.member]
	fixture.s.zoneMu.RUnlock()
	if before == nil || before.catalog != fixture.oldOrigin {
		t.Fatalf("expected missing coo to leave ownership with %s, got %+v", fixture.oldOrigin, before)
	}

	fixture.oldZones.set(fixture.oldOrigin, catalogZoneRecordsFor(t, fixture.oldOrigin, "a", fixture.member, fixture.newOrigin, 2))
	if err := fixture.s.transferIn(fixture.oldOrigin, fixture.oldCatalog, nil); err != nil {
		t.Fatalf("updated old catalog transferIn returned error: %v", err)
	}

	fixture.s.zoneMu.RLock()
	afterSourceUpdate := fixture.s.dynamicZones[fixture.member]
	fixture.s.zoneMu.RUnlock()
	if afterSourceUpdate != before || afterSourceUpdate.catalog != fixture.oldOrigin {
		t.Fatalf("expected source coo update to wait for a target catalog update, got %+v", afterSourceUpdate)
	}
	select {
	case <-before.shutdown:
		t.Fatal("expected source coo update to preserve the old transfer loop")
	default:
	}
	waitForAddress(t, fixture.s, fixture.member, "192.0.2.1")

	fixture.newZones.set(fixture.newOrigin, catalogZoneRecordsFor(t, fixture.newOrigin, "a", fixture.member, "", 2))
	if err := fixture.s.transferIn(fixture.newOrigin, fixture.newCatalog, nil); err != nil {
		t.Fatalf("updated target catalog transferIn returned error: %v", err)
	}

	fixture.s.zoneMu.RLock()
	afterTargetUpdate := fixture.s.dynamicZones[fixture.member]
	fixture.s.zoneMu.RUnlock()
	if afterTargetUpdate == nil || afterTargetUpdate.catalog != fixture.newOrigin {
		t.Fatalf("expected target catalog update to complete migration to %s, got %+v", fixture.newOrigin, afterTargetUpdate)
	}
	select {
	case <-before.shutdown:
	default:
		t.Fatal("expected completed coo handshake to stop the old transfer loop")
	}
	waitForAddress(t, fixture.s, fixture.member, "192.0.2.1")
	fixture.releaseTarget()
	waitForAddress(t, fixture.s, fixture.member, "192.0.2.2")
}

func TestTransferInCatalogRejectsUncoordinatedMigration(t *testing.T) {
	tests := []struct {
		name string
		coo  string
	}{
		{name: "missing coo"},
		{name: "wrong coo", coo: "other.catalog.example."},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCatalogMigrationFixture(t, "a", "a", test.coo, false)

			if err := fixture.s.transferIn(fixture.oldOrigin, fixture.oldCatalog, nil); err != nil {
				t.Fatalf("old catalog transferIn returned error: %v", err)
			}
			waitForAddress(t, fixture.s, fixture.member, "192.0.2.1")

			fixture.s.zoneMu.RLock()
			before := fixture.s.dynamicZones[fixture.member]
			fixture.s.zoneMu.RUnlock()
			if err := fixture.s.transferIn(fixture.newOrigin, fixture.newCatalog, nil); err != nil {
				t.Fatalf("new catalog transferIn returned error: %v", err)
			}

			fixture.s.zoneMu.RLock()
			after := fixture.s.dynamicZones[fixture.member]
			fixture.s.zoneMu.RUnlock()
			if after != before || after == nil || after.catalog != fixture.oldOrigin {
				t.Fatalf("expected uncoordinated migration to preserve old ownership, got before=%+v after=%+v", before, after)
			}
			select {
			case <-before.shutdown:
				t.Fatal("expected uncoordinated migration to preserve the old transfer loop")
			default:
			}
			waitForAddress(t, fixture.s, fixture.member, "192.0.2.1")
		})
	}
}

func TestTransferInCatalogMigrationResetsStateForDifferentID(t *testing.T) {
	fixture := newCatalogMigrationFixture(t, "a", "b", "new.catalog.example.", true)

	if err := fixture.s.transferIn(fixture.oldOrigin, fixture.oldCatalog, nil); err != nil {
		t.Fatalf("old catalog transferIn returned error: %v", err)
	}
	waitForAddress(t, fixture.s, fixture.member, "192.0.2.1")

	fixture.s.zoneMu.RLock()
	oldDynamic := fixture.s.dynamicZones[fixture.member]
	fixture.s.zoneMu.RUnlock()
	if err := fixture.s.transferIn(fixture.newOrigin, fixture.newCatalog, nil); err != nil {
		t.Fatalf("new catalog transferIn returned error: %v", err)
	}

	fixture.s.zoneMu.RLock()
	memberZone := fixture.s.Z[fixture.member]
	newDynamic := fixture.s.dynamicZones[fixture.member]
	fixture.s.zoneMu.RUnlock()
	if newDynamic == nil || newDynamic.catalog != fixture.newOrigin || newDynamic.memberID != "b" {
		t.Fatalf("expected member zone to migrate to %s with ID b, got %+v", fixture.newOrigin, newDynamic)
	}
	select {
	case <-oldDynamic.shutdown:
	default:
		t.Fatal("expected migration to stop the old catalog transfer loop")
	}
	memberZone.RLock()
	soa := memberZone.SOA
	memberZone.RUnlock()
	if soa != nil {
		t.Fatal("expected different member ID to reset old zone data before the target transfer")
	}

	fixture.releaseTarget()
	waitForAddress(t, fixture.s, fixture.member, "192.0.2.2")
}

func TestTransferInCatalogRejectsInvalidCatalog(t *testing.T) {
	const origin = "catalog.example."
	rrs := catalogZoneRecords(t, false)

	server := dnstest.NewServer(newTestTransferZones(map[string][]dns.RR{origin: rrs}).handler())
	defer server.Close()

	z := file.NewZone(origin, "stdin")
	z.TransferFrom = []string{server.Addr}
	s := newTestSecondary(origin, z, true)

	err := s.transferIn(origin, z, nil)
	if err == nil {
		t.Fatal("expected invalid catalog transfer to fail")
	}
	if !strings.Contains(err.Error(), "version") {
		t.Fatalf("expected version error, got %v", err)
	}
	if s.catalogs[origin] != nil {
		t.Fatal("expected invalid catalog not to be stored")
	}
	if z.SOA != nil {
		t.Fatal("expected invalid catalog transfer not to replace live zone data")
	}
}

func TestTransferInSkipsCatalogParseForRegularZone(t *testing.T) {
	const origin = "catalog.example."
	rrs := catalogZoneRecords(t, false)

	server := dnstest.NewServer(newTestTransferZones(map[string][]dns.RR{origin: rrs}).handler())
	defer server.Close()

	z := file.NewZone(origin, "stdin")
	z.TransferFrom = []string{server.Addr}
	s := newTestSecondary(origin, z, false)

	if err := s.transferIn(origin, z, nil); err != nil {
		t.Fatalf("transferIn returned error: %v", err)
	}
	if s.catalogs[origin] != nil {
		t.Fatal("expected regular secondary transfer not to store catalog")
	}
	if z.SOA == nil {
		t.Fatal("expected regular secondary transfer to replace live zone data")
	}
}

type testTransferZones struct {
	mu        sync.RWMutex
	records   map[string][]dns.RR
	axfrGates map[string]<-chan struct{}
}

func newTestTransferZones(records map[string][]dns.RR) *testTransferZones {
	return &testTransferZones{records: records}
}

func (z *testTransferZones) set(zone string, rrs []dns.RR) {
	z.mu.Lock()
	z.records[zone] = rrs
	z.mu.Unlock()
}

func (z *testTransferZones) blockAXFR(zone string, gate <-chan struct{}) {
	z.mu.Lock()
	if z.axfrGates == nil {
		z.axfrGates = make(map[string]<-chan struct{})
	}
	z.axfrGates[zone] = gate
	z.mu.Unlock()
}

func (z *testTransferZones) handler() dns.HandlerFunc {
	return func(w dns.ResponseWriter, req *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(req)
		if len(req.Question) > 0 {
			qname := strings.ToLower(dns.Fqdn(req.Question[0].Name))
			z.mu.RLock()
			rrs := append([]dns.RR(nil), z.records[qname]...)
			gate := z.axfrGates[qname]
			z.mu.RUnlock()

			switch req.Question[0].Qtype {
			case dns.TypeSOA:
				for _, rr := range rrs {
					if rr.Header().Rrtype == dns.TypeSOA {
						m.Answer = []dns.RR{rr}
						break
					}
				}
			case dns.TypeAXFR:
				if gate != nil {
					<-gate
				}
				m.Answer = rrs
			}
		}
		_ = w.WriteMsg(m)
	}
}

type catalogMigrationFixture struct {
	s          *Secondary
	oldOrigin  string
	newOrigin  string
	member     string
	oldCatalog *file.Zone
	newCatalog *file.Zone
	oldZones   *testTransferZones
	newZones   *testTransferZones
	release    func()
}

func newCatalogMigrationFixture(t *testing.T, oldID, newID, coo string, blockTarget bool) *catalogMigrationFixture {
	t.Helper()

	const (
		oldOrigin = "old.catalog.example."
		newOrigin = "new.catalog.example."
		member    = "example.org."
	)

	oldZones := newTestTransferZones(map[string][]dns.RR{
		oldOrigin: catalogZoneRecordsFor(t, oldOrigin, oldID, member, coo, 1),
		member:    memberZoneRecordsWithAddress(t, 1, "192.0.2.1"),
	})
	newZones := newTestTransferZones(map[string][]dns.RR{
		newOrigin: catalogZoneRecordsFor(t, newOrigin, newID, member, "", 1),
		member:    memberZoneRecordsWithAddress(t, 2, "192.0.2.2"),
	})

	var releaseOnce sync.Once
	targetGate := make(chan struct{})
	release := func() { releaseOnce.Do(func() { close(targetGate) }) }
	if blockTarget {
		newZones.blockAXFR(member, targetGate)
	}

	oldServer := dnstest.NewMultipleServer(oldZones.handler())
	t.Cleanup(oldServer.Close)
	newServer := dnstest.NewMultipleServer(newZones.handler())
	t.Cleanup(newServer.Close)
	t.Cleanup(release)

	oldCatalog := file.NewZone(oldOrigin, "stdin")
	oldCatalog.TransferFrom = []string{oldServer.Addr}
	newCatalog := file.NewZone(newOrigin, "stdin")
	newCatalog.TransferFrom = []string{newServer.Addr}
	s := newSecondary(file.Zones{
		Z:     map[string]*file.Zone{oldOrigin: oldCatalog, newOrigin: newCatalog},
		Names: []string{oldOrigin, newOrigin},
	}, fall.F{}, map[string]plugin.Zones{oldOrigin: nil, newOrigin: nil})
	t.Cleanup(s.stopDynamicZones)

	return &catalogMigrationFixture{
		s:          s,
		oldOrigin:  oldOrigin,
		newOrigin:  newOrigin,
		member:     member,
		oldCatalog: oldCatalog,
		newCatalog: newCatalog,
		oldZones:   oldZones,
		newZones:   newZones,
		release:    release,
	}
}

func (f *catalogMigrationFixture) releaseTarget() {
	f.release()
}

func newTestSecondary(origin string, z *file.Zone, catalog bool) *Secondary {
	catalogZones := map[string]plugin.Zones{}
	if catalog {
		catalogZones[origin] = nil
	}
	return newSecondary(file.Zones{Z: map[string]*file.Zone{origin: z}, Names: []string{origin}}, fall.F{}, catalogZones)
}

func waitForAnswer(t *testing.T, s *Secondary, name string, qtype uint16) *dns.Msg {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		req := new(dns.Msg)
		req.SetQuestion(name, qtype)
		rec := dnstest.NewRecorder(&plugintest.ResponseWriter{})
		code, err := s.ServeDNS(context.Background(), rec, req)
		if err == nil && code == dns.RcodeSuccess && rec.Msg != nil && rec.Msg.Rcode == dns.RcodeSuccess && len(rec.Msg.Answer) > 0 {
			return rec.Msg
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for answer to %s", name)
	return nil
}

func waitForAddress(t *testing.T, s *Secondary, zone, address string) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		msg := waitForAnswer(t, s, "www."+zone, dns.TypeA)
		if len(msg.Answer) == 1 {
			if a, ok := msg.Answer[0].(*dns.A); ok && a.A.String() == address {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s to answer with %s", zone, address)
}

func catalogZoneRecords(t *testing.T, includeVersion bool) []dns.RR {
	t.Helper()
	if !includeVersion {
		soa := mustRR(t, "catalog.example. 0 IN SOA invalid. hostmaster.invalid. 1 3600 600 604800 0")
		return []dns.RR{
			soa,
			mustRR(t, "catalog.example. 0 IN NS invalid."),
			mustRR(t, "a.zones.catalog.example. 0 IN PTR example.org."),
			mustRR(t, `group.a.zones.catalog.example. 0 IN TXT "default"`),
			soa,
		}
	}
	return catalogZoneRecordsWithMemberID(t, "a", 1)
}

func catalogZoneRecordsWithMemberID(t *testing.T, id string, serial int) []dns.RR {
	t.Helper()
	return catalogZoneRecordsFor(t, "catalog.example.", id, "example.org.", "", serial)
}

func catalogZoneRecordsFor(t *testing.T, origin, id, member, coo string, serial int) []dns.RR {
	t.Helper()
	return catalogZoneRecordsForMembers(t, origin, []catalogRecordMember{{id: id, zone: member, coo: coo}}, serial)
}

type catalogRecordMember struct {
	id   string
	zone string
	coo  string
}

func catalogZoneRecordsForMembers(t *testing.T, origin string, members []catalogRecordMember, serial int) []dns.RR {
	t.Helper()

	soa := mustRR(t, fmt.Sprintf("%s 0 IN SOA invalid. hostmaster.invalid. %d 3600 600 604800 0", origin, serial))
	rrs := []dns.RR{
		soa,
		mustRR(t, fmt.Sprintf("%s 0 IN NS invalid.", origin)),
		mustRR(t, fmt.Sprintf(`version.%s 0 IN TXT "2"`, origin)),
	}
	for _, member := range members {
		rrs = append(rrs,
			mustRR(t, fmt.Sprintf("%s.zones.%s 0 IN PTR %s", member.id, origin, member.zone)),
			mustRR(t, fmt.Sprintf(`group.%s.zones.%s 0 IN TXT "default"`, member.id, origin)),
		)
		if member.coo != "" {
			rrs = append(rrs, mustRR(t, fmt.Sprintf("coo.%s.zones.%s 0 IN PTR %s", member.id, origin, member.coo)))
		}
	}
	return append(rrs, soa)
}

func catalogZoneRecordsWithoutMembers(t *testing.T) []dns.RR {
	t.Helper()

	soa := mustRR(t, "catalog.example. 0 IN SOA invalid. hostmaster.invalid. 2 3600 600 604800 0")
	return []dns.RR{
		soa,
		mustRR(t, "catalog.example. 0 IN NS invalid."),
		mustRR(t, `version.catalog.example. 0 IN TXT "2"`),
		soa,
	}
}

func memberZoneRecords(t *testing.T) []dns.RR {
	t.Helper()
	return memberZoneRecordsWithAddress(t, 1, "192.0.2.1")
}

func memberZoneRecordsWithAddress(t *testing.T, serial int, address string) []dns.RR {
	t.Helper()
	return memberZoneRecordsFor(t, "example.org.", serial, address)
}

func memberZoneRecordsFor(t *testing.T, origin string, serial int, address string) []dns.RR {
	t.Helper()

	soa := mustRR(t, fmt.Sprintf("%s 0 IN SOA ns.%s hostmaster.%s %d 3600 600 604800 0", origin, origin, origin, serial))
	return []dns.RR{
		soa,
		mustRR(t, fmt.Sprintf("%s 0 IN NS ns.%s", origin, origin)),
		mustRR(t, fmt.Sprintf("www.%s 0 IN A %s", origin, address)),
		soa,
	}
}

func mustRR(t *testing.T, s string) dns.RR {
	t.Helper()

	rr, err := dns.NewRR(s)
	if err != nil {
		t.Fatalf("failed to parse RR %q: %v", s, err)
	}
	return rr
}
