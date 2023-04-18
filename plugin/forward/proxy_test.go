package forward

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
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

// TestMalformedSpoof tests that a malformed UDP response spoofed to the client's (proxy) source port shouldn't
// block the real response from reaching the client. Note that the spoofed response is an invalid dns payload,
// and contains no message id.
func TestMalformedSpoof(t *testing.T) {
	// Test Flow/Stucture:
	// [dnstest client] <-> [forward plugin controller] <-[packet sniffing and injection]-> [upstream dns]
	// 1. dnstest client makes request to forward controller
	// 2. forward controller sends the request upstream
	// 3. packet sniffer detects the request and injects a malformed spoof response before the upstream server responds
	//    with the real answer.
	// 4. forward receives malformed response, rejects it, and continues waiting for a valid response.
	// 5. forward receives valid response and forwards it down to dnstest client

	inject := make(chan struct{})
	// start upstream server
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		<-inject // wait until the request is intercepted and spoof is injected
		if r.Question[0].Qtype == dns.TypeNS {
			ret := new(dns.Msg)
			ret.SetReply(r)

			ret.Answer = append(ret.Answer, test.NS(". IN NS ."))
			w.WriteMsg(ret)
			return
		}
		ret := new(dns.Msg)
		ret.SetReply(r)
		ret.Answer = append(ret.Answer, test.A("example.org. IN A 1.2.3.4"))
		w.WriteMsg(ret)
	})
	defer s.Close()
	_, serverPort, _ := plugin.SplitHostPort(s.Addr)

	// Start packet sniffer/injector
	// open live capture on loopback device: NOTE -1 timeout = flush packets to channel immediately
	handle, err := pcap.OpenLive("lo", 65535, false, -1) // linux naming
	if err != nil {
		handle, err = pcap.OpenLive("lo0", 65535, false, -1) // MacOS/BSD naming
		if err != nil {
			t.Fatal(err)
		}
	}
	defer handle.Close()
	// set capture filter
	var filter string = "udp and port " + serverPort
	err = handle.SetBPFFilter(filter)
	if err != nil {
		log.Fatal(err)
	}

	// process packets
	malformedPayload := "malformedresponse"
	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	go func() {
		pktNum := 0
		for packet := range packetSource.Packets() {
			if err != nil {
				t.Fatal("failed to sniff request packet")
			}
			pktNum++
			// log some info about the sniffed packet
			if string(packet.ApplicationLayer().Payload()) == malformedPayload {
				fmt.Printf("packet %v <- malformed/spoofed response\n", pktNum)
			} else {
				// decode the dns layer
				dnsPkt := layers.DNS{}
				err := dnsPkt.DecodeFromBytes(packet.ApplicationLayer().Payload(), nil)
				if err != nil {
					t.Fatalf("captured unexpected invalid dns packet: %v\n", err)
				} else {
					if uint16(dnsPkt.Questions[0].Type) == dns.TypeNS && !dnsPkt.QR {
						fmt.Printf("packet %v -> forward health check request\n", pktNum)
					}
					if uint16(dnsPkt.Questions[0].Type) == dns.TypeNS && dnsPkt.QR {
						fmt.Printf("packet %v <- server health response\n", pktNum)
					}
					if uint16(dnsPkt.Questions[0].Type) == dns.TypeA && !dnsPkt.QR {
						fmt.Printf("packet %v -> forwarded A request\n", pktNum)
					}
					if uint16(dnsPkt.Questions[0].Type) == dns.TypeA && dnsPkt.QR {
						fmt.Printf("packet %v <- server A response\n", pktNum)
					}
				}
			}

			// simulate some network delay
			time.Sleep(time.Millisecond * 50)

			// If this is a request (dest port == server's udp port), build and inject a spoofed response.
			udp := packet.Layer(layers.LayerTypeUDP).(*layers.UDP)
			if fmt.Sprintf("%v", udp.DstPort) == serverPort {
				// construct spoofed response
				var linkLayer gopacket.SerializableLayer
				// handle bsd vs linux loopback link layer encapsulation differences
				if linkLo, ok := packet.Layer(layers.LayerTypeLoopback).(*layers.Loopback); ok { //bsd
					linkLayer = &layers.Loopback{Family: linkLo.Family}
				} else if linkEth, ok := packet.Layer(layers.LayerTypeEthernet).(*layers.Ethernet); ok { //linux
					linkLayer = &layers.Ethernet{SrcMAC: linkEth.SrcMAC, DstMAC: linkEth.DstMAC, EthernetType: linkEth.EthernetType}
				}
				ipv6 := packet.Layer(layers.LayerTypeIPv6).(*layers.IPv6)
				ipLayer := &layers.IPv6{Version: 6, SrcIP: ipv6.DstIP, DstIP: ipv6.SrcIP, HopLimit: 64, NextHeader: layers.IPProtocolUDP}
				udpLayer := &layers.UDP{SrcPort: udp.DstPort, DstPort: udp.SrcPort}
				udpLayer.SetNetworkLayerForChecksum(ipLayer)
				payload := gopacket.Payload(gopacket.Payload(malformedPayload))
				spoof := gopacket.NewSerializeBuffer()
				err := gopacket.SerializeLayers(spoof,
					gopacket.SerializeOptions{ComputeChecksums: true, FixLengths: true},
					linkLayer, ipLayer, udpLayer, payload,
				)
				if err != nil {
					t.Fatalf("serialization of spoof failed: %v\n", err)
				}

				// inject spoofed response
				err = handle.WritePacketData(spoof.Bytes())
				if err != nil {
					t.Fatalf("write spoof packet fail: %v\n", err)
				}
				// unblock the server, so it can send the real response
				inject <- struct{}{}
			}
		}
	}()

	// Create forward plugin instance
	// configure forward plugin to forward to upstream dns server
	c := caddy.NewTestController("dns", "forward . "+s.Addr)
	fs, err := parseForward(c)
	if err != nil {
		t.Errorf("Failed to create forward plugin controller: %s", err)
	}
	f := fs[0]

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
	time.Sleep(time.Second) // crude sleep to allow packet sniffer to see all packets before ending test
}
