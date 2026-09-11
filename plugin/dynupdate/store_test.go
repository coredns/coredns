package dynupdate

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	coretest "github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
	bolt "go.etcd.io/bbolt"
)

func persistentTestZone(t *testing.T, path string) *DynUpdate {
	t.Helper()
	d := newTestDynUpdate(t)
	d.database = path
	d.seed = path + ".zone"
	var seed strings.Builder
	for _, rr := range d.records {
		fmt.Fprintln(&seed, rr)
	}
	if err := os.WriteFile(d.seed, []byte(seed.String()), 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := d.close(); err != nil {
			t.Errorf("closing dynamic zone: %v", err)
		}
	})
	return d
}

func querySnapshot(t *testing.T, d *DynUpdate, name string, rrType uint16) *dns.Msg {
	t.Helper()
	w := dnstest.NewRecorder(&coretest.ResponseWriter{})
	r := new(dns.Msg)
	r.SetQuestion(name, rrType)
	code, err := d.ServeDNS(context.Background(), w, r)
	if err != nil || code != dns.RcodeSuccess || w.Msg == nil {
		t.Fatalf("query %s: code=%d err=%v response=%v", name, code, err, w.Msg)
	}
	return w.Msg
}

func TestStoreSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "updates.db")
	d := persistentTestZone(t, path)
	add := mustRR(t, `new.example.org. 120 IN TXT "persistent"`)
	if code, err := d.applyUpdate(testKey, nil, []dns.RR{add}); code != dns.RcodeSuccess || err != nil {
		t.Fatalf("update: %d %v", code, err)
	}
	if err := d.close(); err != nil {
		t.Fatal(err)
	}
	restarted := persistentTestZone(t, path)
	// The database is authoritative once initialized, even if the seed goes away.
	if err := os.Remove(restarted.seed); err != nil {
		t.Fatal(err)
	}
	answer := querySnapshot(t, restarted, add.Header().Name, dns.TypeTXT)
	if len(answer.Answer) != 1 || answer.Answer[0].String() != add.String() {
		t.Fatalf("record lost on restart: %v", answer)
	}
	soa := querySnapshot(t, restarted, testZone, dns.TypeSOA)
	if len(soa.Answer) != 1 || soa.Answer[0].(*dns.SOA).Serial != 11 {
		t.Fatalf("serial lost on restart: %v", soa)
	}
	ch, err := restarted.Transfer(testZone, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for batch := range ch {
		for _, rr := range batch {
			found = found || rr.String() == add.String()
		}
	}
	if !found {
		t.Fatal("AXFR lost persistent record")
	}
}

func TestStoreSurvivesProcessKill(t *testing.T) {
	path := filepath.Join(t.TempDir(), "updates.db")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestStoreCrashHelper$")
	cmd.Env = append(os.Environ(), "COREDNS_DYNUPDATE_CRASH_TEST="+path)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(stdout)
	ready := scanner.Scan() && scanner.Text() == "committed"
	killErr := cmd.Process.Kill()
	waitErr := cmd.Wait()
	if !ready || killErr != nil || waitErr == nil {
		t.Fatalf("helper did not commit then crash: ready=%v kill=%v wait=%v", ready, killErr, waitErr)
	}
	d := persistentTestZone(t, path)
	answer := querySnapshot(t, d, "crash.example.org.", dns.TypeTXT)
	if len(answer.Answer) != 1 || answer.Answer[0].String() != "crash.example.org.\t60\tIN\tTXT\t\"committed\"" {
		t.Fatalf("acknowledged record lost after process kill: %v", answer)
	}
}

func TestStoreCrashHelper(t *testing.T) {
	path := os.Getenv("COREDNS_DYNUPDATE_CRASH_TEST")
	if path == "" {
		return
	}
	d := persistentTestZone(t, path)
	rr := mustRR(t, `crash.example.org. 60 IN TXT "committed"`)
	if code, err := d.applyUpdate(testKey, nil, []dns.RR{rr}); code != dns.RcodeSuccess || err != nil {
		t.Fatalf("update: %d %v", code, err)
	}
	fmt.Println("committed")
	// Do not close the database or run cleanup; the parent kills this process.
	time.Sleep(time.Minute)
}

func TestStoreReloadSharesTransactions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "updates.db")
	old := persistentTestZone(t, path)
	next := persistentTestZone(t, path)
	// Open both before writing, as happens with overlapping Corefile instances.
	querySnapshot(t, old, testZone, dns.TypeSOA)
	querySnapshot(t, next, testZone, dns.TypeSOA)
	const count = 16
	var wg sync.WaitGroup
	for i := range count {
		rr := mustRR(t, fmt.Sprintf("host-%d.example.org. 60 IN A 192.0.2.%d", i, i+1))
		wg.Go(func() {
			d := old
			if i%2 == 0 {
				d = next
			}
			if code, err := d.applyUpdate(testKey, nil, []dns.RR{rr}); code != dns.RcodeSuccess || err != nil {
				t.Errorf("update %d: %d %v", i, code, err)
			}
		})
	}
	wg.Wait()
	for _, d := range []*DynUpdate{old, next} {
		for i := range count {
			answer := querySnapshot(t, d, fmt.Sprintf("host-%d.example.org.", i), dns.TypeA)
			if len(answer.Answer) != 1 {
				t.Fatalf("overlapping instance lost update %d: %v", i, answer)
			}
		}
	}
	if err := old.close(); err != nil {
		t.Fatal(err)
	}
	soa := querySnapshot(t, next, testZone, dns.TypeSOA)
	if len(soa.Answer) != 1 || soa.Answer[0].(*dns.SOA).Serial != 10+count {
		t.Fatalf("surviving instance has wrong serial: %v", soa)
	}
}

