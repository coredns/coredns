package opentelemetry

import (
	clog "github.com/coredns/coredns/plugin/pkg/log"
)

// loggerAdapter is a simple adapter around plugin logger made to implement io.Writer
type loggerAdapter struct {
	clog.P
}

func (l *loggerAdapter) Write(p []byte) (n int, err error) {
	l.P.Debug(string(p))
	return len(p), nil
}
