package forward

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/google/gopacket"
	"github.com/google/gopacket/pcap"
	"github.com/miekg/dns"
)

func TestProxy(t *testing.T) {
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		ret.Answer = append(ret.Answer, test.A("example.org. IN A 127.0.0.1"))
		w.WriteMsg(ret)
	})
	defer s.Close()

	c := caddy.NewTestController("dns", "forward . "+s.Addr)
	fs, err := parseForward(c)
	f := fs[0]
	if err != nil {
		t.Errorf("Failed to create forwarder: %s", err)
	}
	f.OnStartup()
	defer f.OnShutdown()

	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeA)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	if _, err := f.ServeDNS(context.TODO(), rec, m); err != nil {
		t.Fatal("Expected to receive reply, but didn't")
	}
	if x := rec.Msg.Answer[0].Header().Name; x != "example.org." {
		t.Errorf("Expected %s, got %s", "example.org.", x)
	}
}

func TestProxyTLSFail(t *testing.T) {
	// This is an udp/tcp test server, so we shouldn't reach it with TLS.
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		ret.Answer = append(ret.Answer, test.A("example.org. IN A 127.0.0.1"))
		w.WriteMsg(ret)
	})
	defer s.Close()

	c := caddy.NewTestController("dns", "forward . tls://"+s.Addr)
	fs, err := parseForward(c)
	f := fs[0]
	if err != nil {
		t.Errorf("Failed to create forwarder: %s", err)
	}
	f.OnStartup()
	defer f.OnShutdown()

	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeA)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	if _, err := f.ServeDNS(context.TODO(), rec, m); err == nil {
		t.Fatal("Expected *not* to receive reply, but got one")
	}
}

func TestMalformedSpoof1(t *testing.T) {
	// Stucture:
	// [dnstest client] <-> [forward plugin controller] <-> [crude udp gateway] <-> [upstream dns]

	// var clientWrite sync.Mutex
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		ret.Answer = append(ret.Answer, test.A("example.org. IN A 1.2.3.4"))
		w.WriteMsg(ret)
	})
	defer s.Close()
	// create net.Addr object from s.Addr string
	sAddr, _ := net.ResolveUDPAddr("udp", s.Addr)
	println("upstream dns listening on " + sAddr.String())

	// set up crude udp packet gateway to be a man-in-the-middle
	l, err := net.ListenPacket("udp", ":0")
	if err != nil {
		panic(err)
	}
	println("udp packet gateway listening on " + l.LocalAddr().String())
	defer l.Close()
	// start the udp packet gateway read loop
	go func() {
		p := make([]byte, 64)
		var fwdAddr net.Addr
		for {
			_, cAddr, err := l.ReadFrom(p)
			if err != nil {
				return
			}
			fmt.Printf("data from %v: %v\n", cAddr.String(), p)
			// if the source is the server, send spoofed response
			if cAddr.String() == s.Addr {
				// send malformed spoofed response to cAddr.String(), from s.Addr
				fmt.Printf("writing data from server to %v: %v\n", cAddr.String(), p)
				// then send good response
				l.WriteTo(p, fwdAddr)
			} else {
				fmt.Printf("writing data from client to server %v: %v\n", sAddr, p)
				fwdAddr = cAddr
				l.WriteTo(p, sAddr)
			}

		}
	}()

	// Create forward plugin instance
	// configure forward plugin to forward to udp packet gateway
	println("forward plugin to = " + l.LocalAddr().String())
	c := caddy.NewTestController("dns", "forward . "+l.LocalAddr().String())
	fs, err := parseForward(c)
	f := fs[0]
	if err != nil {
		t.Errorf("Failed to create forwarder: %s", err)
	}
	f.OnStartup()
	defer f.OnShutdown()

	// Create client query
	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeA)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	if _, err := f.ServeDNS(context.TODO(), rec, m); err != nil {
		t.Fatal("Expected to receive reply, but didn't")
	}

	if x := rec.Msg.Answer[0].Header().Name; x != "example.org." {
		t.Errorf("Expected %s, got %s", "example.org.", x)
	}
}

func example() {

}

func TestMalformedSpoof(t *testing.T) {
	// Stucture:
	// [dnstest client] <-> [forward plugin controller] <-[packet sniffing and injection]-> [upstream dns]

	// start upstream server
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		ret.Answer = append(ret.Answer, test.A("example.org. IN A 1.2.3.4"))
		w.WriteMsg(ret)
	})
	defer s.Close()
	println("upstream dns listening on " + s.Addr)
	_, serverPort, _ := plugin.SplitHostPort(s.Addr)

	// Start packet sniffer
	handle, err := pcap.OpenLive("lo0", 1024, true, 30 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	// Set filter
	var filter string = "udp and port " + serverPort
	println("packet filter = "+ filter)
	err = handle.SetBPFFilter(filter)
	if err != nil {
		log.Fatal(err)
	}

	// process all packets
	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	go func() {
		for packet := range packetSource.Packets() {
			// Process packet here
			fmt.Printf("sniffed packet: %v\n", packet)
		}
	}()
	time.Sleep(30 * time.Second)

	// Create forward plugin instance
	// configure forward plugin to forward to upstream dns server
	println("forward plugin to = " + s.Addr)
	//c := caddy.NewTestController("dns", "forward . "+s.Addr)
	c := caddy.NewTestController("dns", "forward . [::1]:"+serverPort)
	fs, err := parseForward(c)
	f := fs[0]
	if err != nil {
		t.Errorf("Failed to create forward plugin controller: %s", err)
	}
	f.OnStartup()
	defer f.OnShutdown()

	// Create client query
	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeA)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	println("serving query " + m.Question[0].Name)

	if _, err := f.ServeDNS(context.TODO(), rec, m); err != nil {
		t.Fatal("Expected to receive reply, but didn't")
	}
	println("got answer ")
	if x := rec.Msg.Answer[0].Header().Name; x != "example.org." {
		t.Errorf("Expected %s, got %s", "example.org.", x)
	}
	println("done test ")
}
