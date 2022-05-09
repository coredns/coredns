package strategy

import "time"

const (
	// garbageCollectionDefaultTTLSeconds defines the period after which the resource is removed
	garbageCollectionDefaultTTLSeconds = 600
	// garbageCollectionPeriodSeconds defines the period when garbage collection is triggered
	garbageCollectionPeriodSeconds = 10
)

// garbageCollector clear the state of dead records
type garbageCollector struct {
	state mstate
	ttl   time.Duration
}

func newGarbageCollector(state mstate, ttlSeconds int) *garbageCollector {
	return &garbageCollector{
		state: state,
		ttl:   time.Duration(ttlSeconds),
	}
}

func (gc *garbageCollector) collect() {
	for k, qm := range gc.state {
		for q, a := range qm {
			for _, s := range a {
				// remove death states for death questions
				if s.timestamp.Before(time.Now().Add(-gc.ttl * time.Second)) {
					delete(qm, q)
				}
			}
		}
		// remove key, if contains 0 items
		if len(qm) == 0 {
			delete(gc.state, k)
		}
	}
}
