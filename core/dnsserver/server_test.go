package dnsserver

import (
	"testing"

	"github.com/coredns/coredns/pb"
	"github.com/mholt/caddy"
	"github.com/miekg/dns"
        "golang.org/x/net/context"
        "google.golang.org/grpc"
)

func TestGrpc(t *testing.T) {
	cfgText := `grpc://.:55553 { erratic }`
	ctrl := caddy.NewTestController("dns", cfgText)
	cfg := GetConfig(ctrl)
	g, err := NewServer(":55553", []*Config{cfg})
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
	}()

        conn, err := grpc.Dial("127.0.0.1:55553", grpc.WithInsecure(), grpc.WithBlock(), grpc.WithTimeout(g.connTimeout))
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

	//TODO: check reply
	if reply == nil {
		t.Error("Expected reply but got nil")
	}
}
