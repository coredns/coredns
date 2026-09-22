//go:build gofuzz

package grpc

import (
	"context"

	"github.com/coredns/coredns/pb"
	"github.com/coredns/coredns/plugin/pkg/fuzz"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeClient implements pb.DnsServiceClient without doing any network I/O.
// Its behavior is controlled by the mode field.
type fakeClient struct {
	mode byte
	idx  int
}

func (f *fakeClient) Query(_ context.Context, in *pb.DnsPacket, _ ...grpcgo.CallOption) (*pb.DnsPacket, error) {
	// Derive mode deterministically from request bytes to vary behavior per call.
	m := f.mode
	if len(in.GetMsg()) > 0 {
		b := in.GetMsg()[f.idx%len(in.GetMsg())]
		f.idx++
		m = b
	}

	switch m % 12 {
	case 0:
		// Success echo: return the same bytes.
		return &pb.DnsPacket{Msg: in.GetMsg()}, nil
	case 1:
		// Return NotFound to exercise NXDOMAIN conversion and optional fallthrough.
		return nil, status.Error(codes.NotFound, "not found")
	case 2:
		// Return a transient error to trigger retry/rotation.
		return nil, status.Error(codes.Unavailable, "unavailable")
	case 3:
		// Corrupt response that fails dns.Msg Unpack.
		return &pb.DnsPacket{Msg: []byte{0x00, 0x01, 0x02}}, nil
	case 4:
		// Valid DNS message with mismatched ID/qname to trigger formerr path in ServeDNS.
		var req dns.Msg
		if err := req.Unpack(in.GetMsg()); err != nil {
			// If input isn't a DNS message, just echo to avoid blocking fuzzing.
			return &pb.DnsPacket{Msg: in.GetMsg()}, nil
		}
		resp := new(dns.Msg)
		resp.SetReply(&req)
		resp.Id = req.Id + 1
		// Alter question name if present.
		if len(req.Question) > 0 {
			resp.Question[0].Name = "example.net."
		}
		packed, err := resp.Pack()
		if err != nil {
			return &pb.DnsPacket{Msg: in.GetMsg()}, nil
		}
		return &pb.DnsPacket{Msg: packed}, nil
	case 5:
		// Success with EDNS and larger answer to stress flags and sizes.
		var req dns.Msg
		if err := req.Unpack(in.GetMsg()); err != nil {
			return &pb.DnsPacket{Msg: in.GetMsg()}, nil
		}
		resp := new(dns.Msg)
		resp.SetReply(&req)
		// Set EDNS0 with varying UDP size and DO bit based on m.
		size := uint16(512)
		if (m>>1)&1 == 1 {
			size = 1232
		}
		if (m>>2)&1 == 1 {
			size = 4096
		}
		do := ((m>>3)&1 == 1)
		resp.SetEdns0(size, do)
		// Optionally set TC bit to exercise truncation handling.
		if (m>>4)&1 == 1 {
			resp.Truncated = true
		}
		// Add a few TXT records to grow the payload.
		name := "."
		if len(req.Question) > 0 {
			name = req.Question[0].Name
		}
		n := int(1 + (m % 16))
		for range n {
			resp.Answer = append(resp.Answer, &dns.TXT{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 0}, Txt: []string{"aaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbb"}})
		}
		packed, err := resp.Pack()
		if err != nil {
			return &pb.DnsPacket{Msg: in.GetMsg()}, nil
		}
		return &pb.DnsPacket{Msg: packed}, nil
	case 6:
		return nil, status.Error(codes.DeadlineExceeded, "timeout")
	case 7:
		return nil, status.Error(codes.Internal, "internal")
	case 8:
		return nil, status.Error(codes.ResourceExhausted, "quota")
	case 9:
		return nil, status.Error(codes.PermissionDenied, "denied")
	case 10:
		// NODATA: NOERROR with empty Answer and SOA in Authority.
		var req dns.Msg
		if err := req.Unpack(in.GetMsg()); err != nil {
			return &pb.DnsPacket{Msg: in.GetMsg()}, nil
		}
		resp := new(dns.Msg)
		resp.SetRcode(&req, dns.RcodeSuccess)
		name := "."
		if len(req.Question) > 0 {
			name = req.Question[0].Name
		}
		resp.Ns = append(resp.Ns, &dns.SOA{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: 60}, Ns: "ns.example.", Mbox: "hostmaster.example.", Serial: 1, Refresh: 3600, Retry: 600, Expire: 86400, Minttl: 60})
		packed, err := resp.Pack()
		if err != nil {
			return &pb.DnsPacket{Msg: in.GetMsg()}, nil
		}
		return &pb.DnsPacket{Msg: packed}, nil
	case 11:
		// TC-only: truncated response without answers.
		var req dns.Msg
		if err := req.Unpack(in.GetMsg()); err != nil {
			return &pb.DnsPacket{Msg: in.GetMsg()}, nil
		}
		resp := new(dns.Msg)
		resp.SetReply(&req)
		resp.Truncated = true
		packed, err := resp.Pack()
		if err != nil {
			return &pb.DnsPacket{Msg: in.GetMsg()}, nil
		}
		return &pb.DnsPacket{Msg: packed}, nil
	default:
		// Empty/zero-length response to exercise unpack error path.
		return &pb.DnsPacket{Msg: nil}, nil
	}
}

// Fuzz exercises the grpc plugin using a fake client and the shared fuzz harness.
func Fuzz(data []byte) int {
	if len(data) == 0 {
		return 0
	}

	cfg := data[0]
	rest := data[1:]

	g := &GRPC{
		from: ".",
		Next: test.ErrorHandler(),
	}

	// Select policy based on cfg bits to vary list() ordering.
	switch cfg % 3 {
	case 0:
		g.p = &random{}
	case 1:
		g.p = &roundRobin{}
	default:
		g.p = &sequential{}
	}

	// Optionally enable fallthrough; choose scope based on input bit.
	if cfg&0x80 != 0 {
		if cfg&0x01 != 0 {
			g.Fall.SetZonesFromArgs([]string{"."})
		} else {
			g.Fall.SetZonesFromArgs([]string{g.from})
		}
	}

	// Create 0–3 fake proxies with varied behaviors.
	numProxies := int((cfg >> 4) & 0x03)
	if numProxies == 0 {
		if _, is := g.p.(*roundRobin); is {
			// Avoid divide-by-zero in roundRobin policy when pool is empty.
			g.p = &sequential{}
		}
	}
	for i := range numProxies {
		mode := byte(i)
		if len(rest) > 0 {
			mode = rest[i%len(rest)]
		}
		p := &Proxy{addr: "fake"}
		p.client = &fakeClient{mode: mode}
		g.proxies = append(g.proxies, p)
	}

	// Deterministically set a narrow from to miss match and hit Next/SERVFAIL paths.
	if cfg&0x20 != 0 {
		g.from = "_not_matching_."
	}

	// Optionally construct a tiny deterministic query to vary RD/CD flags.
	if cfg&0x08 != 0 {
		var rq dns.Msg
		rq.SetQuestion("example.org.", dns.TypeA)
		rq.RecursionDesired = (cfg&0x04 != 0)
		rq.CheckingDisabled = (cfg&0x02 != 0)
		if packed, err := rq.Pack(); err == nil {
			rest = packed
		}
	}

	return fuzz.Do(g, rest)
}
