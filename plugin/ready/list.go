package ready

import (
	"sort"
	"strings"
	"sync"
)

// list is a structure that holds the plugins that signals readiness for this server block.
type list struct {
	sync.RWMutex
	rs map[string]Readiness

	// keepReadiness indicates whether the readiness status of plugins should be retained
	// after they have been confirmed as ready. When set to false, the plugin readiness
	// status will be reset to nil to conserve resources, assuming ready plugins don't
	// need continuous monitoring.
	keepReadiness bool
}

// Reset resets l
func (l *list) Reset() {
	l.Lock()
	defer l.Unlock()
	l.rs = nil
}

// Append adds a new readiness to l.
func (l *list) Append(r Readiness, name string) {
	l.Lock()
	defer l.Unlock()

	if l.rs == nil {
		l.rs = make(map[string]Readiness)
	}
	l.rs[name] = r
}

// Ready return true when all plugins ready, if the returned value is false the string
// contains a comma separated list of plugins that are not ready.
func (l *list) Ready() (bool, string) {
	l.Lock()
	defer l.Unlock()
	ok := true
	s := []string{}
	for name, r := range l.rs {
		if r == nil {
			continue
		}
		if r.Ready() {
			if !l.keepReadiness {
				l.rs[name] = nil
			}
			continue
		}
		ok = false
		s = append(s, name)
	}
	if ok {
		return true, ""
	}
	sort.Strings(s)
	return false, strings.Join(s, ",")
}
