package tree

import (
	"github.com/miekg/dns"
	"net"
	"testing"
)

func TestPrint(t *testing.T) {
	rr1 := dns.A{
		Hdr: dns.RR_Header{
			Name:     dns.Fqdn("server1.example.com"),
			Rrtype:   1,
			Class:    1,
			Ttl:      3600,
			Rdlength: 0,
		},
		A: net.IPv4(10, 0, 1, 1),
	}
	rr2 := dns.A{
		Hdr: dns.RR_Header{
			Name:     dns.Fqdn("server2.example.com"),
			Rrtype:   1,
			Class:    1,
			Ttl:      3600,
			Rdlength: 0,
		},
		A: net.IPv4(10, 0, 1, 2),
	}
	rr3 := dns.A{
		Hdr: dns.RR_Header{
			Name:     dns.Fqdn("server3.example.com"),
			Rrtype:   1,
			Class:    1,
			Ttl:      3600,
			Rdlength: 0,
		},
		A: net.IPv4(10, 0, 1, 3),
	}
	rr4 := dns.A{
		Hdr: dns.RR_Header{
			Name:     dns.Fqdn("server4.example.com"),
			Rrtype:   1,
			Class:    1,
			Ttl:      3600,
			Rdlength: 0,
		},
		A: net.IPv4(10, 0, 1, 4),
	}
	//构造一颗树
	tree := Tree{
		Root:  nil,
		Count: 0,
	}
	tree.Insert(&rr1)
	tree.Insert(&rr2)
	tree.Insert(&rr3)
	tree.Insert(&rr4)

	/**
	 the LLRB tree:

				  server2.example.com.
					/             \
		server1.example.com.   server4.example.com.
			   /
	 server3.example.com.

	*/
	tree.Print()
	/**
	  server2.example.com.
	  server1.example.com. server4.example.com.
	  server3.example.com.
	*/

	t.Log("tree.Print run this successful")
}
