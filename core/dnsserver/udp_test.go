//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris || windows

package dnsserver

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"net"
	"runtime"
	"slices"
	"strconv"
	"testing"
	"time"

	cproxyproto "github.com/coredns/coredns/plugin/pkg/proxyproto"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	"github.com/pires/go-proxyproto"
)

func udpTestConn(t *testing.T, network, address string) *net.UDPConn {
	t.Helper()
	addr, err := net.ResolveUDPAddr(network, address)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.ListenUDP(network, addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	return conn
}

// Two requests from the same peer must keep independent destination addresses,
// even when their replies are sent in the opposite order.
func TestUDPDestinationReadWrite(t *testing.T) {
	for _, tc := range []struct {
		name, network, bind string
		destinations        []string
	}{
		{"ipv4", "udp4", "0.0.0.0:0", []string{"127.0.0.1", "127.0.0.2"}},
		{"ipv6", "udp6", "[::]:0", []string{"::1", "::1"}},
		{"dual_stack", "udp", "[::]:0", []string{"127.0.0.1", "::1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "dual_stack" && (runtime.GOOS == "openbsd" || runtime.GOOS == "dragonfly") {
				t.Skip("the kernel requires IPv6-only sockets")
			}
			if tc.network != "udp4" {
				probe, err := net.ListenPacket("udp6", "[::1]:0")
				if err != nil {
					t.Skipf("IPv6 loopback is unavailable: %v", err)
				}
				probe.Close()
			}
			if tc.name == "ipv4" {
				switch runtime.GOOS {
				case "windows":
					// Winsock cannot select an unassigned 127.0.0.2 as the
					// source. Use a second configured local address instead.
					tc.destinations[1] = secondLocalIPv4(t)
				case "aix", "dragonfly", "freebsd", "netbsd", "openbsd":
					// x/net can receive IPv4 destinations but cannot marshal
					// IPv4 source control on these systems. Preserve their
					// existing routing behavior with one destination here.
					tc.destinations[1] = tc.destinations[0]
				default:
					probe, err := net.ListenPacket("udp4", "127.0.0.2:0")
					if err != nil {
						t.Skipf("second loopback address is unavailable: %v", err)
					}
					probe.Close()
				}
			}
			raw := udpTestConn(t, tc.network, tc.bind)
			conn := withUDPDestination(raw)
			if conn == raw {
				t.Fatal("wildcard listener did not enable packet information")
			}
			port := raw.LocalAddr().(*net.UDPAddr).Port
			peer := udpTestConn(t, tc.network, tc.bind)
			var addresses []net.Addr
			for i, dst := range tc.destinations {
				payload := []byte{byte(i)}
				if _, err := peer.WriteToUDP(payload, &net.UDPAddr{IP: net.ParseIP(dst), Port: port}); err != nil {
					t.Fatal(err)
				}
				buf := make([]byte, 16)
				n, addr, err := conn.ReadFrom(buf)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(buf[:n], payload) {
					t.Fatalf("received %v, want %v", buf[:n], payload)
				}
				addresses = append(addresses, addr)
			}
			for i, addr := range slices.Backward(addresses) {
				packet, ok := addr.(interface{ LocalAddr() net.Addr })
				if !ok {
					t.Fatalf("missing destination on %T", addr)
				}
				want := net.JoinHostPort(tc.destinations[i], strconv.Itoa(port))
				if got := packet.LocalAddr().String(); got != want {
					t.Errorf("destination = %s, want %s", got, want)
				}
				if n, err := conn.WriteTo([]byte{byte(i)}, addr); err != nil || n != 1 {
					t.Fatalf("WriteTo = %d, %v, want 1 byte", n, err)
				}
				buf := make([]byte, 16)
				n, source, err := peer.ReadFromUDP(buf)
				if err != nil {
					t.Fatal(err)
				}
				if n != 1 || buf[0] != byte(i) || source.String() != want {
					t.Fatalf("reply = %v from %s, want [%d] from %s", buf[:n], source, i, want)
				}
			}
		})
	}
}

