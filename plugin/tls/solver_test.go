package tls

import (
	"fmt"
	"net"
	"testing"

	"github.com/mholt/acmez/acme"
	"github.com/miekg/dns"
)

func setupACME(readyChan chan string) {
	acmeServer := &DNSSolver{
		Port:      2053,
		readyChan: readyChan,
	}
	addr := net.UDPAddr{
		Port: 2053,
		IP:   net.ParseIP("0.0.0.0"),
	}

	l, err := net.ListenUDP("udp", &addr)
	if err != nil {
		fmt.Println(err)
	}
	go func() {
		challenge := acme.Challenge{}
		err := acmeServer.Start(l, challenge)
		if err != nil {
			fmt.Println(err)
		}
	}()
	return
}

func TestSolveChallenge(t *testing.T) {
	testcases := []struct {
		name     string
		question string
		answer   bool
	}{
		{
			name:     "ACME Challenge",
			question: "_acme-challenge.example.com.",
			answer:   true,
		},
		{
			name:     "ACME Challenge on a subdomain",
			question: "_acme-challenge.sub.example.com.",
			answer:   true,
		},
		{
			name:     "Not an ACME request",
			question: "fail.example.com.",
			answer:   false,
		},
	}
	readyChan := make(chan string)
	setupACME(readyChan)
	<-readyChan
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			m := new(dns.Msg)
			m.SetQuestion(tc.question, dns.TypeTXT)
			resp, err := dns.Exchange(m, "127.0.0.1:2053")
			if err != nil {
				t.Fatalf("Expected to receive reply, but didn't: %v", err)
			}
			if tc.answer {
				if len(resp.Answer) == 0 {
					t.Fatalf("Expected to receive at least one RR in the answer section, got none")
				}
			} else {
				if len(resp.Answer) != 0 {
					t.Fatalf("Expected to receive no RR in the answer section, got at least one")
				}
			}
		})
	}
}
