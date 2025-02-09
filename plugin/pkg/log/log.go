// Package log implements a small wrapper around the std lib log package. It
// implements log levels by prefixing the logs with [INFO], [DEBUG], [WARNING]
// or [ERROR]. Debug logging is available and enabled if the *debug* plugin is
// used.
//
// log.Info("this is some logging"), will log on the Info level.
//
// log.Debug("this is debug output"), will log in the Debug level, etc.
package log

import (
	"fmt"
	"io"
	golog "log"
	"os"
	"sync/atomic"
)

// D controls whether we should output debug logs. If true, we do, once set
// it can not be unset.
var D = &d{}

type d struct {
	on atomic.Bool
}

// Set enables debug logging.
func (d *d) Set() {
	d.on.Store(true)
}

// Clear disables debug logging.
func (d *d) Clear() {
	d.on.Store(false)
}

// Value returns if debug logging is enabled.
func (d *d) Value() bool {
	return d.on.Load()
}

// logf calls log.Printf prefixed with level.
func logf(level, format string, v ...any) {
	golog.Print(level, fmt.Sprintf(format, v...))
}

// log calls log.Print prefixed with level.
func log(level string, v ...any) {
	golog.Print(level, fmt.Sprint(v...))
}

// Debug is equivalent to log.Print(), but prefixed with "[DEBUG] ". It only outputs something
// if D is true.
func Debug(v ...any) {
	if !D.Value() {
		return
	}
	log(debug, v...)
}

// Debugf is equivalent to log.Printf(), but prefixed with "[DEBUG] ". It only outputs something
// if D is true.
func Debugf(format string, v ...any) {
	if !D.Value() {
		return
	}
	logf(debug, format, v...)
}

// Info is equivalent to log.Print, but prefixed with "[INFO] ".
func Info(v ...any) { log(info, v...) }

// Infof is equivalent to log.Printf, but prefixed with "[INFO] ".
func Infof(format string, v ...any) { logf(info, format, v...) }

// Warning is equivalent to log.Print, but prefixed with "[WARNING] ".
func Warning(v ...any) { log(warning, v...) }

// Warningf is equivalent to log.Printf, but prefixed with "[WARNING] ".
func Warningf(format string, v ...any) { logf(warning, format, v...) }

// Error is equivalent to log.Print, but prefixed with "[ERROR] ".
func Error(v ...any) { log(err, v...) }

// Errorf is equivalent to log.Printf, but prefixed with "[ERROR] ".
func Errorf(format string, v ...any) { logf(err, format, v...) }

// Fatal is equivalent to log.Print, but prefixed with "[FATAL] ", and calling
// os.Exit(1).
func Fatal(v ...any) { log(fatal, v...); os.Exit(1) }

// Fatalf is equivalent to log.Printf, but prefixed with "[FATAL] ", and calling
// os.Exit(1)
func Fatalf(format string, v ...any) { logf(fatal, format, v...); os.Exit(1) }

// Discard sets the log output to /dev/null.
func Discard() { golog.SetOutput(io.Discard) }

const (
	debug   = "[DEBUG] "
	err     = "[ERROR] "
	fatal   = "[FATAL] "
	info    = "[INFO] "
	warning = "[WARNING] "
)
