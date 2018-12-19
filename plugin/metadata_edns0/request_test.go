package metadata_edns0

import (
	"encoding/hex"
	"strconv"
	"testing"
)

func TestNewEdns0Opt(t *testing.T) {
	o, err := parseEDNS0Map("0xfffe", "edns", "hex", "16", "0", "8")
	if err != nil {
		t.Error(err)
	} else {
		if o.code != 0xfffe {
			t.Errorf("Expected 0x%x EDNS0 code but got 0x%x", 0xfffe, o.code)
		}

		if o.name != "edns" ||
			o.dataType != typeEDNS0Hex ||
			o.size != 16 ||
			o.start != 0 ||
			o.end != 8 {
			t.Errorf("Unexpected EDNS0 option: %+v", o)
		}
	}

	tests := []struct {
		c string
		n string
		t string
		s string
		b string
		e string
	}{
		{c: "0xGGGG", n: "edns", t: "hex", s: "16", b: "0", e: "8"},
		{c: "0xfffe", n: "edns", t: "xxx", s: "16", b: "0", e: "8"},
		{c: "0xfffe", n: "edns", t: "hex", s: "0x10", b: "0", e: "8"},
		{c: "0xfffe", n: "edns", t: "hex", s: "16", b: "0x0", e: "8"},
		{c: "0xfffe", n: "edns", t: "hex", s: "16", b: "0", e: "0x8"},
		{c: "0xfffe", n: "edns", t: "hex", s: "16", b: "16", e: "8"},
		{c: "0xfffe", n: "edns", t: "hex", s: "16", b: "17", e: "1"},
		{c: "0xfffe", n: "edns", t: "hex", s: "16", b: "0", e: "17"},
	}

	for i, test := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			o, err := parseEDNS0Map(test.c, test.n, test.t, test.s, test.b, test.e)
			if err == nil {
				t.Errorf("Expected error but got EDNS0 0x%x %+v", o.code, o)
			}
		})
	}
}

func TestMakeHexString(t *testing.T) {
	tests := []struct {
		o *edns0Map
		b []byte
		s string
	}{
		{
			o: &edns0Map{
				size: 4,
			},
			b: []byte{0, 1, 2, 3},
			s: "00010203",
		},
		{
			o: &edns0Map{
				size:  4,
				start: 1,
				end:   3,
			},
			b: []byte{0, 1, 2, 3},
			s: "0102",
		},
		{
			o: &edns0Map{
				size:  4,
				start: 1,
				end:   3,
			},
			b: []byte{0, 1, 2, 3, 4, 5, 6, 7},
			s: "",
		},
		{
			o: &edns0Map{
				size:  4,
				start: 4,
				end:   3,
			},
			b: []byte{0, 1, 2, 3},
			s: "",
		},
		{
			o: &edns0Map{
				size:  4,
				start: 1,
				end:   5,
			},
			b: []byte{0, 1, 2, 3},
			s: "",
		},
	}

	for i, test := range tests {
		s := test.o.makeHexString(test.b)
		if s != test.s {
			t.Errorf("expected string %q for test %d but got %q", test.s, i, s)
		}
	}
}

func (o *edns0Map) makeHexString(b []byte) string {
	if o.size > 0 && o.size != uint(len(b)) {
		return ""
	}

	start := uint(0)
	if o.start > 0 {
		if o.start >= uint(len(b)) {
			return ""
		}

		start = o.start
	}

	end := uint(len(b))
	if o.end > 0 {
		if o.end > uint(len(b)) {
			return ""
		}

		end = o.end
	}

	return hex.EncodeToString(b[start:end])
}
