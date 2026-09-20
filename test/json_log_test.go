package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/coremain"
	clog "github.com/coredns/coredns/plugin/pkg/log"

	"github.com/miekg/dns"
)

func TestJSONLoggingProcess(t *testing.T) {
	for _, tc := range []struct {
		name   string
		args   []string
		config string
		serve  bool
		fatal  bool
		json   bool
	}{
		{"serve", []string{"-log-format=json", "-conf=stdin"}, jsonLogCorefile, true, false, true},
		{"quiet", []string{"-log-format=json", "-quiet", "-conf=stdin"}, jsonLogCorefile, true, false, true},
		{"signal-shutdown", []string{"-log-format=json", "-conf=stdin"}, jsonLogCorefile, true, false, true},
		{"bad-corefile", []string{"-log-format=json", "-conf=stdin"}, ".:0 {\n invalid-plugin\n}", false, true, true},
		{"missing-corefile", []string{"-log-format=json", "-conf=does-not-exist/Corefile"}, "", false, true, true},
		{"extra-args", []string{"-log-format=json", "unexpected"}, "", false, true, true},
		{"invalid-format", []string{"-log-format=invalid"}, "", false, true, false},
		{"version", []string{"-log-format=json", "-version"}, "", false, false, false},
		{"plugins", []string{"-log-format=json", "-plugins"}, "", false, false, false},
		{"text-default", []string{"-conf=stdin"}, jsonLogCorefile, true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "signal-shutdown" && runtime.GOOS == "windows" {
				t.Skip("Windows does not support sending os.Interrupt to a process")
			}
			bin, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			args, err := json.Marshal(tc.args)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, bin, "-test.run=^TestJSONLoggingProcessHelper$")
			cmd.Env = append(os.Environ(), "COREDNS_TEST_LOG_ARGS="+string(args),
				fmt.Sprintf("COREDNS_TEST_LOG_SERVE=%t", tc.serve),
				fmt.Sprintf("COREDNS_TEST_LOG_SIGNAL=%t", tc.name == "signal-shutdown"), "GORACE=atexit_sleep_ms=0")
			cmd.Stdin = strings.NewReader(tc.config)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err = cmd.Run()
			if tc.fatal {
				if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
					t.Fatalf("expected exit 1, got %v\nstdout: %s\nstderr: %s", err, &stdout, &stderr)
				}
			} else if err != nil {
				t.Fatalf("process failed: %v\nstdout: %s\nstderr: %s", err, &stdout, &stderr)
			}
			if !tc.json {
				if tc.serve && !strings.Contains(stdout.String(), "[INFO]") {
					t.Fatalf("default text logging changed: %s", &stdout)
				}
				if stdout.Len()+stderr.Len() == 0 || json.Valid(append(stdout.Bytes(), stderr.Bytes()...)) {
					t.Fatal("expected human-readable output")
				}
				return
			}
			queries, fatal, startup, afterReload := 0, 0, 0, 0
			shutdown := false
			for stream, output := range map[string]*bytes.Buffer{"stdout": &stdout, "stderr": &stderr} {
				for line := range bytes.SplitSeq(bytes.TrimSpace(output.Bytes()), []byte("\n")) {
					if len(line) == 0 {
						continue
					}
					var record map[string]any
					if err := json.Unmarshal(line, &record); err != nil {
						t.Fatalf("non-JSON %s: %q: %v", stream, line, err)
					}
					if record["time"] == nil || record["level"] == nil || record["msg"] == nil {
						t.Fatalf("missing common fields: %v", record)
					}
					if record["level"] == "FATAL" {
						fatal++
						if stream != "stderr" {
							t.Error("startup fatal log must go to stderr")
						}
					}
					msg, _ := record["msg"].(string)
					shutdown = shutdown || strings.Contains(msg, "SIGINT: Shutting down")
					if strings.Contains(msg, "CoreDNS-") {
						startup++
					}
					if tc.name == "quiet" && strings.Contains(msg, ":0 on 127.0.0.1") {
						t.Error("quiet mode emitted listener startup output")
					}
					if record["plugin"] == "log" {
						queries++
						if record["rcode"] != "NOERROR" || record["client_ip"] != "127.0.0.1" {
							t.Errorf("wrong query result: %v", record)
						}
						qname := record["qname"].(string)
						if !strings.Contains(msg, qname) {
							t.Errorf("custom format lost: %v", record)
						}
						if strings.HasSuffix(qname, ".example.org.") && !strings.Contains(msg, "-special ") {
							t.Errorf("server block's custom format lost: %v", record)
						}
						if strings.HasPrefix(msg, "after-") {
							afterReload++
						}
					}
				}
			}
			if tc.fatal && fatal != 1 {
				t.Errorf("expected one fatal record, got %d", fatal)
			}
			if tc.serve {
				wantQueries := 4
				if runtime.GOOS != "windows" && tc.name != "signal-shutdown" {
					wantQueries = 12
				}
				if queries != wantQueries {
					t.Errorf("expected %d query records, got %d", wantQueries, queries)
				}
				if runtime.GOOS != "windows" && tc.name != "signal-shutdown" && afterReload != 4 {
					t.Errorf("expected four records with the reloaded FORMAT, got %d", afterReload)
				}
				if tc.name == "quiet" && startup != 0 || tc.name == "serve" && startup != 1 {
					t.Errorf("unexpected startup version count: %d", startup)
				}
				if tc.name == "signal-shutdown" && !shutdown {
					t.Error("missing JSON shutdown record")
				}
			}
		})
	}
}

