package edns

import "github.com/miekg/dns"

// TruncateOptions removes EDNS options that cannot fit in size bytes. It keeps the
// OPT header, and leaves responses that already contain a TSIG untouched.
// Callers must still truncate ordinary records and account for any later TSIG.
func TruncateOptions(m *dns.Msg, size int) {
	opt := m.IsEdns0()
	if opt == nil || len(opt.Option) == 0 || m.IsTsig() != nil {
		return
	}
	if m.Len() <= size {
		return
	}
	compressed := *m
	compressed.Compress = true
	if compressed.Len() <= size {
		return
	}

	// RFC 8914 section 3: discard EDE before sacrificing DNS data.
	// dns.Msg.Truncate retains the entire OPT, even if it exceeds the budget.
	bareOPT := &dns.OPT{Hdr: opt.Hdr}
	minimum := &dns.Msg{Question: m.Question, Extra: []dns.RR{bareOPT}, Compress: true}
	remaining := size - minimum.Len()
	// Reserve COOKIE and ECS before optional metadata, regardless of order
	// (RFC 7873 and RFC 7871).
	requiredSize := 0
	for _, option := range opt.Option {
		if option.Option() == dns.EDNS0COOKIE || option.Option() == dns.EDNS0SUBNET {
			requiredSize += dns.Len(&dns.OPT{Hdr: opt.Hdr, Option: []dns.EDNS0{option}}) - dns.Len(bareOPT)
		}
	}
	keepRequired := requiredSize <= remaining
	if keepRequired {
		remaining -= requiredSize
	}
	options := make([]dns.EDNS0, 0, len(opt.Option))
	for _, option := range opt.Option {
		if option.Option() == dns.EDNS0EDE {
			continue
		}
		if (option.Option() == dns.EDNS0COOKIE || option.Option() == dns.EDNS0SUBNET) && keepRequired {
			options = append(options, option)
			continue
		}
		single := &dns.OPT{Hdr: opt.Hdr, Option: []dns.EDNS0{option}}
		length := dns.Len(single) - dns.Len(bareOPT)
		if length <= remaining {
			options = append(options, option)
			remaining -= length
		}
	}
	if len(options) == len(opt.Option) {
		return
	}

	// OPT and its slices may also belong to a request or another response.
	replacement := *opt
	replacement.Option = options
	extra := append([]dns.RR(nil), m.Extra...)
	for i, rr := range extra {
		if rr == opt {
			extra[i] = &replacement
			break
		}
	}
	m.Extra = extra
	m.Truncated = true
}
