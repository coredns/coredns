package header

import (
	"fmt"
	"strings"

	clog "github.com/coredns/coredns/plugin/pkg/log"

	"github.com/miekg/dns"
)

// Supported flags
const (
	authoritative      = "aa"
	recursionAvailable = "ra"
	recursionDesired   = "rd"
)

var log = clog.NewWithPlugin("header")

// ResponseHeaderWriter is a response writer that allows modifying dns.MsgHdr
type ResponseHeaderWriter struct {
	dns.ResponseWriter
	Rules []Rule
}

// WriteMsg implements the dns.ResponseWriter interface.
func (r *ResponseHeaderWriter) WriteMsg(res *dns.Msg) error {
	// handle all supported flags
	for _, rule := range r.Rules {
		switch rule.Flag {
		case authoritative:
			res.Authoritative = rule.State
		case recursionAvailable:
			res.RecursionAvailable = rule.State
		case recursionDesired:
			res.RecursionDesired = rule.State
		}
	}

	return r.ResponseWriter.WriteMsg(res)
}

// Write implements the dns.ResponseWriter interface.
func (r *ResponseHeaderWriter) Write(buf []byte) (int, error) {
	log.Warning("ResponseHeaderWriter called with Write: not ensuring headers")
	n, err := r.ResponseWriter.Write(buf)
	return n, err
}

// Rule is used to set/clear Flag in dns.MsgHdr
type Rule struct {
	Flag  string
	State bool
}

func newRules(key string, args []string) ([]Rule, error) {
	if key == "" {
		return nil, fmt.Errorf("no flag action provided")
	}

	if len(args) < 1 {
		return nil, fmt.Errorf("invalid length for flags, at least one should be provided")
	}

	var state bool
	action := strings.ToLower(key)
	switch action {
	case "set":
		state = true
	case "clear":
		state = false
	default:
		return nil, fmt.Errorf("unknown flag action=%s, should be set or clear", action)
	}

	var rules []Rule
	for _, arg := range args {
		flag := strings.ToLower(arg)
		switch flag {
		case authoritative:
		case recursionAvailable:
		case recursionDesired:
		default:
			return nil, fmt.Errorf("unknown/unsupported flag=%s", flag)
		}
		rule := Rule{Flag: flag, State: state}
		rules = append(rules, rule)
	}

	return rules, nil
}
