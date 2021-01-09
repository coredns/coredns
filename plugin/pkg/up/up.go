// Package up is used to run a function for some duration. If a new function is added while a previous run is
// still ongoing, nothing new will be executed.
package up

import (
	"sync"
	"time"
)

// Probe is used to run a single Func until it returns true (indicating a target is healthy). If an Func
// is already in progress no new one will be added, i.e. there is always a maximum of 1 checks in flight.
//
// There is a tradeoff to be made in figuring out quickly that an upstream is healthy and not doing to much work
// (sending queries) to find that out. Having some kind of exp. backoff here won't help much, because you don't won't
// to backoff too much. You then also need random queries to be perfomed every so often to quickly detect a working
// upstream. In the end we just send a query every 0.5 second to check the upstream. This hopefully strikes a balance
// between getting information about the upstream state quickly and not doing too much work. Note that 0.5s is still an
// eternity in DNS, so we may actually want to shorten it.
type Probe struct {
	sync.Mutex
	inprogress int
	interval   time.Duration
}

// Func is used to determine if a target is alive. If so this function must return nil.
type Func func() error

// New returns a pointer to an initialized Probe.
func New() *Probe { return &Probe{} }

// Do will probe target, if a probe is already in progress this is a noop.
func (p *Probe) Do(f Func) {
	p.Lock()
	if p.inprogress != idle {
		p.Unlock()
		return
	}
	p.inprogress = active
	interval := p.interval
	p.Unlock()
	// Passed the lock. Now run f for as long it returns false. If a true is returned
	// we return from the goroutine and we can accept another Func to run.
	go func() {
		i := 1
		for {
			if err := f(); err == nil {
				break
			}
			time.Sleep(interval)
			p.Lock()
			if p.inprogress == stop {
				p.Unlock()
				return
			}
			p.Unlock()
			i++
		}

		p.Lock()
		p.inprogress = idle
		p.Unlock()
	}()
}

// Stop stops the probing.
func (p *Probe) Stop() {
	p.Lock()
	p.inprogress = stop
	p.Unlock()
}

// Start will initialize the probe manager, after which probes can be initiated with Do.
func (p *Probe) Start(interval time.Duration) {
	p.Lock()
	p.interval = interval
	p.Unlock()
}

const (
	idle = iota
	active
	stop
)
