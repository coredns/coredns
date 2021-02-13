package sign

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coredns/caddy"
)

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

func TestResignModTime(t *testing.T) {
	input := `sign testdata/db.miek.nl miek.nl {
               key file testdata/Kmiek.nl.+013+59725
               directory testdata
       }`
	c := caddy.NewTestController("dns", input)
	sign, err := parse(c)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove("testdata/db.miek.nl.signed")

	if len(sign.signers) != 1 {
		t.Fatalf("Expected 1 signer, got %d", len(sign.signers))
	}
	signer := sign.signers[0]

	why := signer.resign()
	if !strings.Contains(why.Error(), "no such file or directory") {
		t.Fatalf("Expected %q, got: %s", "no such file or directory", why.Error())
	}

	// Slightly harder to properly test this, as we need to pull in the zone writing as well.
	z, err := signer.Sign(time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := signer.write(z); err != nil {
		t.Fatal(err)
	}
	if x := signer.modTime; x.IsZero() {
		t.Errorf("Expected non zero modification time: got: %s", x.Format(timeFmt))
	}

	why = signer.resign()
	if why != nil {
		t.Errorf("Expected not to have to resign the zone, got: %s", why)
	}

	// set mtime on original zone file and see if we pick it up as a cue to resign
	if err := os.Chtimes("testdata/db.miek.nl", time.Now(), time.Now()); err != nil {
		t.Fatal(err)
	}
	why = signer.resign()
	if !strings.Contains(why.Error(), "differs from last seen modification") {
		t.Errorf("Expecting to resign the zone, but got: %s", why.Error())
	}
}
