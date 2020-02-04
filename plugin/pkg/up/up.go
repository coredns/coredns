// Package up is used to run a function for some duration. If a new function is added while a previous run is
// still ongoing, nothing new will be executed.
package up

import (
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
)

// Probe is used to run a single Func until it returns true (indicating a target is healthy). If an Func
// is already in progress no new one will be added, i.e. there is always a maximum of 1 checks in flight.
// When failures start to happen we will back off every second failure up to maximum of 4 intervals.
type Probe struct {
	sync.Mutex
	inprogress int
	expBackoff backoff.ExponentialBackOff
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
	interval := p.expBackoff.NextBackOff()
	// If exponential backoff has reached the maximum elapsed time (15 minutes),
	// reset it and try again
	if interval == -1 {
		p.expBackoff.Reset()
		interval = p.expBackoff.NextBackOff()
	}
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
// Initializes exponential backoff using the given interval duration
func (p *Probe) Start(interval time.Duration) {
	p.Lock()
	eB := &backoff.ExponentialBackOff{
		InitialInterval:     interval,
		RandomizationFactor: backoff.DefaultRandomizationFactor,
		Multiplier:          backoff.DefaultMultiplier,
		MaxInterval:         15 * time.Second,
		MaxElapsedTime:      2 * time.Minute,
		Stop:                backoff.Stop,
		Clock:               backoff.SystemClock,
	}
	p.expBackoff = *eB
	p.expBackoff.Reset()
	p.Unlock()
}

const (
	idle = iota
	active
	stop
)
