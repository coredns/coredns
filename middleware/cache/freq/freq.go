// Package freq keeps track of last X seen events. The events themselves are not stored
// here. So the Freq type should be added next to the thing it is tracking.
package freq

import "time"

type Freq struct {
	// Last time we saw a query for this element.
	last time.Time
	// Number of this in the last time slice.
	hits int
}

func New(t time.Time) *Freq {
	return &Freq{last: t, hits: 0}
}

// Updates updates the number of hits. Last time seen will be set to now.
// If the last time we've seen this entity is within now - d, we increment hits, otherwise
// we reset hits to 1. It returns the number of hits.
func (f *Freq) Update(d time.Duration, now time.Time) int {
	earliest := now.Add(-1 * d)
	if f.last.Before(earliest) {
		f.last = now
		f.hits = 1
		return f.hits
	}
	f.last = now
	f.hits++
	return f.hits
}

// Hits returns the number of hits that we have seen, according to the updates we have done to f.
func (f *Freq) Hits() int { return f.hits }

// Reset resets f to time t and hits to 0.
func (f *Freq) Reset(t time.Time) {
	f.last = t
	f.hits = 0
}
