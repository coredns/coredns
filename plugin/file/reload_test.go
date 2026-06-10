package file

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/plugin/transfer"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

func TestZoneReload(t *testing.T) {
	fileName, rm, err := test.TempFile(".", reloadZoneTest)
	if err != nil {
		t.Fatalf("Failed to create zone: %s", err)
	}
	defer rm()
	reader, err := os.Open(fileName)
	if err != nil {
		t.Fatalf("Failed to open zone: %s", err)
	}
	z, err := Parse(reader, "miek.nl", fileName, 0)
	if err != nil {
		t.Fatalf("Failed to parse zone: %s", err)
	}

	z.ReloadInterval = 10 * time.Millisecond
	z.Reload(&transfer.Transfer{})
	time.Sleep(20 * time.Millisecond)

	ctx := context.TODO()
	r := new(dns.Msg)
	r.SetQuestion("miek.nl", dns.TypeSOA)
	state := request.Request{W: &test.ResponseWriter{}, Req: r}
	if _, _, _, res := z.Lookup(ctx, state, "miek.nl."); res != Success {
		t.Fatalf("Failed to lookup, got %d", res)
	}

	r = new(dns.Msg)
	r.SetQuestion("miek.nl", dns.TypeNS)
	state = request.Request{W: &test.ResponseWriter{}, Req: r}
	if _, _, _, res := z.Lookup(ctx, state, "miek.nl."); res != Success {
		t.Fatalf("Failed to lookup, got %d", res)
	}

	rrs, err := z.ApexIfDefined() // all apex records.
	if err != nil {
		t.Fatal(err)
	}
	if len(rrs) != 5 {
		t.Fatalf("Expected 5 RRs, got %d", len(rrs))
	}
	if err := os.WriteFile(fileName, []byte(reloadZone2Test), 0644); err != nil {
		t.Fatalf("Failed to write new zone data: %s", err)
	}
	// Could still be racy, but we need to wait a bit for the event to be seen
	time.Sleep(30 * time.Millisecond)

	rrs, err = z.ApexIfDefined()
	if err != nil {
		t.Fatal(err)
	}
	if len(rrs) != 3 {
		t.Fatalf("Expected 3 RRs, got %d", len(rrs))
	}
}

func TestZoneReloadSOAChange(t *testing.T) {
	_, err := Parse(strings.NewReader(reloadZoneTest), "miek.nl.", "stdin", 1460175181)
	if err == nil {
		t.Fatalf("Zone should not have been re-parsed")
	}
}

