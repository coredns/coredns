// Package uniq keeps track of "thing" that are either "todo" or "done". Multiple
// identical events will only be processed once.
package uniq

import (
	"sync"
)

// U keeps track of item to be done.
type U struct {
	sync.RWMutex
	u map[string]item
}

type item struct {
	state int          // either todo or done
	f     func() error // function to be executed.
}

// New returns a new initialized U.
func New() *U { return &U{u: make(map[string]item)} }

// Set sets function f in U under key. If the key already exists
// it is not overwritten.
func (u *U) Set(key string, f func() error) {
	u.Lock()
	defer u.Unlock()
	if _, ok := u.u[key]; ok {
		return
	}
	u.u[key] = item{todo, f}
}

// ForEach iterates for u executes f for each element that is 'todo' and sets it to 'done' - if f executes w/o error
func (u *U) ForEach() error {
	u.Lock()
	defer u.Unlock()
	for k, v := range u.u {
		if v.state == todo {
			if v.f() == nil {
				// change the state only if an error did not occur
				v.state = done
				u.u[k] = v
			}
		}
	}
	return nil
}

// HasTodo inform on weather some things are still in todo mode
func (u *U) HasTodo() bool {
	u.RLock()
	defer u.RUnlock()
	for _, v := range u.u {
		if v.state == todo {
			return true
		}
	}
	return false
}

const (
	todo = 1
	done = 2
)
