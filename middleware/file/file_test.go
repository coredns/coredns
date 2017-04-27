package file

import (
	"io/ioutil"
	"log"
	"strings"
	"testing"

	"github.com/miekg/coredns/middleware/test"

	"github.com/mholt/caddy"
)

func BenchmarkParseInsert(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Parse(strings.NewReader(dbMiekENTNL), testzone, "stdin")
	}
}

func TestParseSerial(t *testing.T) {

	log.SetOutput(ioutil.Discard)

	// DNSSEC
	filename, rm, err := test.TempFile("/tmp", dbMiekENTNL)
	if err != nil {
		t.Fatalf("Could not create tmp zone file: %s", err)
	}
	defer rm()

	c := caddy.NewTestController("dns", `file `+filename+` miek.nl {
			ignore_serial
		}`)

	zones, err := fileParse(c)
	if err != nil {
		t.Fatalf("Failed to parse file stanza: %s", err)
	}

	// This zone is signed so the serial should not be updated
	soa := zones.Z["miek.nl."].Apex.SOA // if this blows up the test is no good anyway
	if soa.Serial != 1282630057 {
		t.Fatalf("SOA serial should have been left alone: %d", soa.Serial)
	}

	// non-DNSSEC
	filename, rm, err = test.TempFile("/tmp", dbMiekNL)
	if err != nil {
		t.Fatalf("Could not create tmp zone file: %s", err)
	}
	defer rm()

	c = caddy.NewTestController("dns", `file `+filename+` miek.nl {
			ignore_serial
		}`)

	zones, err = fileParse(c)
	if err != nil {
		t.Fatalf("Failed to parse file stanza: %s", err)
	}

	soa = zones.Z["miek.nl."].Apex.SOA
	if soa.Serial == 1282630057 {
		t.Fatalf("SOA serial should have been updated: %d", soa.Serial)
	}
}
