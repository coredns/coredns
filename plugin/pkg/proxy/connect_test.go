package proxy

import (
	"sync"
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

// TestDial_TransportStoppedDuringDialSend tests that Dial returns ErrTransportStoppedDuringDial
// if Stop() is called while Dial is attempting to send on the (blocked) t.dial channel.
// This is achieved by not starting the connManager, so t.dial remains unread.
func TestDial_TransportStoppedDuringDialSend(t *testing.T) {
	tr := newTransport("test_during_dial_send", "127.0.0.1:0")
	// No tr.Start() here. This ensures t.dial channel will block.

	dialErrChan := make(chan error, 1)
	go func() {
		// Dial will pass initial stop check (t.stop is open).
		// Then it will block on `t.dial <- proto` because no connManager is reading.
		_, _, err := tr.Dial("udp")
		dialErrChan <- err
	}()

	// Allow Dial goroutine to reach the blocking send on t.dial
	time.Sleep(50 * time.Millisecond)

	tr.Stop() // Close t.stop. Dial's select should now pick <-t.stop.

	err := <-dialErrChan
	if err == nil {
		t.Fatalf("%s: %s", testMsgExpectedError, testMsgUnexpectedNilError)
	}
	if err.Error() != ErrTransportStoppedDuringDial {
		t.Errorf("%s: got '%v', want '%s'", testMsgWrongError, err, ErrTransportStoppedDuringDial)
	}
}

// TestDial_TransportStoppedDuringRetWait tests that Dial returns ErrTransportStoppedDuringRetWait
// when the transport is stopped while Dial is waiting to receive from t.ret channel.
func TestDial_TransportStoppedDuringRetWait(t *testing.T) {
	tr := newTransport("test_during_ret_wait", "127.0.0.1:0")
	// Replace transport's channels to control interaction precisely
	tr.dial = make(chan string)      // Test-controlled, unbuffered
	tr.ret = make(chan *persistConn) // Test-controlled, unbuffered
	// tr.stop remains the original transport stop channel

	// Start connManager and use our controlled channels.
	tr.Start()

	dialErrChan := make(chan error, 1)
	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()
		// Dial will:
		// 1. Pass initial stop check.
		// 2. Send on our tr.dial.
		// 3. Block on our tr.ret in its 3rd select.
		//    When tr.Stop() is called, this 3rd select should pick <-tr.stop.
		_, _, err := tr.Dial("udp")
		dialErrChan <- err
	}()

	// Simulate connManager reading from our tr.dial.
	// This unblocks the Dial goroutine's send.
	var protoFromDial string
	select {
	case protoFromDial = <-tr.dial:
		t.Logf("Test: Simulated connManager read '%s' from Dial via test-controlled tr.dial", protoFromDial)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Test: Timeout waiting for Dial to send on test-controlled tr.dial")
	}

	// Stop the transport and the tr.stop channel
	tr.Stop()

	wg.Wait() // Wait for Dial goroutine to complete.
	err := <-dialErrChan

	if err == nil {
		t.Fatalf("%s: %s", testMsgExpectedError, testMsgUnexpectedNilError)
	}

	// Expected error is ErrTransportStoppedDuringRetWait
	// However, if connManager (using replaced channels) itself reacts to stop faster
	// and somehow closes the test-controlled tr.ret (not its design), other errors are possible.
	// But with tr.ret being ours and unwritten-to, Dial should pick tr.stop.
	if err.Error() != ErrTransportStoppedDuringRetWait {
		t.Errorf("%s: got '%v', want '%s' (or potentially '%s' if timing is very tight)",
			testMsgWrongError, err, ErrTransportStoppedDuringRetWait, ErrTransportStopped)
	} else {
		t.Logf("SUCCESS: Dial correctly returned '%s'", ErrTransportStoppedDuringRetWait)
	}
}

