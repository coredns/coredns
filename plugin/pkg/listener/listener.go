package listener

import (
	"fmt"
	"log"
	"net"

	"github.com/mholt/caddy"
)

// Booker - generic type providing information on the booker
type Booker interface {
	// Name is used to verify re-usability of a Listener - listener is reusable only for Booker with the same Name
	Name() string
}

// booking structure use at booking, and allocation time of the listener
type booking struct {
	booker    Booker
	address   string
	network   string
	reusable  net.Listener
	allocated net.Listener
}

func (b booking) equal(network string, address string) bool {
	// Consider that port "0" means "a new port" and will never match another port
	// Consider that "localhost" or "" is matching both "127.0.01" or "[::1]"

	if network == b.network {
		h, p, _ := net.SplitHostPort(b.address)
		hostname, port, _ := net.SplitHostPort(address)
		if port == "0" || p == "0" {
			return false
		}
		if port == p {
			return hostname == h
		}
	}
	return false
}

// AllocationToken is an interface that is able to allocated the Listener
type AllocationToken interface {
	AllocateListener() (net.Listener, error)
}

//AllocateListener will act upon configuration of the booking:
// - either reuse an existing listener
// - either spin-up a new one
// - keep track of the new Listener in order to provide for next RELOAD
func (b *booking) AllocateListener() (net.Listener, error) {
	// Cannot allocate several times
	if b.allocated != nil {
		return nil, fmt.Errorf("listener already allocated for %s, at address (%s, %s)", b.booker.Name(), b.allocated.Addr().Network(), b.allocated.Addr().String())
	}

	var ln net.Listener
	var err error
	if b.reusable != nil {
		// we want to reuse the same address - duplicate the FD and open a listener on that FD
		// If this is a reload and s is a GracefulServer,
		// reuse the listener for a graceful restart.
		if fileLn, ok := b.reusable.(caddy.Listener); ok {
			file, err := fileLn.File()
			if err != nil {
				return nil, err
			}
			ln, err = net.FileListener(file)
			if err != nil {
				return nil, err
			}
			err = file.Close()
			if err != nil {
				return nil, err
			}

		}
	}
	if ln == nil {
		ln, err = net.Listen(b.network, b.address)
		if err != nil {
			return nil, err
		}
	}
	b.allocated = ln
	log.Printf("[INFO] %s - listening on network %s - address %s", b.booker.Name(), ln.Addr().Network(), ln.Addr().String())
	return ln, err
}

// Distributor provide ability to distribute and re-distribute the listeners. Ensuring bridge for gracefull continuation after a reload
type Distributor struct {
	// Distribution is done in 2 times:
	// 1- Book the listener - (expected during Setup of plugin) - we expect to be able to reuse existing listener, but raise any error during that phase
	// 2- Allocate the listener - (expected after Start of the Servers) - an error here can happen, but would be unfortunate, leaving the system in unknown status

	// need to fail at Book time if:
	//   - address is already in used by ANOTHER plugin
	//   - address is already booked (reusing or not)
	//   - NOTE: need to take care of 'similar addresses' or address always different (port 0)
	//
	reusable []*booking
	booked   []*booking
}

// NewListenerDistributor : provide an new Distributor with ability to reuse the allocated listener of a former Distributor
func NewListenerDistributor(seedDistributor *Distributor) *Distributor {
	reusable := make([]*booking, 0)
	if seedDistributor != nil {
		for _, v := range seedDistributor.booked {
			if v.allocated != nil {
				reusable = append(reusable, v)
			}
		}
	}
	return &Distributor{reusable: reusable, booked: make([]*booking, 0)}
}

// IsBooked return status of booking for this address. The Booker info is returned
// it is typically used for the booker to be able to share the same new Listener - as re-Book the same address is forbidden.
// (see metrics plugin)
func (l *Distributor) IsBooked(network string, address string) (bool, Booker) {
	for _, b := range l.booked {
		if b.equal(network, address) {
			return true, b.booker
		}
	}
	return false, nil
}

// BookListener ask for booking a new listener. Will return an allocation interface to use when need to really instanciate the listener
func (l *Distributor) BookListener(network string, address string, bookerInfo Booker, allowReuse bool) (AllocationToken, error) {
	_, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("listener booking for %s, (%s, %s) - address provider is invalid : %v", bookerInfo.Name(), network, address, err)
	}
	// check if already booked - raise error
	for _, booking := range l.booked {
		if booking.equal(network, address) {
			return nil, fmt.Errorf("listener booking for %s, (%s, %s) - an overlaping listener is already booked for %s (%s, %s) - sharing is not allowed", bookerInfo.Name(), network, address, booking.booker.Name(), booking.network, booking.address)
		}
	}
	toBook := &booking{network: network, address: address, reusable: nil, booker: bookerInfo}
	// check if reusable Listener
	for r, reuse := range l.reusable {
		if reuse != nil {
			if toBook.equal(reuse.network, reuse.address) {
				// check that we reuse for the same family of Booker (same Name)
				if reuse.booker.Name() != bookerInfo.Name() {
					return nil, fmt.Errorf("listener booking for %s, (%s, %s) - an overlaping listener is already in use for %s (%s, %s)", bookerInfo.Name(), network, address, reuse.booker.Name(), reuse.network, reuse.address)
				}
				if !allowReuse {
					return nil, fmt.Errorf("listener booking for %s, (%s, %s) - this listener is already in use, and re-use is not allowed", bookerInfo.Name(), network, address)
				}
				toBook.reusable = reuse.allocated
				// We prevent to reuse a second time - although the test on booking should also prevent it
				l.reusable[r] = nil
				break
			}
		}
	}
	// book it anyway, whatever reuse status
	l.booked = append(l.booked, toBook)
	return toBook, nil
}

//Bookings return all listeners that match the provide name
func (l *Distributor) Bookings(name string) []net.Listener {
	match := make([]net.Listener, 0)
	for _, b := range l.booked {
		if b.booker.Name() == name {
			match = append(match, b.allocated)
		}
	}
	return match
}
