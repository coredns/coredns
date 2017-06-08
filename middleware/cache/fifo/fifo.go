package fifo

import (
	"sync"
	"time"
)

// Fifo is a first in first out queue that keeps track of the last N time stamps.
type Fifo struct {
	// Record the last X timestamps for incoming request for this name.
	stamp []time.Time
	index int

	m sync.RWMutex
}

// NewFifo returns
func NewFifo(size int) *Fifo {
	return &Fifo{
		stamp: make([]time.Time, size, size),
		index: -1,
	}
}

func (f *Fifo) Size() int {
	f.m.RLock()
	defer f.m.RUnlock()

	return f.index + 1
}

// Push pushes a new timestamp on f, possibly pushing out an older element.
func (f *Fifo) Push(t time.Time) {
	f.m.Lock()
	defer f.m.Unlock()

	if f.index < len(f.stamp)-1 { // still room
		f.index++
		f.stamp[f.index] = t
		return
	}
	// move everything down, and set the stamp in the last element
	f.stamp = f.stamp[1:]
	f.stamp = append(f.stamp, t)
}

// Frequency returns the number of timestamps that fall in the interval (start, end).
func (f *Fifo) Frequency(start, end time.Time) int {
	f.m.RLock()
	defer f.m.RUnlock()

	freq := 0

	for i := f.index; i >= 0; i-- {
		if f.stamp[i].After(start) && f.stamp[i].Before(end) {
			freq++
		} else {
			return freq
		}
	}

	return freq
}
