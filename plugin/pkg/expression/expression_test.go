package expression

import (
	"context"
	"testing"

	"github.com/coredns/coredns/plugin/metadata"
	"github.com/coredns/coredns/request"
)

func TestInCidr(t *testing.T) {
	incidr := DefaultEnv(context.Background(), &request.Request{})["incidr"]

	cases := []struct {
		ip        string
		cidr      string
		expected  bool
		shouldErr bool
	}{
		// positive
		{ip: "1.2.3.4", cidr: "1.2.0.0/16", expected: true, shouldErr: false},
		{ip: "10.2.3.4", cidr: "1.2.0.0/16", expected: false, shouldErr: false},
		{ip: "1:2::3:4", cidr: "1:2::/64", expected: true, shouldErr: false},
		{ip: "A:2::3:4", cidr: "1:2::/64", expected: false, shouldErr: false},
		// negative
		{ip: "1.2.3.4", cidr: "invalid", shouldErr: true},
		{ip: "invalid", cidr: "1.2.0.0/16", shouldErr: true},
	}

	for i, c := range cases {
		r, err := incidr.(func(string, string) (bool, error))(c.ip, c.cidr)
		if err != nil && !c.shouldErr {
			t.Errorf("Test %d: unexpected error %v", i, err)
			continue
		}
		if err == nil && c.shouldErr {
			t.Errorf("Test %d: expected error", i)
			continue
		}
		if c.shouldErr {
			continue
		}
		if r != c.expected {
			t.Errorf("Test %d: expected %v", i, c.expected)
			continue
		}
	}
}

func TestMetadata(t *testing.T) {
	ctx := metadata.ContextWithMetadata(context.Background())
	metadata.SetValueFunc(ctx, "test/metadata", func() string {
		return "success"
	})
	f := DefaultEnv(ctx, &request.Request{})["metadata"]

	cases := []struct {
		label     string
		expected  string
		shouldErr bool
	}{
		{label: "test/metadata", expected: "success"},
		{label: "test/nonexistent", expected: ""},
	}

	for i, c := range cases {
		r := f.(func(string) string)(c.label)
		if r != c.expected {
			t.Errorf("Test %d: expected %v", i, c.expected)
			continue
		}
	}
}
