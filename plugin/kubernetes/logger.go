package kubernetes

import (
	clog "github.com/coredns/coredns/plugin/pkg/log"

	"github.com/go-logr/logr"
)

// LoggerAdapter is a simple wrapper around CoreDNS plugin logger made to implement logr.LogSink interface, which is used
// as part of klog library for logging in Kubernetes client. By using this adapter CoreDNS is able to log messages/errors from
// kubernetes client in a CoreDNS logging format
type LoggerAdapter struct {
	clog.P
}

func (l *LoggerAdapter) Init(_ logr.RuntimeInfo) {
}

func (l *LoggerAdapter) Enabled(_ int) bool {
	// verbosity is controlled inside klog library, we do not need to do anything here
	return true
}

func (l *LoggerAdapter) Info(_ int, msg string, _ ...interface{}) {
	l.P.Info(msg)
}

func (l *LoggerAdapter) Error(_ error, msg string, _ ...interface{}) {
	l.P.Error(msg)
}

func (l *LoggerAdapter) WithValues(_ ...interface{}) logr.LogSink {
	return l
}

func (l *LoggerAdapter) WithName(_ string) logr.LogSink {
	return l
}
