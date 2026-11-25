package ready

import (
	"slices"
	"strings"
	"sync"
)

// list holds the readiness checks for one HTTP endpoint.
type list struct {
	sync.Mutex
	checks []readinessCheck
}

type readinessCheck struct {
	name      string
	readiness Readiness
	monitor   monitorType
}

func (l *list) replace(checks []readinessCheck) {
	l.Lock()
	defer l.Unlock()
	l.checks = checks
}

// Ready reports whether all checks are ready and returns the names of any unready plugins.
func (l *list) Ready() (bool, string) {
	l.Lock()
	defer l.Unlock()
	var pending []string
	for i := range l.checks {
		check := &l.checks[i]
		if check.readiness == nil {
			continue
		}
		if check.readiness.Ready() {
			if check.monitor == monitorTypeUntilReady {
				check.readiness = nil
			}
			continue
		}
		pending = append(pending, check.name)
	}
	slices.Sort(pending)
	return len(pending) == 0, strings.Join(slices.Compact(pending), ",")
}
