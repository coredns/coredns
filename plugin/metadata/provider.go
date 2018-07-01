package metadata

import (
	"context"

	"github.com/coredns/coredns/request"
)

// Provider interface needs to be implemented by each plugin willing to provide
// metadata information for other plugins.
// Note: this method should work quickly, because it is called for every request
// from the metadata plugin.
type Provider interface {
	// List of variables which are provided by current Provider. Must not be changed during run-time.
	VarInts() []string
	VarStrings() []string

	// Metadata is expected to return a value with metadata information by the key
	// from 4th argument. Value can be later retrieved from context by any other plugin.
	// If value is not available by some reason returned boolean value should be false.
	String(ctx context.Context, state request.Request, variable string) string
	Int(ctx context.Context, state request.Request, variable string) int
}

// M is metadata information storage.
type M struct {
	s map[string]string
	i map[string]int
}

// New returns a new initialized M.
func New() M {
	return M{s: make(map[string]string), i: make(map[string]int)}
}

// FromContext retrieves the metadata from the context.
func FromContext(ctx context.Context) (M, bool) {
	if metadata := ctx.Value(metadataKey{}); metadata != nil {
		if m, ok := metadata.(M); ok {
			return m, true
		}
	}
	return M{}, false
}

// String returns metadata string value by key.
func (m M) String(key string) (string, bool) {
	s, ok := m.s[key]
	return s, ok
}

// SetString sets the metadata value under key.
func (m M) SetString(key, s string) {
	m.s[key] = s
}

// Value returns metadata value by key.
func (m M) Int(key string) (int, bool) {
	i, ok := m.i[key]
	return i, ok
}

// SetValue sets the metadata value under key.
func (m M) SetInt(key string, i int) {
	m.i[key] = i
}

// metadataKey defines the type of key that is used to save metadata into the context.
type metadataKey struct{}