func TestZoneReloadByMtime(t *testing.T) {
	// Test 1: Basic mtime trigger - file modification should trigger reload
	t.Run("BasicMtimeTrigger", func(t *testing.T) {
		fileName, rm, err := test.TempFile(".", reloadZoneTest)
		if err != nil {
			t.Fatalf("Failed to create zone: %s", err)
		}
		defer rm()

		reader, err := os.Open(fileName)
		if err != nil {
			t.Fatalf("Failed to open zone: %s", err)
		}
		z, err := Parse(reader, "miek.nl", fileName, 0)
		if err != nil {
			t.Fatalf("Failed to parse zone: %s", err)
		}
		reader.Close()

		// Enable mtime-based reload
		z.ReloadInterval = 10 * time.Millisecond
		z.ReloadByMtime = true
		z.Reload(&transfer.Transfer{})

		// Wait for initial load to complete
		time.Sleep(20 * time.Millisecond)

		// Verify initial content (5 records)
		rrs, err := z.ApexIfDefined()
		if err != nil {
			t.Fatal(err)
		}
		if len(rrs) != 5 {
			t.Fatalf("Expected 5 initial RRs, got %d", len(rrs))
		}

		// Modify the zone file (this changes mtime)
		if err := os.WriteFile(fileName, []byte(reloadZone2Test), 0644); err != nil {
			t.Fatalf("Failed to write new zone data: %s", err)
		}

		// Wait for reload to trigger
		time.Sleep(30 * time.Millisecond)

		// Verify reload occurred (3 records now)
		rrs, err = z.ApexIfDefined()
		if err != nil {
			t.Fatal(err)
		}
		if len(rrs) != 3 {
			t.Fatalf("Expected 3 RRs after reload, got %d", len(rrs))
		}
	})

	// Test 2: No reload when mtime unchanged
	t.Run("NoReloadWhenMtimeUnchanged", func(t *testing.T) {
		fileName, rm, err := test.TempFile(".", reloadZoneTest)
		if err != nil {
			t.Fatalf("Failed to create zone: %s", err)
		}
		defer rm()

		reader, err := os.Open(fileName)
		if err != nil {
			t.Fatalf("Failed to open zone: %s", err)
		}
		z, err := Parse(reader, "miek.nl", fileName, 0)
		if err != nil {
			t.Fatalf("Failed to parse zone: %s", err)
		}
		reader.Close()

		// Enable mtime-based reload
		z.ReloadInterval = 10 * time.Millisecond
		z.ReloadByMtime = true
		z.Reload(&transfer.Transfer{})

		// Wait for initial load
		time.Sleep(20 * time.Millisecond)

		// Record initial SOA serial
		initialSerial := z.SOASerialIfDefined()
		if initialSerial == -1 {
			t.Fatal("Failed to get initial SOA serial")
		}

		// Record initial record count
		rrs, err := z.ApexIfDefined()
		if err != nil {
			t.Fatal(err)
		}
		initialCount := len(rrs)

		// Wait for multiple reload intervals WITHOUT modifying the file
		time.Sleep(50 * time.Millisecond)

		// Verify no reload occurred
		currentSerial := z.SOASerialIfDefined()
		if currentSerial != initialSerial {
			t.Fatalf("SOA serial changed unexpectedly: %d -> %d", initialSerial, currentSerial)
		}

		rrs, err = z.ApexIfDefined()
		if err != nil {
			t.Fatal(err)
		}
		if len(rrs) != initialCount {
			t.Fatalf("Record count changed unexpectedly: %d -> %d", initialCount, len(rrs))
		}
	})

	// Test 3: Content verification after reload
	t.Run("ContentVerificationAfterReload", func(t *testing.T) {
		fileName, rm, err := test.TempFile(".", reloadZoneTest)
		if err != nil {
			t.Fatalf("Failed to create zone: %s", err)
		}
		defer rm()

		reader, err := os.Open(fileName)
		if err != nil {
			t.Fatalf("Failed to open zone: %s", err)
		}
		z, err := Parse(reader, "miek.nl", fileName, 0)
		if err != nil {
			t.Fatalf("Failed to parse zone: %s", err)
		}
		reader.Close()

		// Enable mtime-based reload
		z.ReloadInterval = 10 * time.Millisecond
		z.ReloadByMtime = true
		z.Reload(&transfer.Transfer{})

		ctx := context.TODO()

		// Query initial content
		r := new(dns.Msg)
		r.SetQuestion("miek.nl", dns.TypeNS)
		state := request.Request{W: &test.ResponseWriter{}, Req: r}

		records, _, _, res := z.Lookup(ctx, state, "miek.nl.")
		if res != Success {
			t.Fatalf("Failed to lookup initial NS records, got %d", res)
		}

		// Initial zone has 4 NS records
		if len(records) != 4 {
			t.Fatalf("Expected 4 initial NS records, got %d", len(records))
		}

		// Modify to new zone content (only 2 NS records)
		if err := os.WriteFile(fileName, []byte(reloadZone2Test), 0644); err != nil {
			t.Fatalf("Failed to write new zone data: %s", err)
		}

		// Wait for reload
		time.Sleep(30 * time.Millisecond)

		// Query new content
		records, _, _, res = z.Lookup(ctx, state, "miek.nl.")
		if res != Success {
			t.Fatalf("Failed to lookup reloaded NS records, got %d", res)
		}

		// Reloaded zone has 2 NS records
		if len(records) != 2 {
			t.Fatalf("Expected 2 reloaded NS records, got %d", len(records))
		}

		// Verify the actual NS record names match the new zone
		nsNames := make([]string, len(records))
		for i, rr := range records {
			nsNames[i] = rr.(*dns.NS).Ns
		}

		expectedNS := []string{"ext.ns.whyscream.net.", "omval.tednet.nl."}
		for i, expected := range expectedNS {
			if nsNames[i] != expected {
				t.Errorf("Expected NS record %d to be %s, got %s", i, expected, nsNames[i])
			}
		}
	})

	// Test 4: File deleted/missing during reload
	t.Run("FileMissingDuringReload", func(t *testing.T) {
		fileName, rm, err := test.TempFile(".", reloadZoneTest)
		if err != nil {
			t.Fatalf("Failed to create zone: %s", err)
		}
		defer rm()

		reader, err := os.Open(fileName)
		if err != nil {
			t.Fatalf("Failed to open zone: %s", err)
		}
		z, err := Parse(reader, "miek.nl", fileName, 0)
		if err != nil {
			t.Fatalf("Failed to parse zone: %s", err)
		}
		reader.Close()

		// Enable mtime-based reload
		z.ReloadInterval = 10 * time.Millisecond
		z.ReloadByMtime = true
		z.Reload(&transfer.Transfer{})

		// Wait for initial load
		time.Sleep(20 * time.Millisecond)

		// Verify initial content is loaded
		rrs, err := z.ApexIfDefined()
		if err != nil {
			t.Fatal(err)
		}
		initialCount := len(rrs)

		// Delete the zone file
		if err := os.Remove(fileName); err != nil {
			t.Fatalf("Failed to remove zone file: %s", err)
		}

		// Wait for reload interval (reload should fail gracefully)
		time.Sleep(30 * time.Millisecond)

		// Verify zone still serves old content (didn't crash)
		rrs, err = z.ApexIfDefined()
		if err != nil {
			t.Fatal(err)
		}
		if len(rrs) != initialCount {
			t.Fatalf("Zone content changed unexpectedly after file deletion: %d -> %d", initialCount, len(rrs))
		}

		// Verify DNS queries still work
		ctx := context.TODO()
		r := new(dns.Msg)
		r.SetQuestion("miek.nl", dns.TypeSOA)
		state := request.Request{W: &test.ResponseWriter{}, Req: r}

		_, _, _, res := z.Lookup(ctx, state, "miek.nl.")
		if res != Success {
			t.Fatalf("Zone should still serve queries after file deletion, got result %d", res)
		}
	})
}

const reloadZoneTest = `miek.nl.		1627	IN	SOA	linode.atoom.net. miek.miek.nl. 1460175181 14400 3600 604800 14400
miek.nl.		1627	IN	NS	ext.ns.whyscream.net.
miek.nl.		1627	IN	NS	omval.tednet.nl.
miek.nl.		1627	IN	NS	linode.atoom.net.
miek.nl.		1627	IN	NS	ns-ext.nlnetlabs.nl.
`

const reloadZone2Test = `miek.nl.		1627	IN	SOA	linode.atoom.net. miek.miek.nl. 1460175182 14400 3600 604800 14400
miek.nl.		1627	IN	NS	ext.ns.whyscream.net.
miek.nl.		1627	IN	NS	omval.tednet.nl.
`
