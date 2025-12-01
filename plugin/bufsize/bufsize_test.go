package bufsize

import (
	"context"
	"testing"

	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/plugin/whoami"

	"github.com/miekg/dns"
)

func TestBufsize(t *testing.T) {
	const maxBufSize = 1024

	setUpWithRequestBufsz := func(bufferSize uint16) (Bufsize, *dns.Msg) {
		p := Bufsize{
			Size: maxBufSize,
			Next: whoami.Whoami{},
		}
		r := new(dns.Msg)
		r.SetQuestion(dns.Fqdn("."), dns.TypeA)
		r.Question[0].Qclass = dns.ClassINET
		if bufferSize > 0 {
			r.SetEdns0(bufferSize, false)
		}
		return p, r
	}

	t.Run("Limit response buffer size", func(t *testing.T) {
		// GIVEN
		//		plugin initialized with maximum buffer size
		//		request has larger buffer size than allowed
		p, r := setUpWithRequestBufsz(maxBufSize + 128)

		// WHEN
		//		request is processed
		_, err := p.ServeDNS(context.Background(), &test.ResponseWriter{}, r)

		// THEN
		//		no error
		//		OPT RR present
		//		request buffer size is limited
		if err != nil {
			t.Errorf("unexpected error %s", err)
		}
		option := r.IsEdns0()
		if option == nil {
			t.Errorf("OPT RR not present")
		}
		if option.UDPSize() != maxBufSize {
			t.Errorf("buffer size not limited")
		}
	})

	t.Run("Do not increase response buffer size", func(t *testing.T) {
		// GIVEN
		//		plugin initialized with maximum buffer size
		//		request has smaller buffer size than allowed
		const smallerBufferSize = maxBufSize - 128
		p, r := setUpWithRequestBufsz(smallerBufferSize)

		// WHEN
		//		request is processed
		_, err := p.ServeDNS(context.Background(), &test.ResponseWriter{}, r)

		// THEN
		//		no error
		//		request buffer size is not expanded
		if err != nil {
			t.Errorf("unexpected error %s", err)
		}
		option := r.IsEdns0()
		if option == nil {
			t.Errorf("OPT RR not present")
		}
		if option.UDPSize() != smallerBufferSize {
			t.Errorf("buffer size should not be increased")
		}
	})

	t.Run("Buffer size should not be set", func(t *testing.T) {
		// GIVEN
		//		plugin initialized with maximum buffer size
		//		request has no EDNS0 option set
		p, r := setUpWithRequestBufsz(0)

		// WHEN
		//		request is processed
		_, err := p.ServeDNS(context.Background(), &test.ResponseWriter{}, r)

		// THEN
		//		no error
		//		OPT RR is not appended
		if err != nil {
			t.Errorf("unexpected error %s", err)
		}
		if r.IsEdns0() != nil {
			t.Errorf("EDNS0 enabled for incoming request")
		}
	})
}
