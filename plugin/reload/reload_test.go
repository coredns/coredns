package reload

import (
	"testing"

	"github.com/coredns/caddy"
)

// fakeInput implements caddy.Input for testing parse().
type fakeInput struct {
	p string
	b []byte
}

func (f fakeInput) ServerType() string { return "dns" }
func (f fakeInput) Body() []byte       { return f.b }
func (f fakeInput) Path() string       { return f.p }

// TestParseInvalidCorefile ensures parse returns an error for invalid Corefile syntax.
func TestParseInvalidCorefile(t *testing.T) {
	t.Parallel()

	broken := fakeInput{p: "Corefile", b: []byte(". { errors\n")}
	if _, err := parse(broken); err == nil {
		t.Fatalf("expected parse error for invalid Corefile, got nil")
	}
}

// TestShutdownGate ensures the shutdown gate helper recognizes when shutdown is requested.
func TestShutdownGate(t *testing.T) {
	t.Parallel()

	q := make(chan struct{})
	if shutdownRequested(q) {
		t.Fatalf("expected no shutdown before signal")
	}
	close(q)
	if !shutdownRequested(q) {
		t.Fatalf("expected shutdown after signal")
	}
}

// TestHookIgnoresNonStartupEvent ensures hook is a no-op for non-startup events.
func TestHookIgnoresNonStartupEvent(t *testing.T) {
	t.Parallel()

	if err := hook(caddy.EventName("not-startup"), nil); err != nil {
		t.Fatalf("expected no error for non-startup event, got %v", err)
	}
}

// TestShutdownRequestedConsumesSignal ensures that once a shutdown signal is consumed, it still can be observed by other observers as it's a global broadcast.
func TestShutdownRequestedConsumesSignal(t *testing.T) {
	quit := make(chan struct{})
	close(quit)

	if !shutdownRequested(quit) {
		t.Fatal("expected first shutdownRequested call to observe shutdown")
	}

	if !shutdownRequested(quit) {
		t.Fatal("expected second shutdownRequested call to observe shutdown as well")
	}
}
