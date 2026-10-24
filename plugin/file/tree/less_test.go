package tree

import (
	"sync"
	"testing"

	"github.com/miekg/dns"
)

// Test that concurrent calls to Less (which calls Elem.Name) do not race or panic.
// See issue #7561 for reference.
func TestLess_ConcurrentNameAccess(t *testing.T) {
	rr, err := dns.NewRR("a.example. 3600 IN A 1.2.3.4")
	if err != nil {
		t.Fatalf("failed to create RR: %v", err)
	}
	e := newElem(rr)

	const n = 200
	var wg sync.WaitGroup
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			// Compare the same name repeatedly; previously this could race due to lazy Name() writes.
			_ = Less(e, "a.example.")
			_ = e.Name()
		}()
	}
	wg.Wait()
}
