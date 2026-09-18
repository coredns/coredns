package log

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	golog "log"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

func configureTestLog(tb testing.TB, format string, w io.Writer) {
	tb.Helper()
	output, flags, prefix, debug := golog.Writer(), golog.Flags(), golog.Prefix(), D.Value()
	tb.Cleanup(func() {
		if err := Configure("text", output); err != nil {
			tb.Error(err)
		}
		golog.SetFlags(flags)
		golog.SetPrefix(prefix)
		if debug {
			D.Set()
		} else {
			D.Clear()
		}
	})
	if err := Configure(format, w); err != nil {
		tb.Fatal(err)
	}
}

func TestJSONLevels(t *testing.T) {
	var out bytes.Buffer
	configureTestLog(t, "json", &out)
	D.Set()
	p := NewWithPlugin("test\"\nplugin")
	for _, tc := range []struct {
		level string
		plain func(...any)
		args  func(string, ...any)
		named func(...any)
		fmt   func(string, ...any)
	}{
		{"DEBUG", Debug, Debugf, p.Debug, p.Debugf},
		{"INFO", Info, Infof, p.Info, p.Infof},
		{"WARN", Warning, Warningf, p.Warning, p.Warningf},
		{"ERROR", Error, Errorf, p.Error, p.Errorf},
	} {
		t.Run(tc.level, func(t *testing.T) {
			msg := "quoted \"value\"\nnext\rline\t\\end"
			tc.plain(msg)
			tc.args("%s", msg)
			tc.named(msg)
			tc.fmt("%s", msg)
			lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
			if len(lines) != 4 {
				t.Fatalf("expected four JSON lines, got %q", out.String())
			}
			for i, line := range lines {
				var record map[string]any
				if err := json.Unmarshal([]byte(line), &record); err != nil {
					t.Fatalf("invalid JSON: %s: %v", line, err)
				}
				if record["level"] != tc.level || record["msg"] != msg {
					t.Errorf("unexpected record: %v", record)
				}
				stamp, ok := record["time"].(string)
				if !ok {
					t.Fatal("missing timestamp")
				}
				if _, err := time.Parse(time.RFC3339Nano, stamp); err != nil {
					t.Error(err)
				}
				if i < 2 {
					if _, ok := record["plugin"]; ok {
						t.Errorf("global log has plugin: %v", record)
					}
				} else if record["plugin"] != p.name {
					t.Errorf("plugin identity lost: %v", record)
				}
			}
			out.Reset()
		})
	}
	D.Clear()
	Debug("hidden")
	Debugf("%s", "hidden")
	p.Debug("hidden")
	p.Debugf("%s", "hidden")
	if out.Len() != 0 {
		t.Fatalf("debug logs emitted while disabled: %s", &out)
	}
}

func TestJSONStandardLoggerAndOutput(t *testing.T) {
	var out, redirected bytes.Buffer
	configureTestLog(t, "json", &out)
	golog.Print("first\nsecond")
	InfoAttrs("query", slog.Int("size", 12), slog.Bool("do", false))
	dec := json.NewDecoder(&out)
	for i := range 2 {
		var record map[string]any
		if err := dec.Decode(&record); err != nil {
			t.Fatal(err)
		}
		if i == 0 && record["msg"] != "first\nsecond" {
			t.Fatalf("standard logger message changed: %v", record)
		}
		if i == 1 && (record["size"] != float64(12) || record["do"] != false) {
			t.Fatalf("attributes lost their types: %v", record)
		}
	}
	SetOutput(&redirected)
	golog.Print("redirected standard")
	Info("redirected CoreDNS")
	if strings.Count(redirected.String(), "\n") != 2 {
		t.Fatalf("expected two redirected records, got %q", redirected.String())
	}
	before := redirected.String()
	Discard()
	golog.Print("discarded standard")
	Info("discarded CoreDNS")
	if redirected.String() != before {
		t.Fatal("Discard did not stop all output")
	}
}

func TestJSONListeners(t *testing.T) {
	var out bytes.Buffer
	configureTestLog(t, "json", &out)
	listener := &jsonListener{mockListener: *NewMockListener("json-test")}
	if err := RegisterListener(listener); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := DeregisterListener(listener); err != nil {
			t.Error(err)
		}
	})
	NewWithPlugin("example").Info("unchanged", 1)
	InfoAttrs("query", slog.String("plugin", "log"))
	if listener.calls != 1 || listener.plugin != "plugin/example: " || listener.msg != "unchanged1" {
		t.Fatalf("listener contract changed: %+v", listener)
	}
	if strings.Count(out.String(), "\n") != 2 {
		t.Fatalf("listener caused missing or duplicate output: %q", out.String())
	}
}

