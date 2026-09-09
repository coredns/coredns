package dnsserver

import (
	"slices"
	"strings"
	"testing"

	"github.com/coredns/caddy"
)

func TestSetDirectives(t *testing.T) {
	original := Directives
	t.Cleanup(func() { Directives = original })

	for _, tc := range []struct {
		name       string
		directives []string
		wantError  string
	}{
		{name: "ordered", directives: []string{"bind", "test_host", "forward"}},
		{name: "replacement", directives: []string{"whoami", "bind"}},
		{name: "nil", directives: nil},
		{name: "empty", directives: []string{}},
		{name: "empty name", directives: []string{"bind", ""}, wantError: "empty directive name"},
		{name: "duplicate", directives: []string{"bind", "forward", "bind"}, wantError: `duplicate directive "bind"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			Directives = []string{"test_previous"}
			previous := Directives
			input := slices.Clone(tc.directives)
			err := SetDirectives(input)
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("SetDirectives(%v) = %v, want %q", input, err, tc.wantError)
				}
				if !slices.Equal(Directives, previous) || &Directives[0] != &previous[0] {
					t.Fatalf("invalid list changed Directives: got %v, want %v", Directives, previous)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if got := caddy.ValidDirectives("dns"); !slices.Equal(got, tc.directives) {
					t.Fatalf("registered server directives = %v, want %v", got, tc.directives)
				}
				if len(input) > 0 {
					input[0] = "test_mutated"
					if !slices.Equal(Directives, tc.directives) {
						t.Fatalf("caller mutation changed Directives: %v", Directives)
					}
					input[0] = tc.directives[0]
				}
			}
			if !slices.Equal(input, tc.directives) {
				t.Fatalf("SetDirectives changed the input: got %v, want %v", input, tc.directives)
			}
		})
	}
}

func TestSetDirectivesCopiesCurrentList(t *testing.T) {
	original := Directives
	t.Cleanup(func() { Directives = original })
	Directives = []string{"bind", "test_host", "forward"}
	previous := Directives
	if err := SetDirectives(Directives[1:]); err != nil {
		t.Fatal(err)
	}
	previous[1] = "test_mutated"
	if want := []string{"test_host", "forward"}; !slices.Equal(Directives, want) {
		t.Fatalf("old list mutation changed Directives: got %v, want %v", Directives, want)
	}
}