func TestJSONLoggingProcessHelper(t *testing.T) {
	encoded := os.Getenv("COREDNS_TEST_LOG_ARGS")
	if encoded == "" {
		return
	}
	var args []string
	if err := json.Unmarshal([]byte(encoded), &args); err != nil {
		t.Fatal(err)
	}
	os.Args = append([]string{os.Args[0]}, args...)
	done := make(chan error, 1)
	if os.Getenv("COREDNS_TEST_LOG_SERVE") == "true" {
		var once sync.Once
		caddy.RegisterEventHook("json-log-test", func(event caddy.EventName, value any) error {
			if event == caddy.InstanceStartupEvent {
				once.Do(func() {
					go func() {
						inst := value.(*caddy.Instance)
						if os.Getenv("COREDNS_TEST_LOG_SIGNAL") == "true" {
							if err := interruptLogServer(inst); err != nil {
								inst.Stop()
								inst.ShutdownCallbacks()
								done <- err
							}
							// Let Caddy's signal handler finish shutdown and exit.
							return
						}
						done <- exerciseLogServer(inst)
					}()
				})
			}
			return nil
		})
	}
	coremain.Run()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	// Avoid the test runner's own PASS line in the server's output stream.
	os.Exit(0)
}

func interruptLogServer(inst *caddy.Instance) error {
	if err := queryLogServer(inst); err != nil {
		return err
	}
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		return err
	}
	defer p.Release()
	return p.Signal(os.Interrupt)
}

func exerciseLogServer(inst *caddy.Instance) error {
	defer func() {
		inst.Stop()
		inst.ShutdownCallbacks()
	}()
	if err := queryLogServer(inst); err != nil {
		return err
	}
	// Windows cannot inherit Caddy's listener file descriptors on restart.
	if runtime.GOOS == "windows" {
		return nil
	}
	if _, err := inst.Restart(NewInput(".:0 {\n invalid-plugin\n}")); err == nil {
		return fmt.Errorf("invalid Corefile was accepted on reload")
	}
	if err := queryLogServer(inst); err != nil {
		return err
	}
	newInst, err := inst.Restart(NewInput(strings.ReplaceAll(jsonLogCorefile, "before", "after")))
	if err != nil {
		return err
	}
	inst = newInst
	return queryLogServer(inst)
}

func queryLogServer(inst *caddy.Instance) error {
	udp, tcp := CoreDNSServerPorts(inst, 0)
	for protocol, addr := range map[string]string{"udp": udp, "tcp": tcp} {
		for _, name := range []string{`a\.b.example.org.`, `a\001b.example.net.`} {
			query := new(dns.Msg)
			query.SetQuestion(name, dns.TypeA)
			client := &dns.Client{Net: protocol, Timeout: 2 * time.Second}
			reply, _, err := client.Exchange(query, addr)
			if err != nil {
				return fmt.Errorf("%s %s: %w", protocol, name, err)
			}
			if reply.Rcode != dns.RcodeSuccess || len(reply.Extra) != 2 {
				return fmt.Errorf("unexpected %s response: %v", protocol, reply)
			}
		}
	}
	clog.NewWithPlugin("json-test").Error("multiline error\nwith a stack trace")
	return nil
}

const jsonLogCorefile = `.:0 {
    bind 127.0.0.1
    log . "before-default {name}"
    whoami
}
example.org.:0 {
    bind 127.0.0.1
    log . "before-special {name}"
    whoami
}`
