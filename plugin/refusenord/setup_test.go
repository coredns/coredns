package refusenord

import (
	"testing"

	"github.com/coredns/caddy"
)

func TestSetup(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		wantErr bool
	}{
		{
			"plugin with no additional options",
			`refusenord`,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctr := caddy.NewTestController("dns", tt.config)
			if err := setup(ctr); (err != nil) != tt.wantErr {
				t.Errorf("Error: setup() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
