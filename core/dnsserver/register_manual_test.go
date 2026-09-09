//go:build coredns_manual_registration

package dnsserver_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
)

const manualRegistrationHelper = "COREDNS_TEST_MANUAL_REGISTRATION"

func TestMain(m *testing.M) {
	// Most package tests need a registered server type. Fresh subprocesses skip
	// this setup so their first Register call exercises the import-time state.
	if os.Getenv(manualRegistrationHelper) == "" {
		if err := dnsserver.Register(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	os.Exit(m.Run())
}

func TestManualRegistrationImport(t *testing.T) {
	if scenario := os.Getenv(manualRegistrationHelper); scenario != "" {
		checkManualRegistration(t, scenario)
		return
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"start", "concurrent", "conflict"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, executable, "-test.run=^TestManualRegistrationImport$", "-test.count=1")
			cmd.Env = append(os.Environ(), manualRegistrationHelper+"="+scenario)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("fresh-process registration check: %v\n%s", err, output)
			}
		})
	}
}

func checkManualRegistration(t *testing.T, scenario string) {
	t.Helper()
	if slices.Contains(caddy.ListPlugins()["server_types"], "dns") {
		t.Fatal("importing dnsserver registered the DNS server type in manual mode")
	}
	TestEmbeddingDoesNotRegisterCLIFlags(t)
	configureEmbedding(t)
	if slices.Contains(caddy.ListPlugins()["server_types"], "dns") {
		t.Fatal("selecting directives or registering a host plugin registered the server type")
	}

	switch scenario {
	case "start":
		instance, err := caddy.Start(caddy.CaddyfileInput{
			Filepath:       "Corefile",
			Contents:       []byte(".:0 {\nbind 127.0.0.1\n}\n"),
			ServerTypeName: "dns",
		})
		if err == nil {
			stopEmbeddedInstance(t, instance)
			t.Fatal("starting without Register succeeded")
		}
		if !strings.Contains(err.Error(), "no server types plugged in") {
			t.Fatalf("unexpected unregistered startup error: %v", err)
		}
		if len(instance.Servers()) != 0 || len(caddy.Instances()) != 0 {
			t.Fatal("failed startup left servers or instances behind")
		}
	case "concurrent":
		const callers = 32
		errs := make(chan error, callers)
		var wg sync.WaitGroup
		for range callers {
			wg.Go(func() { errs <- dnsserver.Register() })
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
	case "conflict":
		want := []string{"test_foreign_server"}
		caddy.RegisterServerType("dns", caddy.ServerType{
			Directives: func() []string { return want },
		})
		for range 2 {
			if err := dnsserver.Register(); err == nil || !strings.Contains(err.Error(), `server type "dns" already registered`) {
				t.Fatalf("expected registration conflict, got %v", err)
			}
			if got := caddy.ValidDirectives("dns"); !slices.Equal(got, want) {
				t.Fatalf("conflicting server type changed: got %v, want %v", got, want)
			}
		}
		return
	default:
		t.Fatalf("unknown registration scenario %q", scenario)
	}

	for range 2 {
		if err := dnsserver.Register(); err != nil {
			t.Fatal(err)
		}
	}
	if got := caddy.ValidDirectives("dns"); !slices.Equal(got, dnsserver.Directives) {
		t.Fatalf("registration changed the host's directive list: %v", got)
	}
	t.Run("forward", TestEmbeddingForward)
}