// TestDial_Returns_ErrTransportStoppedRetClosed tests that Dial
// returns ErrTransportStoppedRetClosed when tr.ret is closed before Dial reads from it.
func TestDial_Returns_ErrTransportStoppedRetClosed(t *testing.T) {
	tr := newTransport("test_returns_ret_closed", "127.0.0.1:0")

	// Replace transport channels with test-controlled ones
	testDialChan := make(chan string, 1)   // Buffered to allow non-blocking send by Dial
	testRetChan := make(chan *persistConn) // This will be closed by the test
	tr.dial = testDialChan
	tr.ret = testRetChan
	// tr.stop remains the original, initially open channel.

	dialErrChan := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		// Dial will:
		// 1. Pass initial stop check (tr.stop is open).
		// 2. Send "udp" on tr.dial (which is testDialChan).
		// 3. Block on <-tr.ret (which is testRetChan) in its 3rd select.
		//    When testRetChan is closed, it will read (nil, false), hitting the target error.
		_, _, err := tr.Dial("udp")
		dialErrChan <- err
	}()

	// Step 1: Simulate connManager reading the dial request from Dial.
	// Read from testDialChan. This unblocks the Dial goroutine's send to testDialChan.
	select {
	case proto := <-testDialChan:
		if proto != "udp" {
			wg.Done()
			t.Fatalf("Test: Dial sent wrong proto on testDialChan: got %s, want udp", proto)
		}
		t.Logf("Test: Simulated connManager received '%s' from Dial via testDialChan.", proto)
	case <-time.After(500 * time.Millisecond):
		// If Dial didn't send, the test is flawed or Dial is stuck before sending.
		wg.Done()
		t.Fatal("Test: Timeout waiting for Dial to send on testDialChan.")
	}

	// Step 2: Simulate connManager stopping and closing its 'ret' channel.
	close(testRetChan)
	t.Logf("Test: Closed testRetChan (simulating connManager closing tr.ret).")

	// Step 3: Wait for the Dial goroutine to complete.
	wg.Wait()
	err := <-dialErrChan

	if err == nil {
		t.Fatalf("%s: %s", testMsgExpectedError, testMsgUnexpectedNilError)
	}

	if err.Error() != ErrTransportStoppedRetClosed {
		t.Errorf("%s: got '%v', want '%s'", testMsgWrongError, err, ErrTransportStoppedRetClosed)
	} else {
		t.Logf("SUCCESS: Dial correctly returned '%s'", ErrTransportStoppedRetClosed)
	}

	// Call tr.Stop() for completeness to close the original tr.stop channel.
	// connManager was not started with original channels, so this mainly affects tr.stop.
	tr.Stop()
}

// TestDial_ConnManagerClosesRetOnStop verifies that connManager closes tr.ret upon stopping.
func TestDial_ConnManagerClosesRetOnStop(t *testing.T) {
	tr := newTransport("test_connmanager_closes_ret", "127.0.0.1:0")
	tr.Start()

	// Initiate a Dial to interact with connManager so tr.ret is used.
	interactionDialErrChan := make(chan error, 1)
	go func() {
		_, _, err := tr.Dial("udp")
		interactionDialErrChan <- err
	}()

	// Allow the Dial goroutine to interact with connManager.
	time.Sleep(100 * time.Millisecond)

	// Now stop the transport. connManager should clean up and close tr.ret.
	tr.Stop()

	// Wait for connManager to fully stop and close its channels.
	// This duration needs to be sufficient for the select loop in connManager to see <-t.stop,
	// call t.cleanup(true), which in turn calls close(t.ret).
	time.Sleep(50 * time.Millisecond)

	// Check if tr.ret is actually closed by trying a non-blocking read.
	select {
	case _, ok := <-tr.ret:
		if !ok {
			t.Logf("SUCCESS: tr.ret channel is closed as expected after transport stop.")
		} else {
			t.Errorf("FAIL: tr.ret channel was not closed after transport stop, or a value was read unexpectedly.")
		}
	default:
		// This case means tr.ret is open but blocking (empty).
		// This would be unexpected if connManager is supposed to close it on stop.
		t.Errorf("FAIL: tr.ret channel is not closed and is blocking (or empty but open).")
	}

	// Drain the error channel from the initial interaction Dial to ensure the goroutine finishes.
	select {
	case err := <-interactionDialErrChan:
		if err != nil {
			t.Logf("Interaction Dial completed with error (possibly expected due to 127.0.0.1:0 or race with Stop): %v", err)
		} else {
			t.Logf("Interaction Dial completed without error.")
		}
	case <-time.After(500 * time.Millisecond): // Timeout for safety if Dial hangs
		t.Logf("Timeout waiting for interaction Dial to complete.")
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
