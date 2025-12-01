package tree

import (
	"net"
	"os"
	"strings"
	"testing"

	"github.com/miekg/dns"
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
	tree := Tree{
		Root:  nil,
		Count: 0,
	}
	tree.Insert(&rr1)
	tree.Insert(&rr2)
	tree.Insert(&rr3)
	tree.Insert(&rr4)

	/**
	build a LLRB tree, the height of the tree is 3, look like:

				  server2.example.com.
					/             \
		server1.example.com.   server4.example.com.
			   /
	 server3.example.com.

	*/

	f, err := os.CreateTemp("", "print_test_tmp")
	if err != nil {
		t.Error(err)
	}
	defer os.Remove(f.Name())
	//Redirect the printed results to a tmp file for later comparison
	os.Stdout = f

	tree.Print()
	/**
	  server2.example.com.
	  server1.example.com. server4.example.com.
	  server3.example.com.
	*/

	buf := make([]byte, 256)
	f.Seek(0, 0)
	_, err = f.Read(buf)
	if err != nil {
		f.Close()
		t.Error(err)
	}
	height := strings.Count(string(buf), ". \n")
	//Compare the height of the print with the actual height of the tree
	if height != 3 {
		t.Fatal("The number of rows is inconsistent with the actual number of rows in the tree itself.")
	}
}