func TestStoreConcurrentPrerequisites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "updates.db")
	instances := []*DynUpdate{persistentTestZone(t, path), persistentTestZone(t, path)}
	prereq := emptyRR("claimed.example.org.", dns.TypeANY, dns.ClassNONE)
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for i, d := range instances {
		rr := mustRR(t, fmt.Sprintf("claimed.example.org. 60 IN A 192.0.2.%d", i+1))
		wg.Go(func() {
			code, err := d.applyUpdate(testKey, []dns.RR{prereq}, []dns.RR{rr})
			if err != nil {
				t.Errorf("claim: %v", err)
			}
			codes <- code
		})
	}
	wg.Wait()
	first, second := <-codes, <-codes
	valid := first == dns.RcodeSuccess && second == dns.RcodeYXDomain || second == dns.RcodeSuccess && first == dns.RcodeYXDomain
	if !valid {
		t.Fatalf("non-atomic prerequisites: %d, %d", first, second)
	}
}

func TestStoreCommitFailureDoesNotPublish(t *testing.T) {
	d := persistentTestZone(t, filepath.Join(t.TempDir(), "updates.db"))
	querySnapshot(t, d, testZone, dns.TypeSOA)
	// Removing the bucket makes the real write transaction fail, without a
	// production-only test hook or platform-specific filesystem permissions.
	if err := d.store.db.Update(func(tx *bolt.Tx) error { return tx.DeleteBucket(storeBucket) }); err != nil {
		t.Fatal(err)
	}
	add := mustRR(t, `failed.example.org. 60 IN TXT "must not publish"`)
	code, err := d.applyUpdate(testKey, nil, []dns.RR{add})
	if code != dns.RcodeServerFailure || err == nil {
		t.Fatalf("failed commit acknowledged: %d %v", code, err)
	}
	answer := querySnapshot(t, d, add.Header().Name, dns.TypeTXT)
	if answer.Rcode != dns.RcodeNameError {
		t.Fatalf("failed commit became visible: %v", answer)
	}
	if got := serial(d); got != 10 {
		t.Fatalf("failed commit changed serial to %d", got)
	}
}

func TestUpdateLimitsAreAtomic(t *testing.T) {
	for _, mode := range []string{"memory", "database"} {
		for _, limit := range []string{"records", "bytes", "update records"} {
			t.Run(mode+"/"+limit, func(t *testing.T) {
				d := newTestDynUpdate(t)
				if mode == "database" {
					d = persistentTestZone(t, filepath.Join(t.TempDir(), "updates.db"))
					querySnapshot(t, d, testZone, dns.TypeSOA)
				}
				switch limit {
				case "records":
					d.limits.records = len(d.records)
				case "bytes":
					for _, rr := range d.records {
						d.limits.bytes += dns.Len(rr)
					}
				case "update records":
					d.limits.updateRecords = 1
				}
				updates := []dns.RR{mustRR(t, "new.example.org. 60 IN A 192.0.2.10"), mustRR(t, "other.example.org. 60 IN A 192.0.2.11")}
				if code, _ := d.applyUpdate(testKey, nil, updates); code != dns.RcodeRefused {
					t.Fatalf("oversized update: %d", code)
				}
				if serial(d) != 10 || len(d.records) != 4 {
					t.Fatal("rejected update changed zone")
				}
			})
		}
	}
}

func TestParseDatabaseReleasesLock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "updates.db")
	seed := persistentTestZone(t, path)
	c := caddy.NewTestController("dns", fmt.Sprintf(`dynupdate example.org. {
		file %s
		database updates.db
		max_records 100
		max_bytes 1048576
		max_update_records 16
		allow %s * A TXT
	}`, filepath.Base(seed.seed), testKey))
	dnsserver.GetConfig(c).Root = dir
	d, err := parse(c)
	if err != nil {
		t.Fatal(err)
	}
	if d.store != nil || d.database != path || d.limits != (limits{100, 1048576, 16}) {
		t.Fatalf("unexpected parsed database configuration: %+v", d)
	}
	// A failed later directive must not leave a database lock behind.
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatalf("parse retained database lock: %v", err)
	}
	db.Close()
}

