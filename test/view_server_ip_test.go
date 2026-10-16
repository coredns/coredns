//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris || windows

package test

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	clog "github.com/coredns/coredns/plugin/pkg/log"

	"github.com/miekg/dns"
)

func TestViewServerIP(t *testing.T) {
	for _, tc := range []struct{ name, bind, target string }{
		{"wildcard4", "0.0.0.0", "127.0.0.1"},
		{"wildcard6", "::", "::1"},
		{"dual_stack", "::", "127.0.0.1"},
		{"bound4", "127.0.0.1", "127.0.0.1"},
		{"bound6", "::1", "::1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "dual_stack" && (runtime.GOOS == "openbsd" || runtime.GOOS == "dragonfly") {
				t.Skip("the kernel requires IPv6-only sockets")
			}
			if net.ParseIP(tc.bind).To4() == nil {
				checkIPv6Loopback(t)
			}
			corefile := fmt.Sprintf(`.:0 {
    bind %s
    view destination {
        expr server_ip() == '%s' && name() == 'destination.example.'
    }
    template IN TXT . {
        answer "{{ .Name }} 60 IN TXT \"matched\""
    }
}`, tc.bind, tc.target)
			instance, udp, tcp, err := CoreDNSServerAndPorts(corefile)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { instance.Stop() })
			for _, network := range []string{"udp", "tcp"} {
				t.Run(network, func(t *testing.T) {
					listener := udp
					if network == "tcp" {
						listener = tcp
					}
					_, port, err := net.SplitHostPort(listener)
					if err != nil {
						t.Fatal(err)
					}
					client := &dns.Client{Net: network, Timeout: 2 * time.Second}
					conn, err := client.Dial(net.JoinHostPort(tc.target, port))
					if err != nil {
						t.Fatal(err)
					}
					defer conn.Close()
					if network == "udp" {
						waitForViewUDP(t, net.JoinHostPort(tc.target, port))
					}
					for range 2 {
						for _, name := range []string{"destination.example.", "unmatched.example."} {
							answer, _, err := client.ExchangeWithConn(new(dns.Msg).SetQuestion(name, dns.TypeTXT), conn)
							if err != nil {
								t.Fatal(err)
							}
							want := dns.RcodeRefused
							if name == "destination.example." {
								want = dns.RcodeSuccess
							}
							if answer.Rcode != want {
								t.Errorf("query %s to %s: rcode=%s, want %s", name, tc.target, dns.RcodeToString[answer.Rcode], dns.RcodeToString[want])
							}
							if want == dns.RcodeSuccess && (len(answer.Answer) != 1 || answer.Answer[0].String() != name+"\t60\tIN\tTXT\t\"matched\"") {
								t.Errorf("unexpected answer: %v", answer.Answer)
							}
						}
					}
				})
			}
		})
	}
}

func checkIPv6Loopback(t *testing.T) {
	t.Helper()
	conn, err := net.ListenPacket("udp6", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 loopback is unavailable: %v", err)
	}
	conn.Close()
}

// Caddy returns before the serving goroutines start. Complete one exchange so
// assertions do not depend on packet information for a datagram queued before
// ServePacket enabled the socket options. The view may refuse this probe.
func waitForViewUDP(t *testing.T, address string) {
	t.Helper()
	client := &dns.Client{Net: "udp", Timeout: 2 * time.Second}
	if _, _, err := client.Exchange(new(dns.Msg).SetQuestion("ready.invalid.", dns.TypeTXT), address); err != nil {
		t.Fatal(err)
	}
}

