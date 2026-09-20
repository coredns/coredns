package test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// Exercise Kea's actual RFC 4703 state machine. Only the lease-change
// notifications are synthetic; Kea constructs and signs every DNS UPDATE.
func TestDynUpdateKeaLifecycle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Kea DHCP-DDNS is not available on Windows")
	}
	kea, err := exec.LookPath("kea-dhcp-ddns")
	if err != nil {
		t.Skip("Kea DHCP-DDNS is not installed")
	}
	version, err := exec.Command(kea, "-v").CombinedOutput()
	if err != nil {
		t.Fatalf("Kea version: %v: %s", err, version)
	}
	t.Logf("Kea DHCP-DDNS %s", strings.TrimSpace(string(version)))

	for _, tc := range []struct {
		name, address, conflictAddress, reverse string
		rrType                                  uint16
	}{
		{"v4", "192.0.2.10", "192.0.2.11", "2.0.192.in-addr.arpa.", dns.TypeA},
		{"v6", "2001:db8::10", "2001:db8::11", "8.b.d.0.1.0.0.2.ip6.arpa.", dns.TypeAAAA},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			var corefile strings.Builder
			for i, zone := range []string{"example.org.", tc.reverse} {
				seed := filepath.Join(dir, fmt.Sprintf("zone-%d", i))
				data := fmt.Sprintf("%s 60 IN SOA ns.example.org. hostmaster.example.org. 10 60 60 60 60\n%s 60 IN NS ns.example.org.\n", zone, zone)
				if err := os.WriteFile(seed, []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
				grants := "A AAAA DHCID ANY"
				if i == 1 {
					grants = "PTR DHCID ANY"
				}
				fmt.Fprintf(&corefile, `%s:0 {
	bind 127.0.0.1
	cache
	tsig {
		secret %s %s
		require_opcode UPDATE
	}
	dynupdate {
		file %q
		database %q
		allow %s * %s
	}
}
`, zone, dynUpdateKey, dynUpdateSecret, seed, seed+".db", dynUpdateKey, grants)
			}
			s, addr, _, err := CoreDNSServerAndPorts(corefile.String())
			if err != nil {
				t.Fatal(err)
			}
			defer stopDynUpdateServer(t, s)
			send := startKeaDynUpdate(t, kea, addr, tc.reverse)
			client := &dns.Client{Net: "udp", Timeout: time.Second}
			owner := tc.name + ".example.org."
			ptr, err := dns.ReverseAddr(tc.address)
			if err != nil {
				t.Fatal(err)
			}
			conflictPTR, err := dns.ReverseAddr(tc.conflictAddress)
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256([]byte("client-" + tc.name))
			dhcid := append([]byte{0, 1, 1}, digest[:]...)
			check := func(name string, rrType uint16, ttl uint32, value string) {
				t.Helper()
				query := new(dns.Msg).SetQuestion(name, rrType)
				r := exchangeDynUpdate(t, client, addr, query, dns.RcodeSuccess)
				want, err := dns.NewRR(fmt.Sprintf("%s %d IN %s %s", name, ttl, dns.TypeToString[rrType], value))
				if err != nil {
					t.Fatal(err)
				}
				if ttl == 0 && len(r.Answer) == 1 {
					want.Header().Ttl = r.Answer[0].Header().Ttl
				}
				if !r.Authoritative || len(r.Answer) != 1 || r.Answer[0].String() != want.String() {
					t.Fatalf("expected %s, got %v", want, r)
				}
			}
			checkLease := func(ttl uint32) {
				t.Helper()
				check(owner, tc.rrType, ttl, tc.address)
				// Renewals replace addresses, but need not replace the ownership RR.
				check(owner, dns.TypeDHCID, 0, base64.StdEncoding.EncodeToString(dhcid))
				check(ptr, dns.TypePTR, ttl, owner)
			}
			serials := func() []uint32 {
				t.Helper()
				var result []uint32
				for _, zone := range []string{"example.org.", tc.reverse} {
					r := exchangeDynUpdate(t, client, addr, new(dns.Msg).SetQuestion(zone, dns.TypeSOA), dns.RcodeSuccess)
					if len(r.Answer) != 1 {
						t.Fatalf("missing SOA: %v", r)
					}
					result = append(result, r.Answer[0].(*dns.SOA).Serial)
				}
				return result
			}

			// Populate negative cache entries before claiming the name.
			for _, name := range []string{owner, ptr} {
				exchangeDynUpdate(t, client, addr, new(dns.Msg).SetQuestion(name, dns.TypeANY), dns.RcodeNameError)
			}
			send(0, owner, tc.address, dhcid, 60, "DHCP_DDNS_ADD_SUCCEEDED")
			checkLease(60)
			send(0, owner, tc.address, dhcid, 120, "DHCP_DDNS_ADD_SUCCEEDED")
			checkLease(120)

			before := serials()
			other := append([]byte(nil), dhcid...)
			other[len(other)-1] ^= 1
			send(0, owner, tc.conflictAddress, other, 60, "DHCP_DDNS_ADD_FAILED")
			checkLease(120)
			exchangeDynUpdate(t, client, addr, new(dns.Msg).SetQuestion(conflictPTR, dns.TypePTR), dns.RcodeNameError)
			after := serials()
			if before[0] != after[0] || before[1] != after[1] {
				t.Fatalf("conflicting client changed zone serials: %v -> %v", before, after)
			}

			send(1, owner, tc.address, dhcid, 120, "DHCP_DDNS_REMOVE_SUCCEEDED")
			for _, name := range []string{owner, ptr} {
				exchangeDynUpdate(t, client, addr, new(dns.Msg).SetQuestion(name, dns.TypeANY), dns.RcodeNameError)
			}
			// A different client may claim the name only after the old owner releases it.
			send(0, owner, tc.conflictAddress, other, 60, "DHCP_DDNS_ADD_SUCCEEDED")
			check(owner, tc.rrType, 60, tc.conflictAddress)
			check(owner, dns.TypeDHCID, 60, base64.StdEncoding.EncodeToString(other))
			check(conflictPTR, dns.TypePTR, 60, owner)
			send(1, owner, tc.conflictAddress, other, 60, "DHCP_DDNS_REMOVE_SUCCEEDED")
		})
	}
}

