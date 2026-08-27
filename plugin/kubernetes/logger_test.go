package kubernetes

import (
	"bytes"
	"errors"
	golog "log"
	"strings"
	"testing"

	clog "github.com/coredns/coredns/plugin/pkg/log"
)

func newTestLoggerAdapter(buf *bytes.Buffer) *loggerAdapter {
	golog.SetOutput(buf)
	return &loggerAdapter{P: clog.NewWithPlugin("kubernetes")}
}

func TestLoggerAdapterErrorIncludesKeysAndValues(t *testing.T) {
	var buf bytes.Buffer
	l := newTestLoggerAdapter(&buf)
	defer clog.Discard()

	err := errors.New(`Get "https://10.96.0.1:443/api/v1/services": i/o timeout`)
	l.Error(err, "Failed to watch", "reflector", "reflector-x", "type", "*v1.Service")

	got := buf.String()
	for _, want := range []string{
		"Failed to watch",
		`Get "https://10.96.0.1:443/api/v1/services": i/o timeout`,
		`reflector="reflector-x"`,
		`type="*v1.Service"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("log output missing %q, got: %q", want, got)
		}
	}
}

func TestLoggerAdapterErrorNilError(t *testing.T) {
	var buf bytes.Buffer
	l := newTestLoggerAdapter(&buf)
	defer clog.Discard()

	l.Error(nil, "Unexpected watch event object", "reflector", "reflector-x")

	got := buf.String()
	if strings.Contains(got, "<nil>") {
		t.Errorf("log output contains formatting artifact for nil error: %q", got)
	}
	for _, want := range []string{"Unexpected watch event object", `reflector="reflector-x"`} {
		if !strings.Contains(got, want) {
			t.Errorf("log output missing %q, got: %q", want, got)
		}
	}
}

func TestLoggerAdapterInfoIncludesKeysAndValues(t *testing.T) {
	var buf bytes.Buffer
	l := newTestLoggerAdapter(&buf)
	defer clog.Discard()

	err := errors.New("very short watch")
	l.Info(0, "Warning: watch ended with error", "reflector", "reflector-x", "err", err)

	got := buf.String()
	for _, want := range []string{
		"Warning: watch ended with error",
		`reflector="reflector-x"`,
		`err="very short watch"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("log output missing %q, got: %q", want, got)
		}
	}
}

func TestLoggerAdapterWithValues(t *testing.T) {
	var buf bytes.Buffer
	l := newTestLoggerAdapter(&buf)
	defer clog.Discard()

	sink := l.WithValues("reflector", "reflector-x")
	sink.Error(errors.New("boom"), "Failed to watch")

	got := buf.String()
	for _, want := range []string{"Failed to watch", "boom", `reflector="reflector-x"`} {
		if !strings.Contains(got, want) {
			t.Errorf("log output missing %q, got: %q", want, got)
		}
	}

	// WithValues must not mutate the parent sink.
	buf.Reset()
	l.Error(errors.New("boom"), "Failed to watch")
	if strings.Contains(buf.String(), "reflector-x") {
		t.Errorf("parent sink polluted by WithValues: %q", buf.String())
	}
}
