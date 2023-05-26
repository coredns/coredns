package trace

import (
	clog "github.com/coredns/coredns/plugin/pkg/log"
)

// loggerAdapter is a simple adapter around plugin logger made to implement io.Writer and ddtrace.Logger interface
// in order to log errors from span reporters as warnings
type loggerAdapter struct {
	clog.P
}

func (l *loggerAdapter) Write(p []byte) (n int, err error) {
	l.P.Warning(string(p))
	return len(p), nil
}

func (l *loggerAdapter) Log(msg string) {
	l.P.Warning(msg)
}
