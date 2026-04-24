//go:build coredns_all || coredns_ready

package ready

import (
	"testing"

	"github.com/coredns/caddy"
)

func TestSetupReady(t *testing.T) {
	tests := []struct {
		input string

		expectedAddr        string
		expectedMonitorType monitorType

		shouldErr bool
	}{
		{
			input:               `ready`,
			expectedAddr:        ":8181",
			expectedMonitorType: monitorTypeUntilReady,
			shouldErr:           false,
		},
		{
			input:               `ready localhost:1234`,
			expectedAddr:        "localhost:1234",
			expectedMonitorType: monitorTypeUntilReady,
			shouldErr:           false,
		},
		{
			input: `
ready { 
	monitor until-ready
}`,
			expectedAddr:        ":8181",
			expectedMonitorType: monitorTypeUntilReady,
			shouldErr:           false,
		},
		{
			input: `
ready { 
	monitor continuously 
}`,
			expectedAddr:        ":8181",
			expectedMonitorType: monitorTypeContinuously,
			shouldErr:           false,
		},
		{
			input: `
ready localhost:1234 { 
	monitor continuously 
}`,
			expectedAddr:        "localhost:1234",
			expectedMonitorType: monitorTypeContinuously,
			shouldErr:           false,
		},
		{
			input: `
ready localhost:1234 { 
	monitor 404 
}`,
			shouldErr: true,
		},
		{
			input:     `ready localhost:1234 b`,
			shouldErr: true,
		},
		{
			input:     `ready bla`,
			shouldErr: true,
		},
		{
			input:     `ready bla bla`,
			shouldErr: true,
		},
	}

	for i, test := range tests {
		actualAddress, actualMonitorType, err := parse(caddy.NewTestController("dns", test.input))

		if actualAddress != test.expectedAddr {
			t.Errorf("Test %d: Expected address %s but found %s for input %s", i, test.expectedAddr, actualAddress, test.input)
		}
		if actualMonitorType != test.expectedMonitorType {
			t.Errorf("Test %d: Expected monitor type %s but found %s for input %s", i, test.expectedMonitorType, actualMonitorType, test.input)
		}

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: Expected error but found none for input %s", i, test.input)
		}

		if err != nil {
			if !test.shouldErr {
				t.Errorf("Test %d: Expected no error but found one for input %s. Error was: %v", i, test.input, err)
			}
		}
	}
}
