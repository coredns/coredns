package loop

import (
	"testing"

	"github.com/coredns/caddy"
)

func TestSetup(t *testing.T) {
	c := caddy.NewTestController("dns", `loop`)
	if err := setup(c); err != nil {
		t.Fatalf("Expected no errors, but got: %v", err)
	}

	c = caddy.NewTestController("dns", `loop argument`)
	if err := setup(c); err == nil {
		t.Fatal("Expected errors, but got none")
	}
}

func TestParseServerBlockKeys(t *testing.T) {
	tests := []struct {
		name   string
		key    string
		want   string
		wantOk bool
	}{
		{name: "valid domain", key: "example.org", want: "example.org.", wantOk: true},
		{name: "invalid scheme", key: "unix://", want: ".", wantOk: true},
		{name: "empty", key: "", want: ".", wantOk: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := caddy.NewTestController("dns", `loop`)
			if tt.key != "" {
				c.ServerBlockKeys = []string{tt.key}
			}
			l, err := parse(c)
			if (err == nil) != tt.wantOk {
				t.Fatalf("parse err=%v, wantOk=%v", err, tt.wantOk)
			}
			if l.zone != tt.want {
				t.Fatalf("zone=%q, want %q", l.zone, tt.want)
			}
		})
	}
}
