package tls

import (
	"context"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/coredns/coredns/request"

	"github.com/mholt/acmez/acme"
	"github.com/miekg/dns"
)

// DNSSolver is minimal dns.Server that can solve the ACME Challenge
type DNSSolver struct {
	Port      int
	m         sync.Mutex
	server    *dns.Server
	readyChan chan string
}

// Start starts a dns.Server that can solve the ACME Challenge, which means it answer on TXT requests
// that start with _acme-challenge - this server will ignore all other requests
func (ds *DNSSolver) Start(p net.PacketConn, challenge acme.Challenge) error {
	log.Debug("ACME DNS-Server starts")
	ds.m.Lock()
	ds.server = &dns.Server{PacketConn: p, Net: "udp", Handler: dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		acmeRequest := true
		state := request.Request{W: w, Req: r}

		log.Debugf("Received DNS request | name: %s, type: %s, source ip: %s \n", state.Name(), state.Type(), state.IP())
		m := new(dns.Msg)
		m.SetReply(r)

		// Answering CAA Requests is mandatory for some CA's.
		// Let's Encrypt will not issue a Certificate if these requests time out
		if state.QType() == dns.TypeCAA {
			hdr := dns.RR_Header{Name: state.QName(), Rrtype: dns.TypeCAA, Class: dns.ClassANY, Ttl: 0}
			m.Answer = append(m.Answer, &dns.CAA{Hdr: hdr})
		}

		if state.QType() != dns.TypeTXT {
			acmeRequest = false
		}

		if !checkDNSChallenge(state.Name()) {
			acmeRequest = false
		}

		if acmeRequest {
			log.Info("Answering ACME DNS request:", state.Name())
			hdr := dns.RR_Header{Name: state.QName(), Rrtype: dns.TypeTXT, Class: dns.ClassANY, Ttl: 0}
			m.Answer = append(m.Answer, &dns.TXT{Hdr: hdr, Txt: []string{challenge.DNS01KeyAuthorization()}})
		}
		w.WriteMsg(m)
		return
	})}
	ds.m.Unlock()

	ds.readyChan <- "ready"
	return ds.server.ActivateAndServe()
}

// ShutDown is wrapper around dns.Server.shutdown()
func (ds *DNSSolver) ShutDown() error {
	err := ds.server.Shutdown()
	return err
}

const (
	dnsChallengeString = "_acme-challenge."
)

// check for the prefix _acme-challenge
func checkDNSChallenge(zone string) bool {
	return strings.HasPrefix(zone, dnsChallengeString)
}

// Present is called just before a challenge is initiated.
// The implementation MUST prepare anything that is necessary
// for completing the challenge
// for CoreDNS that means that we need to start a DNS Server
// that can answer DNS requests for the Challenge
func (ds *DNSSolver) Present(ctx context.Context, challenge acme.Challenge) error {
	addr := net.UDPAddr{
		Port: ds.Port,
		IP:   net.ParseIP("0.0.0.0"),
	}

	l, err := net.ListenUDP("udp", &addr)
	if err != nil {
		log.Errorf("Failed to create Listener: %v \n", err)
	}

	go func() {
		// start a dns.server that runs in a seperate goroutine
		err := ds.Start(l, challenge)
		if err != nil {
			log.Errorf("Failed to start DNS Server for ACME Challenge: %v \n", err)
		}
	}()
	return nil
}

// Wait waits for the dns.server that we start in Present() to be ready
func (ds *DNSSolver) Wait(ctx context.Context, challenge acme.Challenge) error {
	select {
	case <-ds.readyChan:
		log.Debug("ACME Challenge is ready")
		return nil
	case <-time.After(4 * time.Second):
		// Wait no longer than 4 seconds
		log.Warning("ACME Server take too long to confirm ready")
		return nil
	}
}

// CleanUp is called after a challenge is finished, whether
// successful or not. It stops the dns.server that we started in Present()
func (ds *DNSSolver) CleanUp(ctx context.Context, challenge acme.Challenge) error {
	err := ds.ShutDown()
	if err != nil {
		log.Errorf("Failed to Shutdown the ACME DNS-Server: %v \n", err)
	}
	return nil
}
