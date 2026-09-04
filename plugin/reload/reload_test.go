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

// TestShutdownRequestedBroadcastsClosedSignal ensures a shutdown remains visible to every observer.
func TestShutdownRequestedBroadcastsClosedSignal(t *testing.T) {
	quit := make(chan struct{})
	close(quit)

	if !shutdownRequested(quit) {
		t.Fatal("expected first shutdownRequested call to observe shutdown")
	}

	if !shutdownRequested(quit) {
		t.Fatal("expected second shutdownRequested call to observe shutdown as well")
	}
}

// TestSetupCreatesIndependentReloadStates ensures one instance shutdown does not stop another.
func TestSetupCreatesIndependentReloadStates(t *testing.T) {
	c1 := caddy.NewTestController("dns", `reload 2s 0s`)
	if err := setup(c1); err != nil {
		t.Fatalf("expected first setup to succeed, got %v", err)
	}
	state1, ok := c1.Get(reloadStorageKey{}).(*reload)
	if !ok {
		t.Fatal("expected first controller to own reload state")
	}

	c2 := caddy.NewTestController("dns", `reload 2s 0s`)
	if err := setup(c2); err != nil {
		t.Fatalf("expected second setup to succeed, got %v", err)
	}
	state2, ok := c2.Get(reloadStorageKey{}).(*reload)
	if !ok {
		t.Fatal("expected second controller to own reload state")
	}
	if state1 == state2 {
		t.Fatal("expected each controller to own independent reload state")
	}

	if err := state1.shutdown(); err != nil {
		t.Fatalf("expected first state shutdown to succeed, got %v", err)
	}
	if shutdownRequested(state2.quit) {
		t.Fatal("expected shutting down the first state not to stop the second")
	}
}

// TestReloadStateShutdownIsIdempotent ensures repeated shutdown callbacks do not panic.
func TestReloadStateShutdownIsIdempotent(t *testing.T) {
	state := newReload()

	if err := state.shutdown(); err != nil {
		t.Fatalf("expected first shutdown to succeed, got %v", err)
	}
	if err := state.shutdown(); err != nil {
		t.Fatalf("expected second shutdown to succeed, got %v", err)
	}

	if !shutdownRequested(state.quit) {
		t.Fatal("expected shutdown after state shutdown")
	}
}
