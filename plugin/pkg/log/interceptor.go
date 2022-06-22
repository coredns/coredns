package log

var i Interceptor

// LoadInterceptor loads an interceptor object here.
func LoadInterceptor(inter Interceptor) error {
	i = inter
	return nil
}

// Interceptor intercepts all log prints of plugin logger.
// When plugin logger gets called, it should first call the same method in the Interceptor object.
// A usage example is, the external plugin k8s_event will intercept log prints and report the logs to Kubernetes.
type Interceptor interface {
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
