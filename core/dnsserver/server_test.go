package dnsserver_test

import (
	"testing"
	"time"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/middleware"
	"github.com/coredns/coredns/middleware/erratic"
	"github.com/coredns/coredns/pb"

	"github.com/miekg/dns"

	"golang.org/x/net/context"

	"google.golang.org/grpc"
)

func TestGrpc(t *testing.T) {
	cfg := &dnsserver.Config{Zone: "."}

	e := &erratic.Erratic{}
	cfg.AddMiddleware(func(next middleware.Handler) middleware.Handler {
		return e
	})
	g, err := dnsserver.NewServergRPC("grpc://:55553", []*dnsserver.Config{cfg})
	if err != nil {
		t.Errorf("Expected no error but got: %s", err)
	}
	l, err := g.Listen()
	if err != nil {
		t.Errorf("Expected no error but got: %s", err)
	}
	go func() {
		err = g.Serve(l)
		if err != nil {
			t.Errorf("Expected no error but got: %s", err)
		}
		l.Close()
	}()

	conn, err := grpc.Dial(":55553", grpc.WithInsecure(), grpc.WithBlock(), grpc.WithTimeout(10*time.Second))
	if err != nil {
		t.Fatalf("Expected no error but got: %s", err)
	}
	client := pb.NewDnsServiceClient(conn)
	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeA)
	msg, err := m.Pack()
	if err != nil {
		t.Errorf("Expected no error but got: %s", err)
	}

	reply, err := client.Query(context.TODO(), &pb.DnsPacket{Msg: msg})
	if err != nil {
		t.Errorf("Expected no error but got: %s", err)
	}

	d := new(dns.Msg)
	err = d.Unpack(reply.Msg)
	if err != nil {
		t.Errorf("Expected no error but got: %s", err)
	}

	if d.Rcode != dns.RcodeSuccess {
		t.Errorf("Expected success but got %s", d.Rcode)
	}
}
