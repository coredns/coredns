package view

import (
	"context"
	"errors"
	"testing"

	"github.com/coredns/caddy"
	ptest "github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestSetup(t *testing.T) {
	tests := []struct {
		input     string
		shouldErr bool
		progCount int
	}{
		{"view example {\n expr name() == 'example.com.'\n}", false, 1},
		{"view example {\n expr incidr(client_ip(), '10.0.0.0/24')\n}", false, 1},
		{"view example {\n expr name() == 'example.com.'\n expr name() == 'example2.com.'\n}", false, 2},
		{"view", true, 0},
		{"view example {\n expr invalid expression\n}", true, 0},
		{"view x {\n foo bar\n}\n", true, 0},
		{"view a { }\nview b { }\n", true, 0},
	}

	for i, test := range tests {
		v, err := parse(caddy.NewTestController("dns", test.input))

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: Expected error but found none for input %s", i, test.input)
		}
		if err != nil && !test.shouldErr {
			t.Errorf("Test %d: Expected no error but found one for input %s. Error was: %v", i, test.input, err)
		}
		if test.shouldErr {
			continue
		}
		if test.progCount != len(v.progs) {
			t.Errorf("Test %d: Expected prog length %d, but got %d for %s.", i, test.progCount, len(v.progs), test.input)
		}
	}
}

func TestServeDNS_DelegatesToNext(t *testing.T) {
	v := &View{}
	wantCode := 123
	wantErr := errors.New("boom")
	v.Next = ptest.NextHandler(wantCode, wantErr)

	rr := new(dns.Msg)
	rr.SetQuestion("example.com.", dns.TypeA)
	w := &ptest.ResponseWriter{}
	gotCode, gotErr := v.ServeDNS(context.Background(), w, rr)
	if gotCode != wantCode {
		t.Fatalf("rcode: got %d, want %d", gotCode, wantCode)
	}
	if gotErr != wantErr {
		t.Fatalf("error: got %v, want %v", gotErr, wantErr)
	}
}
