package sign

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// validSOASig is an SOA RRSIG whose inception is ~1 day before and whose
// expiration is ~21 days after the then used throughout these tests, so a zone
// carrying it does not need resigning.
const validSOASig = "miek.nl.\t1800\tIN\tRRSIG\tSOA 13 2 1800 20190808191936 20190717161936 59725 miek.nl. eU6gI1OkSEbyt\n"

func TestResignLargeZoneWithValidSOASig(t *testing.T) {
	then := time.Date(2019, 7, 18, 22, 50, 0, 0, time.UTC)
	var b strings.Builder
	// A valid SOA RRSIG, followed by more than 100 ordinary records. A validly
	// signed zone this size must not be flagged for resigning just because it
	// has more than 100 records after the SOA RRSIG.
	b.WriteString(validSOASig)
	for i := range 150 {
		fmt.Fprintf(&b, "a%d.miek.nl.\t1800\tIN\tA\t127.0.0.1\n", i)
	}
	if x := resign(strings.NewReader(b.String()), then); x != nil {
		t.Errorf("Expected valid signed large zone to not need resigning, got: %s", x)
	}
}

// fillerRecords returns n ordinary A records, one per line, none of them an SOA
// RRSIG, used to push the SOA RRSIG to a known position in the zone.
func fillerRecords(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "a%d.miek.nl.\t1800\tIN\tA\t127.0.0.1\n", i)
	}
	return b.String()
}

func TestResignSOASigAtRecord100(t *testing.T) {
	then := time.Date(2019, 7, 18, 22, 50, 0, 0, time.UTC)
	// 99 filler records then a valid SOA RRSIG: the signature is the 100th
	// record and must still be accepted.
	zr := strings.NewReader(fillerRecords(99) + validSOASig)
	if x := resign(zr, then); x != nil {
		t.Errorf("Expected SOA RRSIG at record 100 to be accepted, got: %s", x)
	}
}

func TestResignSOASigAtRecord101(t *testing.T) {
	then := time.Date(2019, 7, 18, 22, 50, 0, 0, time.UTC)
	// 100 filler records then a valid SOA RRSIG: the signature is the 101st
	// record, outside the search window, so the zone must be resigned.
	zr := strings.NewReader(fillerRecords(100) + validSOASig)
	if x := resign(zr, then); x == nil {
		t.Errorf("Expected SOA RRSIG at record 101 to require a resign, got valid")
	}
}

func TestResignMultipleSOASigs(t *testing.T) {
	then := time.Date(2019, 7, 18, 22, 50, 0, 0, time.UTC)
	// Two SOA signatures (two signing keys) written together after the SOA. The
	// first is fresh; the second expires ~3 days away, inside the one-week
	// window, so every signature must be considered and a resign requested.
	nearExpiry := "miek.nl.\t1800\tIN\tRRSIG\tSOA 13 2 1800 20190722000000 20190717161936 12345 miek.nl. eU6gI1OkSEbyt\n"
	zr := strings.NewReader(validSOASig + nearExpiry + fillerRecords(10))
	if x := resign(zr, then); x == nil {
		t.Errorf("Expected a near-expiry second SOA RRSIG to require a resign, got valid")
	}
}

func TestResignInception(t *testing.T) {
	then := time.Date(2019, 7, 18, 22, 50, 0, 0, time.UTC)
	// signed yesterday
	zr := strings.NewReader(`miek.nl.	1800	IN	RRSIG	SOA 13 2 1800 20190808191936 20190717161936 59725 miek.nl. eU6gI1OkSEbyt`)
	if x := resign(zr, then); x != nil {
		t.Errorf("Expected RRSIG to be valid for %s, got invalid: %s", then.Format(timeFmt), x)
	}
	// inception starts after this date.
	zr = strings.NewReader(`miek.nl.	1800	IN	RRSIG	SOA 13 2 1800 20190808191936 20190731161936 59725 miek.nl. eU6gI1OkSEbyt`)
	if x := resign(zr, then); x == nil {
		t.Errorf("Expected RRSIG to be invalid for %s, got valid", then.Format(timeFmt))
	}
}

func TestResignExpire(t *testing.T) {
	then := time.Date(2019, 7, 18, 22, 50, 0, 0, time.UTC)
	// expires tomorrow
	zr := strings.NewReader(`miek.nl.	1800	IN	RRSIG	SOA 13 2 1800 20190717191936 20190717161936 59725 miek.nl. eU6gI1OkSEbyt`)
	if x := resign(zr, then); x == nil {
		t.Errorf("Expected RRSIG to be invalid for %s, got valid", then.Format(timeFmt))
	}
	// expire too far away
	zr = strings.NewReader(`miek.nl.	1800	IN	RRSIG	SOA 13 2 1800 20190731191936 20190717161936 59725 miek.nl. eU6gI1OkSEbyt`)
	if x := resign(zr, then); x != nil {
		t.Errorf("Expected RRSIG to be valid for %s, got invalid: %s", then.Format(timeFmt), x)
	}
	// expired yesterday
	zr = strings.NewReader(`miek.nl.	1800	IN	RRSIG	SOA 13 2 1800 20190721191936 20190717161936 59725 miek.nl. eU6gI1OkSEbyt`)
	if x := resign(zr, then); x == nil {
		t.Errorf("Expected RRSIG to be invalid for %s, got valid", then.Format(timeFmt))
	}
}
