package sign

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestResignLargeZoneWithValidSOASig(t *testing.T) {
	then := time.Date(2019, 7, 18, 22, 50, 0, 0, time.UTC)
	var b strings.Builder
	// A valid SOA RRSIG: inception ~1 day ago, expiration ~21 days away, so
	// the zone does not need resigning.
	b.WriteString("miek.nl.\t1800\tIN\tRRSIG\tSOA 13 2 1800 20190808191936 20190717161936 59725 miek.nl. eU6gI1OkSEbyt\n")
	// Followed by more than 100 ordinary records. A validly signed zone this
	// size must not be flagged for resigning just because it has more than 100
	// records after the SOA RRSIG.
	for i := 0; i < 150; i++ {
		fmt.Fprintf(&b, "a%d.miek.nl.\t1800\tIN\tA\t127.0.0.1\n", i)
	}
	if x := resign(strings.NewReader(b.String()), then); x != nil {
		t.Errorf("Expected valid signed large zone to not need resigning, got: %s", x)
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
