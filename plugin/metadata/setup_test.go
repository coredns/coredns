package metadata

import (
	"reflect"
	"testing"

	"github.com/mholt/caddy"
)

func TestSetup(t *testing.T) {
	tests := []struct {
		input     string
		zones     []string
		shouldErr bool
	}{
		{"metadata", []string{}, false},
		{"metadata example.com.", []string{"example.com."}, false},
		{"metadata example.com. net.", []string{"example.com.", "net."}, false},

		{`metadata example.com. { 
						some_param 
				}`, []string{}, true},
		{`metadata { 
					group_id edns0 0xffee hex 16 0 16
				}`, []string{}, false},
		{`metadata example.com { 
					client_id edns0 0xffed
					group_id edns0 0xffee hex 16 0 16
				}`, []string{}, false},
		{`metadata { 
					client_id edns0 
				}`, []string{}, true},
		{"metadata\nmetadata", []string{}, true},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		err := setup(c)

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: Setup call expected error but found none for input %s", i, test.input)
		}

		if !test.shouldErr && err != nil {
			t.Errorf("Test %d: Setup call expected no error but found one for input %s. Error was: %v", i, test.input, err)
		}
	}
}

func TestRequestParse(t *testing.T) {
	tests := []struct {
		input       string
		shouldErr   bool
		expectedLen int
	}{
		{`metadata example.com {
			client_id edns0 0xffed
		}`, false, 1},

		{`metadata {
			client_id edns0
		}`, true, 1},
		{`metadata {
			group_id edns0 0xffee hex 16 0 16
			client_id edns0
		}`, true, 2},

		{`metadata {
			client_id edns0 0xffed
			group_id edns0 0xffee hex 16 0 16
		}`, false, 2},

		{`metadata example.com {
			client_id edns0 0xffed
			label edns0 0xffee
		}`, false, 2},

		{`metadata {
			group_id edns0
		}`, true, 1},
		{`metadata {
			example/label edns0 0xffed
		}`, false, 1},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		actualRequest, err := metadataParse(c)
		if test.shouldErr && err == nil {
			t.Errorf("Test %v: Expected error but found nil", i)
			continue
		} else if !test.shouldErr && err != nil {
			t.Errorf("Test %v: Expected no error but found error: %v", i, err)
			continue
		}
		if test.shouldErr && err != nil {
			continue
		}
		x := len(actualRequest.options)
		if x != test.expectedLen {
			t.Errorf("Test %v: Expected map length of %d, got: %d", i, test.expectedLen, x)
		}
	}
}

func TestSetupHealth(t *testing.T) {
	tests := []struct {
		input     string
		zones     []string
		shouldErr bool
	}{
		{"metadata", []string{}, false},
		{"metadata example.com.", []string{"example.com."}, false},
		{"metadata example.com. net.", []string{"example.com.", "net."}, false},

		{`metadata example.com. { 
					some_param 
				}`, []string{}, true},
		{`metadata example.com { 
					group_id edns0 0xffee hex 16 0 16
				}`, []string{"example.com."}, false},
		{"metadata\nmetadata", []string{}, true},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		m, err := metadataParse(c)

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: Expected error but found none for input %s", i, test.input)
		}

		if !test.shouldErr && err != nil {
			t.Errorf("Test %d: Expected no error but found one for input %s. Error was: %v", i, test.input, err)
		}

		if !test.shouldErr && err == nil {
			if !reflect.DeepEqual(test.zones, m.Zones) {
				t.Errorf("Test %d: Expected zones %s. Zones were: %v", i, test.zones, m.Zones)
			}
		}
	}
}
