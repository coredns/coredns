package reload_test

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	_ "github.com/coredns/coredns/core/plugin"
	"github.com/coredns/coredns/plugin"
)

// TestReloadWithImports tests that reload works as expected if the Corefile imports other files which might change during the reload.
// See https://github.com/coredns/coredns/issues/6243.
func TestReloadWithImports(t *testing.T) {
	const pluginName = "dummy"
	var mu sync.Mutex
	counter := 0
	maxCounter := 3
	seenCounter := -1

	tmpdir, err := os.MkdirTemp("", "")
	if err != nil {
		t.Fatalf("Could not create temporary directory: %s", err)
	}
	defer os.RemoveAll(tmpdir)

	updateImport := func() {
		mu.Lock()
		defer mu.Unlock()
		if counter < maxCounter {
			counter++
		}
		err := os.WriteFile(tmpdir+"/import.conf", []byte(fmt.Sprintf("%s %d", pluginName, counter)), 0644)
		if err != nil {
			t.Fatalf("Could not get write import file: %s", err)
		}
	}

	corefile := `.:0 {
		import *.conf
		reload 2s 1s
	}`
	err = os.WriteFile(tmpdir+"/Corefile", []byte(corefile), 0644)
	if err != nil {
		t.Fatalf("Could not write Corefile: %s", err)
	}
	caddy.RegisterCaddyfileLoader("test", caddy.LoaderFunc(func(serverType string) (caddy.Input, error) {
		return caddy.CaddyfileInput{Filepath: tmpdir + "/Corefile", Contents: []byte(corefile), ServerTypeName: "dns"}, nil
	}))
	updateImport()

	dnsserver.Directives = append([]string{pluginName}, dnsserver.Directives...)
	plugin.Register(pluginName, func(c *caddy.Controller) error {
		mu.Lock()
		defer mu.Unlock()
		i := 0
		for c.Next() {
			if i > 0 {
				return plugin.ErrOnce
			}
			i++
			args := c.RemainingArgs()
			if len(args) != 1 {
				return plugin.Error(pluginName, fmt.Errorf("wrong number of arguments: %d (expected: 1)", len(args)))
			}
			arg := args[0]
			t.Logf("setup called: %s", arg)
			argVal, err := strconv.Atoi(arg)
			if err != nil {
				return plugin.Error(pluginName, fmt.Errorf("argument must be an integer value, but got: %s", arg))
			}
			seenCounter = argVal
		}
		c.OnShutdown(func() error {
			t.Log("onShutdown called")
			updateImport()
			return nil
		})
		return nil
	})

	coreInput, err := caddy.LoadCaddyfile("dns")
	if err != nil {
		t.Fatalf("Could not load corefile: %s", err)
	}

	_, err = caddy.Start(coreInput)
	if err != nil {
		t.Fatalf("Could not start CoreDNS instance: %s", err)
	}

	updateImport()

	for i := 0; i < 20; i++ {
		if func() bool {
			mu.Lock()
			defer mu.Unlock()
			t.Logf("got: %d, want: %d", seenCounter, counter)
			return seenCounter == counter
		}() {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Errorf("Reload did not happen within 10s")
}
