package test

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredns/caddy"

	"github.com/miekg/dns"
)

func persistentDynUpdateConfig(t *testing.T, network string) (corefile, seed string) {
	t.Helper()
	dir := t.TempDir()
	seed = filepath.Join(dir, "example.org.zone")
	if err := os.WriteFile(seed, []byte(dynUpdateZone), 0600); err != nil {
		t.Fatal(err)
	}
	server, tlsLine := "example.org:0", ""
	if network == "tcp-tls" {
		server = "tls://" + server
		tlsLine = "tls ../plugin/tls/test_cert.pem ../plugin/tls/test_key.pem"
	}
	return fmt.Sprintf(`%s {
		bind 127.0.0.1
		%s
		tsig {
			secret %s %s
			require_opcode UPDATE
		}
		cache
		dynupdate {
			file "%s"
			database "%s"
			allow %s * *
		}
	}`, server, tlsLine, dynUpdateKey, dynUpdateSecret, filepath.ToSlash(seed), filepath.ToSlash(filepath.Join(dir, "updates.db")), dynUpdateKey), seed
}

func stopDynUpdateServer(t *testing.T, s *caddy.Instance) {
	t.Helper()
	if err := s.Stop(); err != nil {
		t.Errorf("stopping server: %v", err)
	}
	for _, err := range s.ShutdownCallbacks() {
		t.Errorf("shutdown callback: %v", err)
	}
}

func exchangeDynUpdate(t *testing.T, client *dns.Client, addr string, m *dns.Msg, code int) *dns.Msg {
	t.Helper()
	if m.Opcode == dns.OpcodeUpdate {
		m.SetTsig(dynUpdateKey, dns.HmacSHA256, 300, time.Now().Unix())
	}
	r, _, err := client.Exchange(m, addr)
	if err != nil || r == nil || r.Rcode != code {
		t.Fatalf("exchange: response=%v err=%v want=%s", r, err, dns.RcodeToString[code])
	}
	return r
}

func TestDynUpdatePersistentWire(t *testing.T) {
	for _, network := range []string{"udp", "tcp", "tcp-tls"} {
		t.Run(network, func(t *testing.T) {
			corefile, seed := persistentDynUpdateConfig(t, network)
			s, udp, tcp, err := CoreDNSServerAndPorts(corefile)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if s != nil {
					stopDynUpdateServer(t, s)
				}
			}()
			addr := tcp
			if network == "udp" {
				addr = udp
			}
			client := &dns.Client{
				Net: network, Timeout: 5 * time.Second,
				TsigSecret: map[string]string{dynUpdateKey: dynUpdateSecret},
				TLSConfig:  &tls.Config{InsecureSkipVerify: true}, // test certificate
			}
			query := new(dns.Msg)
			query.SetQuestion("new.example.org.", dns.TypeTXT)
			exchangeDynUpdate(t, client, addr, query, dns.RcodeNameError)
			query.SetQuestion("www.example.org.", dns.TypeA)
			exchangeDynUpdate(t, client, addr, query, dns.RcodeSuccess)
			types := []string{
				`www.example.org. 120 IN A 192.0.2.100`,
				`www.example.org. 120 IN AAAA 2001:db8::100`,
				`new.example.org. 120 IN TXT "durable"`,
				`ptr.example.org. 120 IN PTR www.example.org.`,
				`_service._tcp.example.org. 120 IN SRV 0 0 443 www.example.org.`,
				`www.example.org. 120 IN DHCID AAEAAQ==`,
				`example.org. 120 IN CAA 0 issue "ca.example"`,
			}
			records := make([]dns.RR, 0, len(types))
			for _, text := range types {
				rr, err := dns.NewRR(text)
				if err != nil {
					t.Fatal(err)
				}
				records = append(records, rr)
			}
			update := new(dns.Msg).SetUpdate("example.org.")
			update.RemoveRRset([]dns.RR{records[0]})
			update.Insert(records)
			exchangeDynUpdate(t, client, addr, update, dns.RcodeSuccess)
			for round := range 2 {
				for _, rr := range records {
					query.SetQuestion(rr.Header().Name, rr.Header().Rrtype)
					r := exchangeDynUpdate(t, client, addr, query, dns.RcodeSuccess)
					if len(r.Answer) != 1 || r.Answer[0].String() != rr.String() {
						t.Fatalf("round %d: stale or lost record: %v", round, r)
					}
				}
				query.SetQuestion("example.org.", dns.TypeSOA)
				soa := exchangeDynUpdate(t, client, addr, query, dns.RcodeSuccess)
				if len(soa.Answer) != 1 || soa.Answer[0].(*dns.SOA).Serial != 11 {
					t.Fatalf("wrong SOA serial: %v", soa)
				}
				if round == 0 {
					stopDynUpdateServer(t, s)
					s = nil
					if err := os.Remove(seed); err != nil {
						t.Fatal(err)
					}
					s, udp, tcp, err = CoreDNSServerAndPorts(corefile)
					if err != nil {
						t.Fatal(err)
					}
					addr = tcp
					if network == "udp" {
						addr = udp
					}
				}
			}
			// Delete after recovery and confirm the positive response cannot stick.
			update = new(dns.Msg).SetUpdate("example.org.")
			update.RemoveRRset([]dns.RR{records[2]})
			exchangeDynUpdate(t, client, addr, update, dns.RcodeSuccess)
			query.SetQuestion("new.example.org.", dns.TypeTXT)
			exchangeDynUpdate(t, client, addr, query, dns.RcodeNameError)
		})
	}
}

