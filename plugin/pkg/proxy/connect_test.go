package proxy

import (
	"testing"
	"time"
)

const (
	testMsgExpectedError      = "expected error"
	testMsgUnexpectedNilError = "unexpected nil error"
	testMsgWrongError         = "wrong error message"
)

// TestDial_TransportStopped_InitialCheck tests that Dial returns ErrTransportStopped
// if the transport is stopped before Dial is called.
func TestDial_TransportStopped_InitialCheck(t *testing.T) {
	tr := newTransport("test_initial_stop", "127.0.0.1:0")
	tr.Start()

	tr.Stop()
	time.Sleep(50 * time.Millisecond) // Ensure connManager processes stop and exits

	_, _, err := tr.Dial("udp")
	if err == nil {
		t.Fatalf("%s: %s", testMsgExpectedError, testMsgUnexpectedNilError)
	}
	if err.Error() != ErrTransportStopped {
		t.Errorf("%s: got '%v', want '%s'", testMsgWrongError, err, ErrTransportStopped)
	}
}

// TestDial_MultipleCallsAfterStop tests that multiple Dial calls after Stop
// consistently return ErrTransportStopped.
func TestDial_MultipleCallsAfterStop(t *testing.T) {
	tr := newTransport("test_multiple_after_stop", "127.0.0.1:0")
	tr.Start()

	tr.Stop()
	time.Sleep(50 * time.Millisecond)

	for i := range 3 {
		_, _, err := tr.Dial("udp")
		if err == nil {
			t.Errorf("Attempt %d: %s: %s", i+1, testMsgExpectedError, testMsgUnexpectedNilError)
			continue
		}
		if err.Error() != ErrTransportStopped {
			t.Errorf("Attempt %d: %s: got '%v', want '%s'", i+1, testMsgWrongError, err, ErrTransportStopped)
		}
	}
}
