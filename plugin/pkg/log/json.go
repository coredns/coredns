package log

import (
	"context"
	"fmt"
	"io"
	golog "log"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
)

var jsonBackend atomic.Pointer[jsonLogger]

type jsonLogger struct {
	logger *slog.Logger
	output *logOutput
	flags  int
	prefix string
}

// Configure selects text (the default) or JSON logging and its destination.
// Call it at process startup, before starting servers. The format is process-wide
// and should not be changed by individual plugins or during a Corefile reload.
// JSON mode also routes the standard library's default logger through this
// backend, at INFO level, without interpreting message text as structured data.
func Configure(format string, output io.Writer) error {
	if format != "text" && format != "json" {
		return fmt.Errorf("unknown log format %q: expected text or json", format)
	}
	if output == nil {
		return fmt.Errorf("log output must not be nil")
	}

	flags, prefix := golog.Flags(), golog.Prefix()
	if previous := jsonBackend.Load(); previous != nil {
		flags, prefix = previous.flags, previous.prefix
	}
	if format == "text" {
		jsonBackend.Store(nil)
		golog.SetOutput(output)
		golog.SetFlags(flags)
		golog.SetPrefix(prefix)
		return nil
	}

	w := &logOutput{writer: output}
	l := slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: slog.LevelDebug,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.LevelKey {
				if level, ok := a.Value.Any().(slog.Level); ok && level == levelFatal {
					return slog.String(slog.LevelKey, "FATAL")
				}
			}
			return a
		},
	}))
	jsonBackend.Store(&jsonLogger{logger: l, output: w, flags: flags, prefix: prefix})
	golog.SetFlags(0)
	golog.SetPrefix("")
	golog.SetOutput(standardWriter{logger: l})
	return nil
}

// IsJSON reports whether structured logging is enabled.
func IsJSON() bool { return jsonBackend.Load() != nil }

// SetOutput changes the destination of both CoreDNS and standard-library logs.
// Use this instead of log.SetOutput when JSON logging is enabled.
func SetOutput(w io.Writer) {
	if b := jsonBackend.Load(); b != nil {
		b.output.mu.Lock()
		b.output.writer = w
		b.output.mu.Unlock()
		return
	}
	golog.SetOutput(w)
}

// InfoAttrs logs msg with typed attributes in JSON mode. In text mode it is
// equivalent to Info(msg). Like Info, it does not call named-plugin listeners.
func InfoAttrs(msg string, attrs ...slog.Attr) {
	if b := jsonBackend.Load(); b != nil {
		b.logger.LogAttrs(context.Background(), slog.LevelInfo, msg, attrs...)
		return
	}
	Info(msg)
}

func (b *jsonLogger) log(level, plugin, msg string) {
	lvl := slog.LevelInfo
	switch level {
	case debug:
		lvl = slog.LevelDebug
	case warning:
		lvl = slog.LevelWarn
	case err:
		lvl = slog.LevelError
	case fatal:
		lvl = levelFatal
	}
	if plugin != "" {
		b.logger.LogAttrs(context.Background(), lvl, msg, slog.String("plugin", plugin))
		return
	}
	b.logger.LogAttrs(context.Background(), lvl, msg)
}

const levelFatal = slog.Level(12)

// The adapter writes directly to the shared handler, not back through golog.
// This preserves multiline messages as one JSON record without recursion.
type standardWriter struct{ logger *slog.Logger }

func (w standardWriter) Write(p []byte) (int, error) {
	w.logger.Info(strings.TrimSuffix(string(p), "\n"))
	return len(p), nil
}

type logOutput struct {
	mu     sync.Mutex
	writer io.Writer
}

func (w *logOutput) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writer.Write(p)
}
