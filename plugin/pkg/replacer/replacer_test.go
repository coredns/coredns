package replacer

import (
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestNewReplacer(t *testing.T) {
	w := dnstest.NewRecorder(&test.ResponseWriter{})

	r := new(dns.Msg)
	r.SetQuestion("example.org.", dns.TypeHINFO)
	r.MsgHdr.AuthenticatedData = true

	replaceValues := New(r, w, "", "s")

	switch v := replaceValues.(type) {
	case replacer:

		if v.replacements["{type}"] != "HINFO" {
			t.Errorf("Expected type to be HINFO, got %q", v.replacements["{type}"])
		}
		if v.replacements["{name}"] != "example.org." {
			t.Errorf("Expected request name to be example.org., got %q", v.replacements["{name}"])
		}
		if v.replacements["{size}"] != "29" { // size of request
			t.Errorf("Expected size to be 29, got %q", v.replacements["{size}"])
		}

	default:
		t.Fatal("Return Value from New Replacer expected pass type assertion into a replacer type\n")
	}
}

func TestSet(t *testing.T) {
	w := dnstest.NewRecorder(&test.ResponseWriter{})

	r := new(dns.Msg)
	r.SetQuestion("example.org.", dns.TypeHINFO)
	r.MsgHdr.AuthenticatedData = true

	repl := New(r, w, "", "ms")

	repl.Set("name", "coredns.io.")
	repl.Set("type", "A")
	repl.Set("size", "20")

	if repl.Replace("This name is {name}") != "This name is coredns.io." {
		t.Error("Expected name replacement failed")
	}
	if repl.Replace("This type is {type}") != "This type is A" {
		t.Error("Expected type replacement failed")
	}
	if repl.Replace("The request size is {size}") != "The request size is 20" {
		t.Error("Expected size replacement failed")
	}
}

func TestNormalizeTime(t *testing.T) {
	var dur1 = time.Second
	var dur2 = (time.Millisecond * 300)
	var dur3 = (time.Second * 36)
	var dur4 = (time.Minute * 4)
	var dur5 = (time.Microsecond * 56)

	dur1ToMilliseconds := "1000"
	dur1Normalized := NormalizeTime(dur1, "ms")

	if dur1Normalized != dur1ToMilliseconds {
		t.Error("Seconds to Milliseconds failed")
	}

	dur2Normalized := NormalizeTime(dur2, "s")
	dur2ToSeconds := "0.3"

	if dur2Normalized != dur2ToSeconds {
		t.Error("Milliseconds to Seconds failed")
	}

	dur3Normalized := NormalizeTime(dur3, "ns")
	dur3ToNano := "36000000000"

	if dur3Normalized != dur3ToNano {
		t.Error("Seconds to Nanoseconds failed")
	}

	dur4Normalized := NormalizeTime(dur4, "micro")
	dur4ToMicro := "240000000"

	if dur4Normalized != dur4ToMicro {
		t.Error("Minutes to Microseconds failed")
	}

	if dur5.String() != NormalizeTime(dur5, "") {
		t.Error("Default case failed")
	}

}
