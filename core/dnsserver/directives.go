package dnsserver

import (
	"fmt"
	"slices"
)

// SetDirectives replaces [Directives] with a copy of directives, in execution
// order. Names must be nonempty and unique. On error, Directives is unchanged.
// An empty or nil list disables all directives.
//
// This selects directives but does not import or register plugins. A host may
// register its plugins after this call, before starting Caddy. Unregistered
// directives used in a Corefile are rejected by Caddy during startup.
//
// The list is process-wide, not per instance. Call SetDirectives before starting
// any servers, not while servers are running. The caller must serialize it with
// all other access to Directives, including startup and reload. It is not a
// runtime reconfiguration API.
func SetDirectives(directives []string) error {
	seen := make(map[string]struct{}, len(directives))
	for i, name := range directives {
		if name == "" {
			return fmt.Errorf("empty directive name at index %d", i)
		}
		if _, ok := seen[name]; ok {
			return fmt.Errorf("duplicate directive %q", name)
		}
		seen[name] = struct{}{}
	}
	Directives = slices.Clone(directives)
	return nil
}
