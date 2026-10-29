package reuseport

import (
	"errors"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/capability"

	"golang.org/x/sys/unix"
)

func threadCapabilities(t *testing.T) [2]unix.CapUserData {
	t.Helper()
	header := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	var caps [2]unix.CapUserData
	if err := unix.Capget(&header, &caps[0]); err != nil {
		t.Fatal(err)
	}
	return caps
}

// This test also runs in the Linux capability CI job as a nonroot process with
// only CAP_NET_BIND_SERVICE permitted, not effective, and privileged ports
// enabled. Ordinary test runs need no privileges and skip this integration case.
func TestListenWithPermittedBindCapability(t *testing.T) {
	const bindCapability = uint32(1 << unix.CAP_NET_BIND_SERVICE)
	before := threadCapabilities(t)
	if before[0].Permitted&bindCapability == 0 || before[0].Effective&bindCapability != 0 {
		if os.Getenv("COREDNS_TEST_BIND_CAPABILITY") == "1" {
			t.Fatalf("expected permitted-only CAP_NET_BIND_SERVICE, got %+v", before)
		}
		t.Skip("requires permitted-only CAP_NET_BIND_SERVICE")
	}
	data, err := os.ReadFile("/proc/sys/net/ipv4/ip_unprivileged_port_start")
	if err != nil {
		t.Fatal(err)
	}
	threshold, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || threshold <= 53 {
		t.Fatalf("test requires port 53 to be privileged, got %q: %v", data, err)
	}

	for _, network := range []string{"tcp4", "udp4", "tcp6", "udp6"} {
		t.Run(network, func(t *testing.T) {
			address := "127.0.0.1:53"
			if strings.HasSuffix(network, "6") {
				address = "[::1]:53"
			}
			var closeListener func() error
			if strings.HasPrefix(network, "tcp") {
				control, err := net.Listen(network, address)
				if control != nil {
					control.Close()
				}
				if !errors.Is(err, syscall.EACCES) {
					t.Fatalf("ordinary listen should be denied, got %v", err)
				}
				listener, err := Listen(network, address)
				if err != nil {
					t.Fatal(err)
				}
				closeListener = listener.Close
			} else {
				control, err := net.ListenPacket(network, address)
				if control != nil {
					control.Close()
				}
				if !errors.Is(err, syscall.EACCES) {
					t.Fatalf("ordinary listen should be denied, got %v", err)
				}
				listener, err := ListenPacket(network, address)
				if err != nil {
					t.Fatal(err)
				}
				closeListener = listener.Close
			}
			if err := closeListener(); err != nil {
				t.Fatal(err)
			}
			if after := threadCapabilities(t); after != before {
				t.Fatalf("caller capabilities changed: before=%+v after=%+v", before, after)
			}
		})
	}

	// Repeated and concurrent listeners exercise Go scheduling and reload-style
	// rebinding. No worker should inherit the retry thread's effective capability.
	var workers sync.WaitGroup
	for range 16 {
		workers.Go(func() {
			for range 5 {
				listener, err := ListenPacket("udp4", "127.0.0.1:53")
				if err != nil {
					t.Error(err)
					return
				}
				listener.Close()
				runtime.Gosched()
				if after := threadCapabilities(t); after != before {
					t.Errorf("worker capabilities changed: %+v", after)
				}
			}
		})
	}
	workers.Wait()

	// The disposable threads must exit, including after an unsuccessful retry.
	var retryTID int
	sentinel := errors.New("retry failed")
	calls := 0
	_, err = capability.WithNetBindService(func() (int, error) {
		calls++
		if calls == 1 {
			return 0, syscall.EACCES
		}
		retryTID = unix.Gettid()
		return 0, sentinel
	})
	if !errors.Is(err, sentinel) || calls != 2 {
		t.Fatalf("retry result: calls=%d err=%v", calls, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, err := os.Stat("/proc/self/task/" + strconv.Itoa(retryTID))
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("privileged retry thread %d did not exit", retryTID)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestListenWithBindCapabilityErrors(t *testing.T) {
	for _, want := range []error{nil, syscall.EADDRINUSE, syscall.EINVAL} {
		calls := 0
		got, err := capability.WithNetBindService(func() (int, error) {
			calls++
			return 42, want
		})
		if got != 42 || !errors.Is(err, want) || calls != 1 {
			t.Fatalf("want %v: got value=%d error=%v calls=%d", want, got, err, calls)
		}
	}
}
