package log

import (
	"sync"
)

// Listener listens for all log prints of plugin loggers aka loggers with plugin name.
// When a plugin logger gets called, it should first call the same method in the Listener object.
// A usage example is, the external plugin k8s_event will replicate log prints to Kubernetes events.
type Listener interface {
	Name() string
	Debug(plugin string, v ...any)
	Debugf(plugin string, format string, v ...any)
	Info(plugin string, v ...any)
	Infof(plugin string, format string, v ...any)
	Warning(plugin string, v ...any)
	Warningf(plugin string, format string, v ...any)
	Error(plugin string, v ...any)
	Errorf(plugin string, format string, v ...any)
	Fatal(plugin string, v ...any)
	Fatalf(plugin string, format string, v ...any)
}

type listeners struct {
	listeners []Listener
	sync.RWMutex
}

var ls *listeners

func init() {
	ls = &listeners{}
	ls.listeners = make([]Listener, 0)
}

// RegisterListener register a listener object.
func RegisterListener(new Listener) error {
	ls.Lock()
	defer ls.Unlock()
	for k, l := range ls.listeners {
		if l.Name() == new.Name() {
			ls.listeners[k] = new
			return nil
		}
	}
	ls.listeners = append(ls.listeners, new)
	return nil
}

// DeregisterListener deregister a listener object.
func DeregisterListener(old Listener) error {
	ls.Lock()
	defer ls.Unlock()
	for k, l := range ls.listeners {
		if l.Name() == old.Name() {
			ls.listeners = append(ls.listeners[:k], ls.listeners[k+1:]...)
			return nil
		}
	}
	return nil
}

func (ls *listeners) debug(plugin string, v ...any) {
	ls.RLock()
	for _, l := range ls.listeners {
		l.Debug(plugin, v...)
	}
	ls.RUnlock()
}

func (ls *listeners) debugf(plugin string, format string, v ...any) {
	ls.RLock()
	for _, l := range ls.listeners {
		l.Debugf(plugin, format, v...)
	}
	ls.RUnlock()
}

func (ls *listeners) info(plugin string, v ...any) {
	ls.RLock()
	for _, l := range ls.listeners {
		l.Info(plugin, v...)
	}
	ls.RUnlock()
}

func (ls *listeners) infof(plugin string, format string, v ...any) {
	ls.RLock()
	for _, l := range ls.listeners {
		l.Infof(plugin, format, v...)
	}
	ls.RUnlock()
}

func (ls *listeners) warning(plugin string, v ...any) {
	ls.RLock()
	for _, l := range ls.listeners {
		l.Warning(plugin, v...)
	}
	ls.RUnlock()
}

func (ls *listeners) warningf(plugin string, format string, v ...any) {
	ls.RLock()
	for _, l := range ls.listeners {
		l.Warningf(plugin, format, v...)
	}
	ls.RUnlock()
}

func (ls *listeners) error(plugin string, v ...any) {
	ls.RLock()
	for _, l := range ls.listeners {
		l.Error(plugin, v...)
	}
	ls.RUnlock()
}

func (ls *listeners) errorf(plugin string, format string, v ...any) {
	ls.RLock()
	for _, l := range ls.listeners {
		l.Errorf(plugin, format, v...)
	}
	ls.RUnlock()
}

func (ls *listeners) fatal(plugin string, v ...any) {
	ls.RLock()
	for _, l := range ls.listeners {
		l.Fatal(plugin, v...)
	}
	ls.RUnlock()
}

func (ls *listeners) fatalf(plugin string, format string, v ...any) {
	ls.RLock()
	for _, l := range ls.listeners {
		l.Fatalf(plugin, format, v...)
	}
	ls.RUnlock()
}
