// Package up is used to run a function for some duration. If a new function is added while a previous run is
// still ongoing, nothing new will be executed.
package up

import (
	"sync/atomic"
	"time"
)

// Probe is used to run a single Func until it returns true (indicating a target is healthy). If an Func
// is already in progress no new one will be added, i.e. there is always a maximum of 1 checks in flight.
type Probe struct {
	inprogress int32
	interval   time.Duration
}

// Func is used to determine if a target is alive. If so this function must return nil.
type Func func() error

// New returns a pointer to an intialized Probe.
func New() *Probe { return &Probe{} }

// Do will probe target, if a probe is already in progress this is a noop.
func (p *Probe) Do(f Func) {
	if atomic.CompareAndSwapInt32(&p.inprogress, idle, active) {
		// Passed the lock. Now run f for as long it returns false. If a true is returned
		// we return from the goroutine and we can accept another Func to run.
		go func() {
			for {
				if err := f(); err == nil {
					break
				}
				time.Sleep(p.interval)
				if atomic.LoadInt32(&p.inprogress) == stop {
					return
				}
			}
			atomic.CompareAndSwapInt32(&p.inprogress, active, idle)
		}()
	}
}

// Stop stops the probing.
func (p *Probe) Stop() { atomic.StoreInt32(&p.inprogress, stop) }

// Start sets probing interval, after which probes can be initiated with Do.
func (p *Probe) Start(interval time.Duration) { p.interval = interval }

const (
	idle = iota
	active
	stop
)
