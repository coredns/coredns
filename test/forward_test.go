//go:build libpcap

package test

import (
	"fmt"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/miekg/dns"
)

// TestLibpcapMalformedSpoof tests that a malformed UDP response spoofed to the client's (proxy) source port shouldn't
// block the real response from reaching the client. Note that the spoofed response is an invalid dns payload,
// and contains no message id.
func TestLibpcapMalformedSpoof(t *testing.T) {
	// Test Flow/Stucture:
	// [dnstest client] <-> [coredns forwarder] <-[packet sniffing and injection]-> [upstream dns]
	// 1. dnstest client makes request to coredns forwarder
	// 2. coredns forwarder sends the request upstream
	// 3. packet sniffer detects the request and injects a malformed spoof response before the upstream server responds
	//    with the real answer.
	// 4. forward receives malformed response, rejects it, and continues waiting for a valid response.
	// 5. forward receives valid response and forwards it down to dnstest client

	// start upstream server
	inject := make(chan struct{})
	upstreamServer := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
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
	defer upstreamServer.Close()
	_, serverPort, _ := plugin.SplitHostPort(upstreamServer.Addr)

	// Start packet sniffer/injector
	// open live capture on loopback device: NOTE -1 timeout = flush packets to channel immediately
	sniffer, err := pcap.OpenLive("lo", 65535, false, -1) // linux naming
	if err != nil {
		sniffer, err = pcap.OpenLive("lo0", 65535, false, -1) // MacOS/BSD naming
		if err != nil {
			t.Fatal(err)
		}
	}
	defer sniffer.Close()
	// set capture filter
	var filter string = "udp and port " + serverPort
	err = sniffer.SetBPFFilter(filter)
	if err != nil {
		t.Fatalf("Setting pcap filter failed: %v", err)
	}

	// process packets
	malformedPayload := "malformedresponse"
	packetSource := gopacket.NewPacketSource(sniffer, sniffer.LinkType())
	go func() {
		pktNum := 0
		for packet := range packetSource.Packets() {
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
				err = sniffer.WritePacketData(spoof.Bytes())
				if err != nil {
					t.Fatalf("write spoof packet fail: %v\n", err)
				}
				// unblock the server, so it can send the real response
				inject <- struct{}{}
			}
		}
	}()

	// Create CoreDNS instance that forwards to upstream server
	corefile := `example.org:0 {
		forward . ` + upstreamServer.Addr + `
	}`
	fwdInstance, fwdAddr, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not create CoreDNS forwarding instance: %s", err)
	}
	defer fwdInstance.Stop()

	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeA)
	resp, err := dns.Exchange(m, fwdAddr)

	if err != nil {
		t.Fatalf("Query received error: %s", err)
	}
	if err != nil {
		t.Fatalf("Query received error: %s", err)
	}

	if x := len(resp.Answer); x != 1 {
		t.Fatalf("Expected one answer, got %v", x)
	}

	if x := resp.Answer[0].Header().Name; x != "example.org." {
		t.Errorf("Expected %s, got %s", "example.org.", x)
	}
	if t.Failed() {
		fmt.Println("Test failed")
	} else {
		fmt.Println("Test passed")
	}

	time.Sleep(time.Second) // crude sleep to allow packet sniffer to see all packets before ending test
}
