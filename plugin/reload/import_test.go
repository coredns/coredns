package reload

import (
	"bytes"
	"context"
	"crypto/sha512"
	"fmt"
	golog "log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	_ "github.com/coredns/coredns/plugin/bind"
	_ "github.com/coredns/coredns/plugin/template"
	_ "github.com/coredns/coredns/plugin/whoami"

	"github.com/miekg/dns"
)

var registerImportMutation sync.Once

// The startup callback changes the import after directives have been executed,
// but before InstanceStartupEvent starts the reload watcher.
func setupImportMutation(c *caddy.Controller) error {
	c.Next()
	args := c.RemainingArgs()
	if len(args) < 2 {
		return c.ArgErr()
	}
	if args[1] == "copy" && len(args) != 3 {
		return c.ArgErr()
	}
	c.OnStartup(func() error {
		switch args[1] {
		case "copy":
			contents, err := os.ReadFile(args[2])
			if err != nil {
				return err
			}
			return os.WriteFile(args[0], contents, 0600)
		case "remove":
			return os.Remove(args[0])
		case "invalid":
			return os.WriteFile(args[0], []byte("import\n"), 0600)
		default:
			return os.WriteFile(args[0], []byte(strings.Join(args[1:], " ")+"\n"), 0600)
		}
	})
	return nil
}

// Isolate Caddy's process-global configuration loader from the other tests.
// The import changes during startup; a client must eventually receive the new
// answer even when no further edit occurs to trigger another reload.
func TestReloadImportsConverge(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Caddy restart requires passing listener file descriptors")
	}
	const helper = "COREDNS_TEST_RELOAD_IMPORTS"
	if os.Getenv(helper) != "1" {
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, executable, "-test.run=^TestReloadImportsConverge$", "-test.count=1", "-test.timeout=45s", "-test.v")
		cmd.Env = append(os.Environ(), helper+"=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("reload subprocess: %v\n%s", err, output)
		}
		return
	}
	golog.SetOutput(os.Stderr)
	if err := dnsserver.Register(); err != nil {
		t.Fatal(err)
	}
	registerImportMutation.Do(func() { plugin.Register("test_reload_import", setupImportMutation) })
	dnsserver.Directives = append(dnsserver.Directives, "test_reload_import")
	dir := t.TempDir()
	importPath := filepath.Join(dir, "answer.conf")
	nextPath := filepath.Join(dir, "next.conf")
	writeAnswer := func(path, address string) {
		t.Helper()
		content := fmt.Sprintf("template IN A {\n answer \"{{.Name}} 60 IN A %s\"\n}\n", address)
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeAnswer(importPath, "192.0.2.1")
	writeAnswer(nextPath, "192.0.2.2")
	input := caddy.CaddyfileInput{
		Filepath: filepath.Join(dir, "Corefile"),
		Contents: fmt.Appendf(nil, `.:0 {
    bind 127.0.0.1
    reload 2s 0s
    import answer.conf
    test_reload_import %q copy %q
}
`, filepath.ToSlash(importPath), filepath.ToSlash(nextPath)),
		ServerTypeName: "dns",
	}
	canPoll := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(canPoll) })
	defer unblock()
	caddy.SetDefaultCaddyfileLoader("reload-import-test", caddy.LoaderFunc(func(string) (caddy.Input, error) {
		select {
		case <-canPoll:
			return input, nil
		case <-t.Context().Done():
			return nil, t.Context().Err()
		}
	}))
	started := make(chan struct{}, 3)
	caddy.RegisterEventHook("test_reload_import_started", func(event caddy.EventName, _ any) error {
		if event == caddy.InstanceStartupEvent {
			started <- struct{}{}
		}
		return nil
	})
	instance, err := caddy.Start(input)
	if err != nil {
		t.Fatal(err)
	}
	<-started
	t.Cleanup(func() {
		for _, current := range caddy.Instances() {
			for _, err := range current.ShutdownCallbacks() {
				if err != nil {
					t.Error(err)
				}
			}
		}
		if err := caddy.Stop(); err != nil {
			t.Error(err)
		}
		instance.Wait()
	})
	addresses := map[string]string{
		"udp": instance.Servers()[0].LocalAddr().String(),
		"tcp": instance.Servers()[0].Addr().String(),
	}
	for network, address := range addresses {
		if err := queryImportAnswer(network, address, "192.0.2.1"); err != nil {
			t.Fatalf("initial configuration: %v", err)
		}
	}
	unblock()
	for _, want := range []string{"192.0.2.2", "192.0.2.3"} {
		if want == "192.0.2.3" {
			writeAnswer(nextPath, want)
			writeAnswer(importPath, want)
		}
		for network, address := range addresses {
			deadline := time.Now().Add(10 * time.Second)
			for {
				err := queryImportAnswer(network, address, want)
				if err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("import update did not reach the running instance: %v", err)
				}
				time.Sleep(10 * time.Millisecond)
			}
		}
		// The new server can answer before Restart has stopped the old
		// instance. Wait for that handoff before editing again or cleaning up.
		select {
		case <-started:
		case <-time.After(10 * time.Second):
			t.Fatal("reload did not complete the instance handoff")
		}
	}
}

