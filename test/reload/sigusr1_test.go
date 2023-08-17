package reload_test

import (
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/coredns/caddy"
)

func TestReloadAfterSignal(t *testing.T) {
	updateCorefile(`.:0 {
		erratic
		reload 2s 1s
	}`)

	err := startInstance()
	if err != nil {
		t.Fatalf("Could not start CoreDNS instance: %s", err)
	}
	defer stopInstance()
	// wait for instance to be started
	err = waitForInstance(5 * time.Second)
	if err != nil {
		t.Fatalf("Error waiting for instance: %s", err)
	}

	caddy.TrapSignals()
	defer signal.Reset()
	// caddy.TrapSignals() does not establish signal handlers synchronously; that means it calls
	// signal.Notify() in a go routine (which could/should be changed); to avoid/minimize race ceonditions
	// we insert a safety sleep here
	time.Sleep(1 * time.Second)
	err = syscall.Kill(syscall.Getpid(), syscall.SIGUSR1)
	if err != nil {
		t.Fatalf("Error sending USR1 signal: %s", err)
	}
	// wait for instance after SIGUSR1
	err = waitForInstance(5 * time.Second)
	if err != nil {
		t.Fatalf("Error waiting for instance: %s", err)
	}

	// trigger another reload
	updateCorefile(`.:0 {
		forward . 8.8.8.8
		reload 2s 1s
	}`)
	// wait for instance after reload (this proves that reload plugin works after SIGUSR1 triggered reload)
	err = waitForInstance(5 * time.Second)
	if err != nil {
		t.Fatalf("Error waiting for instance: %s", err)
	}
}