func TestDynUpdateCorefileReload(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Caddy listener file-descriptor inheritance is unavailable on Windows")
	}
	corefile, _ := persistentDynUpdateConfig(t, "udp")
	s, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { stopDynUpdateServer(t, s) }()
	client := &dns.Client{Net: "udp", TsigSecret: map[string]string{dynUpdateKey: dynUpdateSecret}}
	rr, err := dns.NewRR(`reload.example.org. 60 IN TXT "survives"`)
	if err != nil {
		t.Fatal(err)
	}
	update := new(dns.Msg).SetUpdate("example.org.")
	update.Insert([]dns.RR{rr})
	exchangeDynUpdate(t, client, udp, update, dns.RcodeSuccess)
	// file is initialized after dynupdate. Its error must leave the old server
	// and database usable, without retaining a ref for the abandoned config.
	bad := strings.Replace(corefile, "\n\t\tcache", "\n\t\tfile does-not-exist.example\n\t\tcache", 1)
	if _, err := s.Restart(NewInput(bad)); err == nil {
		t.Fatal("invalid reload unexpectedly succeeded")
	}
	query := new(dns.Msg)
	query.SetQuestion(rr.Header().Name, dns.TypeTXT)
	exchangeDynUpdate(t, client, udp, query, dns.RcodeSuccess)
	next, err := s.Restart(NewInput(corefile))
	if err != nil {
		t.Fatal(err)
	}
	s = next
	udp, _ = CoreDNSServerPorts(s, 0)
	r := exchangeDynUpdate(t, client, udp, query, dns.RcodeSuccess)
	if len(r.Answer) != 1 || r.Answer[0].String() != rr.String() {
		t.Fatalf("reload lost acknowledged update: %v", r)
	}
	update = new(dns.Msg).SetUpdate("example.org.")
	update.RemoveRRset([]dns.RR{rr})
	exchangeDynUpdate(t, client, udp, update, dns.RcodeSuccess)
	exchangeDynUpdate(t, client, udp, query, dns.RcodeNameError)
}

func TestDynUpdateNsupdate(t *testing.T) {
	nsupdate, err := exec.LookPath("nsupdate")
	if err != nil {
		t.Skip("BIND nsupdate is not installed")
	}
	corefile, _ := persistentDynUpdateConfig(t, "udp")
	s, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatal(err)
	}
	defer stopDynUpdateServer(t, s)
	host, port, err := net.SplitHostPort(udp)
	if err != nil {
		t.Fatal(err)
	}
	for _, tcp := range []bool{false, true} {
		args := []string{"-y", "hmac-sha256:" + dynUpdateKey + ":" + dynUpdateSecret}
		if tcp {
			args = append(args, "-v")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, nsupdate, args...)
		cmd.Stdin = strings.NewReader(fmt.Sprintf(`server %s %s
zone example.org.
prereq nxdomain nsupdate.example.org.
update add nsupdate.example.org. 60 TXT "interop"
send
prereq yxrrset nsupdate.example.org. TXT "interop"
update delete nsupdate.example.org. TXT
send
`, host, port))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("nsupdate tcp=%v: %v\n%s", tcp, err, out)
		}
	}
}

func TestDynUpdateCoalescesNotify(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	blocked := make(chan struct{})
	release := make(chan struct{})
	var unblock sync.Once
	defer unblock.Do(func() { close(release) })
	notifications := make(chan struct{}, 2)
	var calls atomic.Int32
	secondary := &dns.Server{PacketConn: pc, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		if r.Opcode != dns.OpcodeNotify || len(r.Question) != 1 || r.Question[0].Name != "example.org." {
			t.Errorf("unexpected notification: %v", r)
		}
		if calls.Add(1) == 1 {
			close(blocked)
			<-release
		}
		if err := w.WriteMsg(new(dns.Msg).SetReply(r)); err != nil {
			t.Errorf("replying to NOTIFY: %v", err)
		}
		select {
		case notifications <- struct{}{}:
		default:
		}
	})}
	stopped := make(chan error, 1)
	go func() { stopped <- secondary.ActivateAndServe() }()
	defer func() {
		unblock.Do(func() { close(release) })
		secondary.Shutdown()
		if err := <-stopped; err != nil {
			t.Errorf("notification server: %v", err)
		}
	}()
	corefile, _ := persistentDynUpdateConfig(t, "udp")
	corefile = strings.Replace(corefile, "\n\t\tcache", fmt.Sprintf("\n\t\ttransfer {\n\t\t\tto %s\n\t\t}\n\t\tcache", pc.LocalAddr()), 1)
	s, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatal(err)
	}
	defer stopDynUpdateServer(t, s)
	client := &dns.Client{Net: "udp", TsigSecret: map[string]string{dynUpdateKey: dynUpdateSecret}}
	for i := range 4 {
		rr, err := dns.NewRR(fmt.Sprintf("notify-%d.example.org. 60 IN A 192.0.2.%d", i, i+1))
		if err != nil {
			t.Fatal(err)
		}
		update := new(dns.Msg).SetUpdate("example.org.")
		update.Insert([]dns.RR{rr})
		exchangeDynUpdate(t, client, udp, update, dns.RcodeSuccess)
		if i == 0 {
			select {
			case <-blocked:
			case <-time.After(5 * time.Second):
				t.Fatal("committed update did not trigger NOTIFY")
			}
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("concurrent NOTIFY operations: %d", got)
	}
	unblock.Do(func() { close(release) })
	for range 2 {
		select {
		case <-notifications:
		case <-time.After(5 * time.Second):
			t.Fatal("pending notification was lost")
		}
	}
}