func secondLocalIPv4(t *testing.T) string {
	t.Helper()
	interfaces, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			t.Fatal(err)
		}
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err == nil && ip.To4() != nil && ip.IsPrivate() && !ip.IsLoopback() {
				return ip.String()
			}
		}
	}
	t.Skip("a second configured local IPv4 address is unavailable")
	return ""
}

func TestUDPDestinationBoundSocket(t *testing.T) {
	conn := udpTestConn(t, "udp4", "127.0.0.1:0")
	if withUDPDestination(conn) != conn {
		t.Fatal("explicitly bound listener should keep its existing UDP path")
	}
	addr := conn.LocalAddr().String()
	peer := udpTestConn(t, "udp4", "127.0.0.1:0")
	if _, err := peer.WriteToUDP([]byte("bound"), conn.LocalAddr().(*net.UDPAddr)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 16)
	n, from, err := conn.ReadFrom(buf)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.WriteTo(buf[:n], from); err != nil {
		t.Fatal(err)
	}
	if _, _, err := peer.ReadFrom(buf); err != nil {
		t.Fatal(err)
	}
	if got := conn.LocalAddr().String(); got != addr {
		t.Fatalf("socket binding changed after reply: %s -> %s", addr, got)
	}
}

func TestUDPDestinationDeadlineAndClose(t *testing.T) {
	raw := udpTestConn(t, "udp4", "0.0.0.0:0")
	conn := withUDPDestination(raw)
	if err := conn.SetReadDeadline(time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	_, _, err := conn.ReadFrom(make([]byte, 16))
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("expired deadline: %v", err)
	}
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, _, err := conn.ReadFrom(make([]byte, 16))
		done <- err
	}()
	conn.Close()
	select {
	case err := <-done:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatalf("closed socket: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("closing the socket did not unblock ReadFrom")
	}
}

func TestUDPDestinationMissingPacketInfo(t *testing.T) {
	raw := udpTestConn(t, "udp4", "0.0.0.0:0")
	// Deliberately leave packet-info socket options disabled.
	conn := &udpDestinationConn{UDPConn: raw, local: *raw.LocalAddr().(*net.UDPAddr)}
	peer := udpTestConn(t, "udp4", "127.0.0.1:0")
	dst := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: conn.local.Port}
	if _, err := peer.WriteToUDP([]byte("fallback"), dst); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 16)
	n, addr, err := conn.ReadFrom(buf)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := addr.(*net.UDPAddr); !ok {
		t.Fatalf("missing metadata changed the peer address: %T", addr)
	}
	if conn.LocalAddr().String() != raw.LocalAddr().String() {
		t.Fatal("missing metadata changed the listener address")
	}
	if count, err := conn.WriteTo(buf[:n], addr); err != nil || count != n {
		t.Fatalf("fallback reply: %d, %v", count, err)
	}
	if _, _, err := peer.ReadFrom(buf); err != nil {
		t.Fatal(err)
	}
}

type udpAddressPlugin struct{}

func (udpAddressPlugin) Name() string { return "udp-address" }
func (udpAddressPlugin) ServeDNS(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	rawTLVs := ""
	if peer, ok := w.RemoteAddr().(*cproxyproto.Addr); ok {
		rawTLVs = hex.EncodeToString(peer.RawTLVs())
	}
	m := new(dns.Msg).SetReply(r)
	m.Answer = []dns.RR{&dns.TXT{
		Hdr: dns.RR_Header{Name: r.Question[0].Name, Rrtype: dns.TypeTXT, Class: dns.ClassINET},
		Txt: []string{state.LocalIP(), state.LocalPort(), state.IP(), state.Proto(), strconv.Itoa(state.Family()), rawTLVs},
	}}
	return dns.RcodeSuccess, w.WriteMsg(m)
}