type jsonListener struct {
	mockListener
	calls  int
	plugin string
	msg    string
}

func (l *jsonListener) Info(plugin string, v ...any) {
	l.calls++
	l.plugin, l.msg = plugin, fmt.Sprint(v...)
}

func TestJSONConcurrent(t *testing.T) {
	var out bytes.Buffer
	configureTestLog(t, "json", &out)
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for i := range 100 {
				InfoAttrs("query", slog.Int("id", i))
				NewWithPlugin("concurrent").Errorf("error %d", i)
				golog.Printf("legacy %d", i)
			}
		})
	}
	wg.Wait()
	lines := bytes.Split(bytes.TrimSuffix(out.Bytes(), []byte("\n")), []byte("\n"))
	if len(lines) != 4800 {
		t.Fatalf("expected 4800 records, got %d", len(lines))
	}
	for _, line := range lines {
		if !json.Valid(line) {
			t.Fatalf("interleaved JSON record: %q", line)
		}
	}
}

func TestConfigureTextCompatibility(t *testing.T) {
	var out bytes.Buffer
	configureTestLog(t, "text", &out)
	golog.SetFlags(0)
	golog.SetPrefix("prefix: ")
	emit := func() {
		Info("a", 1)
		NewWithPlugin("test").Infof("value %d", 2)
		InfoAttrs("attrs", slog.Int("size", 3))
		golog.Print("legacy")
	}
	want := "prefix: [INFO] a1\nprefix: [INFO] plugin/test: value 2\nprefix: [INFO] attrs\nprefix: legacy\n"
	emit()
	if out.String() != want {
		t.Fatalf("text output changed: %q", out.String())
	}
	if err := Configure("json", io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := Configure("text", &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	emit()
	if out.String() != want || IsJSON() {
		t.Fatalf("text settings not restored: %q", out.String())
	}
	if err := Configure("invalid", io.Discard); err == nil {
		t.Fatal("invalid format accepted")
	}
	if err := Configure("json", nil); err == nil || IsJSON() {
		t.Fatal("invalid configuration changed logging mode")
	}
}

func TestJSONFatal(t *testing.T) {
	for _, mode := range []string{"global", "globalf", "plugin", "pluginf"} {
		t.Run(mode, func(t *testing.T) {
			bin, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(bin, "-test.run=^TestJSONFatalHelper$")
			cmd.Env = append(os.Environ(), "COREDNS_TEST_JSON_FATAL="+mode, "GORACE=atexit_sleep_ms=0")
			out, err := cmd.CombinedOutput()
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
				t.Fatalf("expected exit 1, got %v: %s", err, out)
			}
			var record map[string]any
			if err := json.Unmarshal(out, &record); err != nil {
				t.Fatalf("invalid fatal log %q: %v", out, err)
			}
			if record["level"] != "FATAL" || record["msg"] != "fatal\nmessage" {
				t.Fatalf("unexpected fatal record: %v", record)
			}
			if strings.HasPrefix(mode, "plugin") && record["plugin"] != "test" {
				t.Fatalf("missing plugin: %v", record)
			}
		})
	}
}

func TestJSONFatalHelper(t *testing.T) {
	mode := os.Getenv("COREDNS_TEST_JSON_FATAL")
	if mode == "" {
		return
	}
	if err := Configure("json", os.Stdout); err != nil {
		t.Fatal(err)
	}
	switch mode {
	case "global":
		Fatal("fatal\nmessage")
	case "globalf":
		Fatalf("%s", "fatal\nmessage")
	case "plugin":
		NewWithPlugin("test").Fatal("fatal\nmessage")
	case "pluginf":
		NewWithPlugin("test").Fatalf("%s", "fatal\nmessage")
	}
}

func BenchmarkLogFormat(b *testing.B) {
	for _, format := range []string{"text", "json"} {
		b.Run(format, func(b *testing.B) {
			configureTestLog(b, format, io.Discard)
			p := NewWithPlugin("test")
			b.ReportAllocs()
			for b.Loop() {
				p.Infof("query %s returned %d", "example.org.", 0)
			}
		})
	}
}
