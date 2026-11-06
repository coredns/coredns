//go:build !windows && !plan9 && !nacl && !js

package reload

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	_ "github.com/coredns/coredns/plugin/bind"
	_ "github.com/coredns/coredns/plugin/template"

	"github.com/miekg/dns"
)

func TestReloadAfterSIGUSR1(t *testing.T) {
	corefilePath := filepath.Join(t.TempDir(), "Corefile")
	writeCorefile := func(address string) {
		t.Helper()
		contents := fmt.Sprintf(`.:0 {
	bind 127.0.0.1
	template IN A example.org {
		answer "{{ .Name }} 60 IN A %s"
	}
	reload 2s 0s
}
`, address)
		if err := os.WriteFile(corefilePath, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}

	writeCorefile("192.0.2.1")
	caddy.SetDefaultCaddyfileLoader("reload-sigusr1-test", caddy.LoaderFunc(func(string) (caddy.Input, error) {
		contents, err := os.ReadFile(corefilePath)
		return caddy.CaddyfileInput{
			Filepath:       corefilePath,
			Contents:       contents,
			ServerTypeName: "dns",
		}, err
	}))

	oldInstanceInput, err := caddy.LoadCaddyfile("dns")
	if err != nil {
		t.Fatal(err)
	}
	dnsserver.Quiet = true
	oldInstance, err := caddy.Start(oldInstanceInput)
	if err != nil {
		t.Fatal(err)
	}
	var currentInstance *caddy.Instance
	t.Cleanup(func() {
		if currentInstance == nil {
			_ = caddy.Stop()
			return
		}
		for _, err := range currentInstance.ShutdownCallbacks() {
			if err != nil {
				t.Errorf("shutdown CoreDNS: %v", err)
			}
		}
		if err := currentInstance.Stop(); err != nil {
			t.Errorf("stop CoreDNS: %v", err)
		}
		currentInstance.Wait()
	})

	address := dnsServerAddress(oldInstance)
	waitForA(t, address, "192.0.2.1", 2*time.Second)

	// Make SIGUSR1 load this change directly before the background watcher has
	// a chance to observe it.
	writeCorefile("192.0.2.2")
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGUSR1)
	defer signal.Stop(signalCh)
	caddy.TrapSignals()
	// TrapSignals installs its handlers from a goroutine.
	time.Sleep(100 * time.Millisecond)
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGUSR1); err != nil {
		t.Fatal(err)
	}
	select {
	case <-signalCh:
	case <-time.After(2 * time.Second):
		t.Fatal("SIGUSR1 was not delivered")
	}

	// SIGUSR1 must have loaded the changed configuration itself.
	waitForA(t, address, "192.0.2.2", 5*time.Second)

	started := make(chan *caddy.Instance, 1)
	caddy.RegisterEventHook("reload-sigusr1-test", func(event caddy.EventName, info any) error {
		if event == caddy.InstanceStartupEvent {
			started <- info.(*caddy.Instance)
		}
		return nil
	})

	// The SIGUSR1 restart purges Caddy's event hooks. A later Corefile change
	// must still be observed by the reload plugin after the hook is restored.
	writeCorefile("192.0.2.3")
	select {
	case currentInstance = <-started:
	case <-time.After(8 * time.Second):
		t.Fatal("Corefile change did not restart CoreDNS")
	}
	waitForA(t, address, "192.0.2.3", 2*time.Second)
}

func dnsServerAddress(instance *caddy.Instance) string {
	if address := instance.Servers()[0].LocalAddr(); address != nil {
		return address.String()
	}
	return instance.Servers()[0].Addr().String()
}

func waitForA(t *testing.T, address, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		message := new(dns.Msg)
		message.SetQuestion("example.org.", dns.TypeA)
		client := dns.Client{Net: "udp", Timeout: 500 * time.Millisecond}
		response, _, err := client.Exchange(message, address)
		if err == nil && len(response.Answer) == 1 {
			if answer, ok := response.Answer[0].(*dns.A); ok && answer.A.String() == want {
				return
			}
			lastErr = fmt.Errorf("unexpected response: %s", response)
		} else if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("unexpected response: %s", response)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("DNS answer did not become %s: %v", want, lastErr)
}