func queryImportAnswer(network, address, want string) error {
	query := new(dns.Msg)
	query.SetQuestion("example.org.", dns.TypeA)
	client := dns.Client{Net: network, Timeout: time.Second}
	response, _, err := client.Exchange(query, address)
	if err != nil {
		return err
	}
	if response.Rcode != dns.RcodeSuccess || len(response.Answer) != 1 {
		return fmt.Errorf("%s: unexpected response %s", network, response)
	}
	answer, ok := response.Answer[0].(*dns.A)
	if !ok || answer.A.String() != want {
		return fmt.Errorf("%s: answer %s, want %s", network, response.Answer[0], want)
	}
	return nil
}

func TestHookUsesExecutedImports(t *testing.T) {
	for _, tc := range []struct {
		name   string
		loaded string
		after  string
	}{
		{name: "unchanged", loaded: "reload 2s 0s", after: "reload 2s 0s"},
		{name: "changed", loaded: "reload 2s 0s", after: "reload 3s 0s"},
		{name: "reverted", loaded: "reload 3s 0s", after: "reload 2s 0s"},
		{name: "invalid", loaded: "reload 2s 0s", after: "invalid"},
		{name: "removed", loaded: "reload 2s 0s", after: "remove"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			importPath := filepath.Join(dir, "reload.conf")
			if err := os.WriteFile(importPath, []byte(tc.loaded+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			input := caddy.CaddyfileInput{
				Filepath: filepath.Join(dir, "Corefile"),
				Contents: fmt.Appendf(nil, `EXAMPLE.ORG:0 {
    bind 127.0.0.1
    whoami
    import reload.conf
    test_reload_import %q %s
}
`, filepath.ToSlash(importPath), tc.after),
				ServerTypeName: "dns",
			}
			checkStartupHash(t, input)
		})
	}
}

func TestHookHashesBlocksWithoutReload(t *testing.T) {
	dir := t.TempDir()
	importPath := filepath.Join(dir, "bind.conf")
	if err := os.WriteFile(importPath, []byte("bind 127.0.0.1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	input := caddy.CaddyfileInput{
		Filepath: filepath.Join(dir, "Corefile"),
		Contents: fmt.Appendf(nil, `EXAMPLE.ORG:0 {
    bind 127.0.0.1
    whoami
    reload 2s 0s
    test_reload_import %q bind 127.0.0.2
}
OTHER.EXAMPLE:0 {
    import bind.conf
    whoami
}
`, filepath.ToSlash(importPath)),
		ServerTypeName: "dns",
	}
	checkStartupHash(t, input)
}

func checkStartupHash(t *testing.T, input caddy.Input) {
	t.Helper()
	if err := dnsserver.Register(); err != nil {
		t.Fatal(err)
	}
	registerImportMutation.Do(func() { plugin.Register("test_reload_import", setupImportMutation) })
	oldDirectives, oldQuiet := dnsserver.Directives, caddy.Quiet
	dnsserver.Directives = append(append([]string(nil), oldDirectives...), "test_reload_import")
	caddy.Quiet = true
	t.Cleanup(func() { dnsserver.Directives, caddy.Quiet = oldDirectives, oldQuiet })
	parsed, err := parse(input)
	if err != nil {
		t.Fatal(err)
	}
	want := sha512.Sum512(parsed)
	var output importLog
	previous := golog.Writer()
	golog.SetOutput(&output)
	defer golog.SetOutput(previous)
	instance, err := caddy.Start(input)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, err := range instance.ShutdownCallbacks() {
			if err != nil {
				t.Error(err)
			}
		}
		if err := instance.Stop(); err != nil {
			t.Error(err)
		}
		instance.Wait()
	})
	// Confirm the instance is serving before checking its reload baseline.
	query := new(dns.Msg)
	query.SetQuestion("example.org.", dns.TypeA)
	client := dns.Client{Timeout: time.Second}
	if _, _, err := client.Exchange(query, instance.Servers()[0].LocalAddr().String()); err != nil {
		t.Fatal(err)
	}
	if wantLog := fmt.Sprintf("Running configuration SHA512 = %x", want); !strings.Contains(output.String(), wantLog) {
		t.Fatalf("startup fingerprint does not describe the executed configuration: want %q, got %q", wantLog, output.String())
	}
}

type importLog struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *importLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *importLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}