func TestUDPDestinationServer(t *testing.T) {
	for _, mode := range []string{"plain", "proxy", "proxy_local", "proxy_skip"} {
		t.Run(mode, func(t *testing.T) {
			raw := udpTestConn(t, "udp4", "0.0.0.0:0")
			port := raw.LocalAddr().(*net.UDPAddr).Port
			var pc net.PacketConn = raw
			if mode != "plain" {
				pc = &cproxyproto.PacketConn{PacketConn: raw, CaptureTLVs: true, UDPSessionTrackingTTL: time.Minute,
					ConnPolicy: func(options proxyproto.ConnPolicyOptions) (proxyproto.Policy, error) {
						if _, ok := options.Upstream.(*net.UDPAddr); !ok {
							t.Errorf("policy peer has type %T, want *net.UDPAddr", options.Upstream)
						}
						if options.Downstream.String() != raw.LocalAddr().String() {
							t.Errorf("policy listener = %s, want %s", options.Downstream, raw.LocalAddr())
						}
						if mode == "proxy_skip" {
							return proxyproto.SKIP, nil
						}
						return proxyproto.USE, nil
					},
				}
			}
			ready := make(chan struct{})
			cfg := testConfig("dns", udpAddressPlugin{})
			cfg.UDPDecorateWriterFunc = func(*Server) dns.DecorateWriter { close(ready); return nil }
			srv, err := NewServer("0.0.0.0:0", []*Config{cfg})
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- srv.ServePacket(pc) }()
			select {
			case <-ready:
			case <-time.After(5 * time.Second):
				t.Fatal("UDP adapter did not start")
			}
			t.Cleanup(func() {
				if err := srv.Stop(); err != nil {
					t.Error(err)
				}
				select {
				case err := <-done:
					if err != nil {
						t.Error(err)
					}
				case <-time.After(5 * time.Second):
					t.Error("UDP server did not stop")
				}
			})
			conn, err := net.Dial("udp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			conn.SetDeadline(time.Now().Add(5 * time.Second))
			wantRemote, wantTLVs := "127.0.0.1", ""
			if mode == "proxy" {
				wantRemote, wantTLVs = "192.0.2.10", "ea00020102"
			}
			for i := range 2 {
				m := new(dns.Msg).SetQuestion("example.com.", dns.TypeTXT)
				packet, err := m.Pack()
				if err != nil {
					t.Fatal(err)
				}
				if i == 0 && (mode == "proxy" || mode == "proxy_local") {
					header := &proxyproto.Header{Version: 2, Command: proxyproto.PROXY, TransportProtocol: proxyproto.UDPv4,
						SourceAddr:      &net.UDPAddr{IP: net.ParseIP("192.0.2.10"), Port: 1234},
						DestinationAddr: &net.UDPAddr{IP: net.ParseIP("198.51.100.53"), Port: 53}}
					if mode == "proxy_local" {
						header.Command = proxyproto.LOCAL
					}
					if err := header.SetTLVs([]proxyproto.TLV{{Type: 0xEA, Value: []byte{1, 2}}}); err != nil {
						t.Fatal(err)
					}
					prefix, err := header.Format()
					if err != nil {
						t.Fatal(err)
					}
					packet = append(prefix, packet...)
				}
				if _, err := conn.Write(packet); err != nil {
					t.Fatal(err)
				}
				answer, err := (&dns.Conn{Conn: conn}).ReadMsg()
				if err != nil {
					t.Fatal(err)
				}
				if answer.Rcode != dns.RcodeSuccess || len(answer.Answer) != 1 {
					t.Fatalf("unexpected response: %v", answer)
				}
				want := []string{"127.0.0.1", strconv.Itoa(port), wantRemote, "udp", "1", wantTLVs}
				got := answer.Answer[0].(*dns.TXT).Txt
				for j := range want {
					if got[j] != want[j] {
						t.Errorf("field %d = %q, want %q", j, got[j], want[j])
					}
				}
			}
		})
	}
}

func FuzzUDPPacketInfo(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0xff, 0xff, 0xff, 0xff})
	f.Add(udpSourceControl(udpPacketInfo{dst: net.ParseIP("127.0.0.1"), ifIndex: 1}))
	f.Add(udpSourceControl(udpPacketInfo{dst: net.ParseIP("::1"), ifIndex: 1}))
	f.Fuzz(func(t *testing.T, data []byte) {
		info := parseUDPPacketInfo(data, 0)
		if info.dst != nil && info.dst.To16() == nil {
			t.Fatalf("invalid destination: %v", info.dst)
		}
		udpSourceControl(info)
	})
}
