package kubernetes

import "github.com/miekg/dns"

// AutoPathWriter implements a ResponseWriter that also does the following:
// * reverts question section of a packet to its original state.
//   This is done to avoid the 'Question section mismatch:' error in client.
// * Defers write to the client if the response code is NXDOMAIN.  This is needed
//   to enable further search path probing if a search was not sucessful.
// * Allow forced write to client regardless of response code.  This is needed to
//   write the packet to the client if the final search in the path fails to
//   produce results.
// * Overwrites response code with AutoPathWriter.Rcode if the response code
//   is NXDOMAIN (NameError).  This is needed to support the AutoPath.OnNXDOMAIN
//   function, which returns a NOERROR to client instead of NXDOMAIN if the final
//   search in the path fails to produce results.

type AutoPathWriter struct {
	dns.ResponseWriter
	original dns.Question
	Rcode    int
	Sent     bool
}

// NewAutoPathWriter returns a pointer to a new AutoPathWriter
func NewAutoPathWriter(w dns.ResponseWriter, r *dns.Msg) *AutoPathWriter {
	return &AutoPathWriter{
		ResponseWriter: w,
		original:       r.Question[0],
		Rcode:          dns.RcodeNameError,
	}
}

// WriteMsg records the status code and calls the
// underlying ResponseWriter's WriteMsg method unless
// the write is deferred.
func (r *AutoPathWriter) WriteMsg(res *dns.Msg) error {
	if res.Rcode == dns.RcodeNameError {
		res.Rcode = r.Rcode
	}
	for _, a := range res.Answer {
		if r.original.Name == a.Header().Name {
			continue
		}
		cname := dns.CNAME{
			Hdr: dns.RR_Header{
				Name:   r.original.Name,
				Rrtype: dns.TypeCNAME,
				Class:  dns.ClassINET,
				Ttl:    a.Header().Ttl},
			Target: dns.Fqdn(a.Header().Name)}
		res.Answer = append(res.Answer, &cname)
	}
	res.Question[0] = r.original
	if res.Rcode != dns.RcodeSuccess {
		return nil
	}
	r.Sent = true
	return r.ResponseWriter.WriteMsg(res)

}

// Force the write to client regardless of response code
func (r *AutoPathWriter) ForceWriteMsg(res *dns.Msg) error {
	return r.ResponseWriter.WriteMsg(res)
}

// Write is a wrapper that records the size of the message that gets written.
func (r *AutoPathWriter) Write(buf []byte) (int, error) {
	n, err := r.ResponseWriter.Write(buf)
	return n, err
}

// Hijack implements dns.Hijacker. It simply wraps the underlying
// ResponseWriter's Hijack method if there is one, or returns an error.
func (r *AutoPathWriter) Hijack() {
	r.ResponseWriter.Hijack()
	return
}