func startKeaDynUpdate(t *testing.T, executable, dnsAddr, reverseZone string) func(int, string, string, []byte, uint32, string) {
	t.Helper()
	dir := t.TempDir()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ncrAddr := pc.LocalAddr().String()
	ncrPort := pc.LocalAddr().(*net.UDPAddr).Port
	if err := pc.Close(); err != nil {
		t.Fatal(err)
	}
	host, port, err := net.SplitHostPort(dnsAddr)
	if err != nil {
		t.Fatal(err)
	}
	dnsPort, err := strconv.Atoi(port)
	if err != nil {
		t.Fatal(err)
	}
	domain := func(zone string) map[string]any {
		return map[string]any{"ddns-domains": []any{map[string]any{
			"name": zone, "key-name": dynUpdateKey,
			"dns-servers": []any{map[string]any{"ip-address": host, "port": dnsPort}},
		}}}
	}
	config := map[string]any{"DhcpDdns": map[string]any{
		"ip-address": "127.0.0.1", "port": ncrPort,
		"dns-server-timeout": 2000, "ncr-protocol": "UDP", "ncr-format": "JSON",
		"tsig-keys":    []any{map[string]any{"name": dynUpdateKey, "algorithm": "HMAC-SHA256", "secret": dynUpdateSecret}},
		"forward-ddns": domain("example.org."), "reverse-ddns": domain(reverseZone),
		"loggers": []any{map[string]any{
			"name": "kea-dhcp-ddns", "severity": "DEBUG", "debuglevel": 99,
			"output_options": []any{map[string]any{"output": "stdout", "flush": true}},
		}},
	}}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	configDir := dir
	if root := os.Getenv("COREDNS_KEA_CONFIG_DIR"); root != "" {
		configDir, err = os.MkdirTemp(root, "coredns-ddns-") //nolint:usetesting // Must use a path permitted by the distribution's AppArmor profile.
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.RemoveAll(configDir); err != nil {
				t.Error(err)
			}
		})
	}
	// Distribution AppArmor profiles may require this config basename and
	// fixed runtime directories. CI provisions them without changing the profile.
	path := filepath.Join(configDir, "kea-dhcp-ddns.conf")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	logfile, err := os.Create(filepath.Join(dir, "kea.log"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	cmd := exec.CommandContext(ctx, executable, "-c", path)
	cmd.Env = os.Environ()
	for _, name := range []string{"KEA_PIDFILE_DIR", "KEA_LOCKFILE_DIR"} {
		if os.Getenv(name) == "" {
			cmd.Env = append(cmd.Env, name+"="+dir)
		}
	}
	cmd.Stdout, cmd.Stderr = logfile, logfile
	if err := cmd.Start(); err != nil {
		cancel()
		logfile.Close()
		t.Fatal(err)
	}
	done := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = cmd.Wait()
		close(done)
	}()
	t.Cleanup(func() {
		// Graceful shutdown removes the PID file before the next lifecycle case.
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			cancel()
			<-done
		}
		cancel()
		logfile.Close()
		if t.Failed() {
			out, _ := os.ReadFile(logfile.Name())
			t.Logf("Kea log:\n%s", out)
		}
	})
	waitFor := func(offset int64, event string) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			out, err := os.ReadFile(logfile.Name())
			if err != nil {
				t.Fatal(err)
			}
			if int64(len(out)) >= offset {
				recent := string(out[offset:])
				if strings.Contains(recent, event) {
					return
				}
				for _, terminal := range []string{"DHCP_DDNS_ADD_SUCCEEDED", "DHCP_DDNS_ADD_FAILED", "DHCP_DDNS_REMOVE_SUCCEEDED", "DHCP_DDNS_REMOVE_FAILED"} {
					if strings.Contains(recent, terminal) {
						t.Fatalf("Kea reported %s while waiting for %s", terminal, event)
					}
				}
			}
			select {
			case <-done:
				t.Fatalf("Kea exited before %s: %v", event, waitErr)
			default:
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("Kea did not report %s", event)
	}
	waitFor(0, "DHCP_DDNS_QUEUE_MGR_STARTED")
	return func(change int, fqdn, address string, dhcid []byte, ttl uint32, event string) {
		t.Helper()
		info, err := logfile.Stat()
		if err != nil {
			t.Fatal(err)
		}
		ncr, err := json.Marshal(map[string]any{
			"change-type": change, "forward-change": true, "reverse-change": true,
			"fqdn": fqdn, "ip-address": address, "dhcid": hex.EncodeToString(dhcid),
			"lease-expires-on": time.Now().UTC().Add(time.Duration(ttl) * time.Second).Format("20060102150405"),
			"lease-length":     ttl, "use-conflict-resolution": true,
		})
		if err != nil {
			t.Fatal(err)
		}
		// Kea's UDP NCR framing is a network-order uint16 length followed by JSON.
		wire := make([]byte, 2+len(ncr))
		binary.BigEndian.PutUint16(wire, uint16(len(ncr)))
		copy(wire[2:], ncr)
		conn, err := net.DialTimeout("udp", ncrAddr, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		if _, err := conn.Write(wire); err != nil {
			t.Fatal(err)
		}
		waitFor(info.Size(), event)
	}
}
