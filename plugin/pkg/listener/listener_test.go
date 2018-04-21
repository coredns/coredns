package listener

import (
	"testing"
)

type booker struct {
	name string
}

func (b booker) Name() string {
	return b.name
}

func TestProviderSimpleBooking(t *testing.T) {
	// make a Provider for listener
	// verify
	// 1- new one is allocated if none
	// 2- a second one with the same key, same port reallocated the initial one
	// 3- a second one with the diff key, same port will fail

	// Dry-run mode

	booker := booker{name: "sample"}
	dist := NewListenerDistributor(nil)
	alloc, err := dist.BookListener("tcp", "127.0.0.1:4010", booker, false)
	if err != nil {
		t.Errorf("cannot book a simple address : %v", err)
	}
	listener, err := alloc.AllocateListener()
	if err != nil {
		t.Errorf("cannot allocate the listener after simple booking : %v", err)
	}
	if listener.Addr().String() != "127.0.0.1:4010" {
		t.Errorf("simple listener allocated on the wrong address : %v, expected %v", listener.Addr().String(), "127.0.0.1:4010")
	}

	// now try to reuse this listener
	nextDist := NewListenerDistributor(dist)
	nextAlloc, err := nextDist.BookListener("tcp", "127.0.0.1:4010", booker, true)
	if err != nil {
		t.Errorf("cannot reuse the address of a listener : %v", err)
	}
	nextListener, err := nextAlloc.AllocateListener()
	if err != nil {
		t.Errorf("cannot allocate the listener of a booking including a reused listener : %v", err)
	}
	if nextListener.Addr().String() != listener.Addr().String() {
		t.Errorf("reallocated listener seems on the wrong address : %v, expected %v", nextListener.Addr().String(), listener.Addr().String())
	}

}