func TestViewServerIPConcurrent(t *testing.T) {
	if runtime.GOOS == "openbsd" || runtime.GOOS == "dragonfly" {
		t.Skip("the kernel requires IPv6-only sockets")
	}
	checkIPv6Loopback(t)
	instance, udp, _, err := CoreDNSServerAndPorts(`.:0 {
    bind ::
    view destination {
        expr (server_ip() == '127.0.0.1' && name() == 'ipv4.example.') || (server_ip() == '::1' && name() == 'ipv6.example.')
    }
    template IN TXT . {
        answer "{{ .Name }} 60 IN TXT \"matched\""
    }
}`)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { instance.Stop() })
	_, port, err := net.SplitHostPort(udp)
	if err != nil {
		t.Fatal(err)
	}
	waitForViewUDP(t, net.JoinHostPort("127.0.0.1", port))
	for _, tc := range []struct{ target, name string }{
		{"127.0.0.1", "ipv4.example."},
		{"::1", "ipv6.example."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client := &dns.Client{Net: "udp", Timeout: 2 * time.Second}
			conn, err := client.Dial(net.JoinHostPort(tc.target, port))
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			for range 50 {
				answer, _, err := client.ExchangeWithConn(new(dns.Msg).SetQuestion(tc.name, dns.TypeTXT), conn)
				if err != nil {
					t.Fatal(err)
				}
				if answer.Rcode != dns.RcodeSuccess || len(answer.Answer) != 1 {
					t.Fatalf("view for %s did not match: %v", tc.target, answer)
				}
			}
		})
	}
}

// Isolate request logging and Caddy restart callbacks from other integration
// tests. The child exercises the actual log plugin and its {local} formatter.
func TestViewServerIPLogAndReload(t *testing.T) {
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-test.run=^TestViewServerIPLogHelper$", "-test.v")
	cmd.Env = append(os.Environ(), "COREDNS_TEST_UDP_LOCAL=1", "GORACE=atexit_sleep_ms=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("log/reload process: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "LOCAL=127.0.0.1") || strings.Contains(string(output), "LOCAL=0.0.0.0") {
		t.Fatalf("incorrect request log destination:\n%s", output)
	}
	if runtime.GOOS != "windows" && !strings.Contains(string(output), "RELOADED=127.0.0.1") {
		t.Fatalf("no destination log after listener reuse:\n%s", output)
	}
}

func TestViewServerIPLogHelper(t *testing.T) {
	if os.Getenv("COREDNS_TEST_UDP_LOCAL") != "1" {
		return
	}
	clog.SetOutput(os.Stdout)
	corefile := `.:0 {
    bind 0.0.0.0
    view destination {
        expr server_ip() == '127.0.0.1'
    }
    log . "LOCAL={local}"
    template IN TXT . {
        answer "{{ .Name }} 60 IN TXT \"matched\""
    }
}`
	instance, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { instance.Stop() }()
	_, port, err := net.SplitHostPort(udp)
	if err != nil {
		t.Fatal(err)
	}
	waitForViewUDP(t, net.JoinHostPort("127.0.0.1", port))
	query := func() {
		t.Helper()
		client := &dns.Client{Net: "udp", Timeout: 2 * time.Second}
		answer, _, err := client.Exchange(new(dns.Msg).SetQuestion("destination.example.", dns.TypeTXT), net.JoinHostPort("127.0.0.1", port))
		if err != nil {
			t.Fatal(err)
		}
		if answer.Rcode != dns.RcodeSuccess || len(answer.Answer) != 1 {
			t.Fatalf("destination view did not match: %v", answer)
		}
	}
	query()
	// Caddy cannot inherit Windows socket file descriptors on restart.
	if runtime.GOOS == "windows" {
		return
	}
	// Retain the configured :0 address so Caddy finds the old listener by
	// its configuration key. A fresh listen would choose a different port.
	corefile = strings.Replace(corefile, "LOCAL=", "RELOADED=", 1)
	next, err := instance.Restart(NewInput(corefile))
	if err != nil {
		t.Fatal(err)
	}
	instance = next
	if got, _ := CoreDNSServerPorts(instance, 0); got != udp {
		t.Fatalf("listener was not reused: %s -> %s", udp, got)
	}
	query()
}
