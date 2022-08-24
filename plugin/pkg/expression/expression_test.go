package expression

import (
	"testing"

	"github.com/coredns/coredns/request"
)

func TestInCidr(t *testing.T) {
	incidr := DefaultEnv(&request.Request{})["incidr"]

	cases := []struct {
		cidr1     string
		cidr2     string
		expected  bool
		shouldErr bool
	}{
		// positive
		{cidr1: "1.2.3.4", cidr2: "1.2.0.0/16", expected: true, shouldErr: false},
		{cidr1: "10.2.3.4", cidr2: "1.2.0.0/16", expected: false, shouldErr: false},
		{cidr1: "1:2::3:4", cidr2: "1:2::/64", expected: true, shouldErr: false},
		{cidr1: "A:2::3:4", cidr2: "1:2::/64", expected: false, shouldErr: false},
		// negative
		{cidr1: "1.2.3.4", cidr2: "invalid", shouldErr: true},
		{cidr1: "invalid", cidr2: "1.2.0.0/16", shouldErr: true},
	}

	for i, c := range cases {
		r, err := incidr.(func(string, string) (bool, error))(c.cidr1, c.cidr2)
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