func TestDatabaseLargerThanDNSMessage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "updates.db")
	d := persistentTestZone(t, path)
	var extra strings.Builder
	for i := range 400 {
		fmt.Fprintf(&extra, "large-%d.example.org. 60 IN TXT %q\n", i, strings.Repeat("a", 200))
	}
	seed, err := os.ReadFile(d.seed)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(d.seed, append(seed, extra.String()...), 0600); err != nil {
		t.Fatal(err)
	}
	querySnapshot(t, d, testZone, dns.TypeSOA)
	if err := d.close(); err != nil {
		t.Fatal(err)
	}
	restarted := persistentTestZone(t, path)
	answer := querySnapshot(t, restarted, "large-399.example.org.", dns.TypeTXT)
	if len(answer.Answer) != 1 || answer.Answer[0].(*dns.TXT).Txt[0] != strings.Repeat("a", 200) {
		t.Fatalf("zone larger than a DNS message did not round-trip: %v", answer)
	}
}

func TestParseInvalidLimits(t *testing.T) {
	for _, property := range []string{"max_records", "max_bytes", "max_update_records"} {
		for _, value := range []string{"0", "-1", "invalid", "999999999999999999999999", "1 2", ""} {
			t.Run(property+"/"+value, func(t *testing.T) {
				c := caddy.NewTestController("dns", fmt.Sprintf("dynupdate example.org. {\nfile seed\nallow %s * A\n%s %s\n}", testKey, property, value))
				if _, err := parse(c); err == nil || strings.Contains(err.Error(), "opening zone file") {
					t.Fatalf("invalid limit was not rejected before loading: %v", err)
				}
			})
		}
	}
}

func TestDatabaseCannotReplaceSeed(t *testing.T) {
	d := persistentTestZone(t, filepath.Join(t.TempDir(), "updates.db"))
	before, err := os.ReadFile(d.seed)
	if err != nil {
		t.Fatal(err)
	}
	d.database = d.seed
	if _, err := d.snapshot(); err == nil {
		t.Fatal("database accepted the seed path")
	}
	after, err := os.ReadFile(d.seed)
	if err != nil || string(after) != string(before) {
		t.Fatalf("seed modified: %v", err)
	}
}

func TestStoreSnapshotsDuringUpdates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "updates.db")
	d := persistentTestZone(t, path)
	reader := persistentTestZone(t, path)
	querySnapshot(t, reader, testZone, dns.TypeSOA)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 40 {
				ch, err := reader.Transfer(testZone, 0)
				if err != nil {
					t.Error(err)
					return
				}
				var records []dns.RR
				for batch := range ch {
					records = append(records, batch...)
				}
				if len(records) < 2 {
					t.Error("incomplete transfer")
					return
				}
				first, ok1 := records[0].(*dns.SOA)
				last, ok2 := records[len(records)-1].(*dns.SOA)
				if !ok1 || !ok2 || first.Serial != last.Serial || int(first.Serial)-10 != len(records)-5 {
					t.Errorf("mixed zone generations: %v", records)
					return
				}
			}
		})
	}
	for i := range 20 {
		rr := mustRR(t, fmt.Sprintf("node-%d.example.org. 60 IN A 192.0.2.%d", i, i+1))
		if code, err := d.applyUpdate(testKey, nil, []dns.RR{rr}); code != dns.RcodeSuccess || err != nil {
			t.Errorf("update %d: %d %v", i, code, err)
		}
	}
	wg.Wait()
}

func TestStoreRejectsInvalidDatabase(t *testing.T) {
	for _, tc := range []string{"wrong zone", "unknown format", "truncated records", "invalid zone", "too many records", "too many bytes"} {
		t.Run(tc, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "updates.db")
			d := persistentTestZone(t, path)
			querySnapshot(t, d, testZone, dns.TypeSOA)
			if err := d.close(); err != nil {
				t.Fatal(err)
			}
			db, err := bolt.Open(path, 0600, nil)
			if err != nil {
				t.Fatal(err)
			}
			err = db.Update(func(tx *bolt.Tx) error {
				b := tx.Bucket(storeBucket)
				switch tc {
				case "wrong zone":
					return b.Put(originKey, []byte("other.example."))
				case "unknown format":
					if err := tx.DeleteBucket(storeBucket); err != nil {
						return err
					}
					_, err := tx.CreateBucket([]byte("dynupdate-future"))
					return err
				case "truncated records":
					return b.Put(recordsKey, []byte{0xff, 0x01})
				case "invalid zone":
					return putRecords(b, []dns.RR{mustRR(t, "outside.example. 60 IN A 192.0.2.1")})
				}
				return nil
			})
			db.Close()
			if err != nil {
				t.Fatal(err)
			}
			next := persistentTestZone(t, path)
			if tc == "too many records" {
				next.limits.records = 3
			}
			if tc == "too many bytes" {
				next.limits.bytes = 1
			}
			if _, err := next.snapshot(); err == nil {
				t.Fatal("invalid database was silently replaced with seed")
			}
		})
	}
}

func FuzzDecodeRecords(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0xc0, 0x00, 0x00, 0x01})
	f.Fuzz(func(t *testing.T, data []byte) {
		records, err := decodeRecords(data, limits{records: 64, bytes: 4096})
		if err == nil && (len(records) == 0 || len(records) > 64) {
			t.Fatalf("unbounded decoded records: %d", len(records))
		}
	})
}
