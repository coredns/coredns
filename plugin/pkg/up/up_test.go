package up

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestUp(t *testing.T) {
	pr := New()
	wg := sync.WaitGroup{}
	hits := int32(0)

	upfunc := func() error {
		atomic.AddInt32(&hits, 1)
		// Sleep tiny amount so that our other pr.Do() calls hit the lock.
		time.Sleep(3 * time.Millisecond)
		wg.Done()
		return nil
	}

	pr.Start(5 * time.Millisecond)
	defer pr.Stop()

	// These functions AddInt32 to the same hits variable, but we only want to wait when
	// upfunc finishes, as that only calls Done() on the waitgroup.
	upfuncNoWg := func() error { atomic.AddInt32(&hits, 1); return nil }
	wg.Add(1)
	pr.Do(upfunc)
	pr.Do(upfuncNoWg)
	pr.Do(upfuncNoWg)

	wg.Wait()

	h := atomic.LoadInt32(&hits)
	if h != 1 {
		t.Errorf("Expected hits to be %d, got %d", 1, h)
	}
}

func TestDoBackoff(t *testing.T) {
	pr := New()
	wg := sync.WaitGroup{}
	pr.Start(time.Millisecond)
	pr.Lock()
	pr.expBackoff.RandomizationFactor = 0
	pr.expBackoff.Multiplier = 2
	pr.Unlock()
	defer pr.Stop()

	i := 0
	upfunc := func() error {
		if i > 5 {
			wg.Done()
			return nil
		}
		i++

		return errors.New("test")
	}
	start := time.Now()
	wg.Add(1)
	pr.Do(upfunc)
	wg.Wait()

	elapsed := time.Now().Sub(start)
	expected := time.Millisecond * (1 + 2 + 4 + 5 + 6) // plus execution time
	if elapsed < expected {
		t.Errorf("backoff was not exponential")
	}
}
