package kubernetes

import (
	"fmt"
	"slices"
	"strings"

	clog "github.com/coredns/coredns/plugin/pkg/log"

	"github.com/go-logr/logr"
)

// missingValue is what klog prints for a key without a value.
const missingValue = "(MISSING)"

// loggerAdapter is a simple wrapper around CoreDNS plugin logger made to implement logr.LogSink interface, which is used
// as part of klog library for logging in Kubernetes client. By using this adapter CoreDNS is able to log messages/errors from
// kubernetes client in a CoreDNS logging format
type loggerAdapter struct {
	clog.P
	values []any // always even-length, see WithValues
}

func (l *loggerAdapter) Init(_ logr.RuntimeInfo) {
}

func (l *loggerAdapter) Enabled(_ int) bool {
	// verbosity is controlled inside klog library, we do not need to do anything here
	return true
}

func (l *loggerAdapter) Info(_ int, msg string, keysAndValues ...any) {
	l.P.Info(l.sprint(msg, nil, keysAndValues))
}

func (l *loggerAdapter) Error(err error, msg string, keysAndValues ...any) {
	l.P.Error(l.sprint(msg, err, keysAndValues))
}

func (l *loggerAdapter) WithValues(keysAndValues ...any) logr.LogSink {
	if len(keysAndValues) == 0 {
		return l
	}
	clone := *l
	clone.values = append(slices.Clip(l.values), keysAndValues...)
	if len(clone.values)%2 != 0 {
		// pad like klog does, so later pairs don't shift
		clone.values = append(clone.values, missingValue)
	}
	return &clone
}

func (l *loggerAdapter) WithName(_ string) logr.LogSink {
	return l
}

// sprint renders the message followed by the error (if any) and key/value pairs,
// since client-go puts most context (resource type, reflector name) in the latter.
// Duplicate keys are collapsed with the last value winning, as in klog.
func (l *loggerAdapter) sprint(msg string, err error, keysAndValues []any) string {
	var b strings.Builder
	b.WriteString(msg)
	if err != nil {
		b.WriteString(": ")
		b.WriteString(err.Error())
	}
	kvs := keysAndValues
	if len(l.values) > 0 {
		kvs = append(slices.Clip(l.values), keysAndValues...)
	}
	for i := 0; i < len(kvs); i += 2 {
		k := fmt.Sprint(kvs[i])
		if hasLaterKey(kvs, i+2, k) {
			continue
		}
		var v any = missingValue
		if i+1 < len(kvs) {
			v = kvs[i+1]
		}
		switch v := v.(type) {
		case string:
			fmt.Fprintf(&b, " %s=%q", k, v)
		case error:
			fmt.Fprintf(&b, " %s=%q", k, v.Error())
		default:
			fmt.Fprintf(&b, " %s=%v", k, v)
		}
	}
	return b.String()
}

func hasLaterKey(kvs []any, from int, key string) bool {
	for j := from; j < len(kvs); j += 2 {
		if fmt.Sprint(kvs[j]) == key {
			return true
		}
	}
	return false
}
