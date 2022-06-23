package log

var listeners []Listener

// RegisterListener register a listener object.
func RegisterListener(new Listener) error {
	if listeners == nil {
		listeners = make([]Listener, 0)
	}
	for k, l := range listeners {
		if l.Name() == new.Name() {
			listeners[k] = new
			return nil
		}
	}
	listeners = append(listeners, new)
	return nil
}

// DeregisterListener deregister a listener object.
func DeregisterListener(old Listener) error {
	if listeners == nil {
		return nil
	}
	for k, l := range listeners {
		if l.Name() == old.Name() {
			listeners = append(listeners[:k], listeners[k+1:]...)
			return nil
		}
	}
	return nil
}

// Listener listens for all log prints of plugin loggers aka loggers with plugin name.
// When a plugin logger gets called, it should first call the same method in the Listener object.
// A usage example is, the external plugin k8s_event will replicate log prints to Kubernetes events.
type Listener interface {
	Name() string
	Debug(plugin string, v ...interface{})
	Debugf(plugin string, format string, v ...interface{})
	Info(plugin string, v ...interface{})
	Infof(plugin string, format string, v ...interface{})
	Warning(plugin string, v ...interface{})
	Warningf(plugin string, format string, v ...interface{})
	Error(plugin string, v ...interface{})
	Errorf(plugin string, format string, v ...interface{})
	Fatal(plugin string, v ...interface{})
	Fatalf(plugin string, format string, v ...interface{})
}
