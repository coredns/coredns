// Package rewrite is middleware for rewriting requests internally to something different.
package rewrite

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/miekg/dns"
)

// edns0LocalRule is a class rewrite rule.
type edns0LocalRule struct {
	action string
	code   uint16
	data   []byte
	match  []byte // TODO: add match input for replace rule
}

type edns0NsidRule struct {
	action string
}

func getEdns0Opt(r *dns.Msg) *dns.OPT {
	o := r.IsEdns0()
	if o == nil {
		r.SetEdns0(4096, true)
		o = r.IsEdns0()
	}
	return o
}

func (rule *edns0NsidRule) Rewrite(r *dns.Msg) Result {
	o := getEdns0Opt(r)
	found := false
	for _, s := range o.Option {
		switch e := s.(type) {
		case *dns.EDNS0_NSID:
			if rule.action == "replace" || rule.action == "set" {
				e.Nsid = "" // make sure it is empty for request
			}
			found = true
			break
		}
	}

	// add option if not found
	if !found && (rule.action == "append" || rule.action == "set") {
		o.SetDo(true)
		o.Option = append(o.Option, &dns.EDNS0_NSID{dns.EDNS0NSID, ""})
	}

	return RewriteDone
}

// SetEDNS0Attrs will alter the request
func (rule *edns0LocalRule) Rewrite(r *dns.Msg) Result {
	o := getEdns0Opt(r)
	found := false
	for _, s := range o.Option {
		switch e := s.(type) {
		case *dns.EDNS0_LOCAL:
			if rule.code == e.Code {
				if rule.action == "replace" || rule.action == "set" {
					e.Data = rule.data
				}
				found = true
				break
			}
		}
	}

	// add option if not found
	if !found && (rule.action == "append" || rule.action == "set") {
		o.SetDo(true)
		var opt dns.EDNS0_LOCAL
		opt.Code = rule.code
		opt.Data = rule.data
		o.Option = append(o.Option, &opt)
	}

	return RewriteDone
}

// Initializer
func newEdns0Rule(args ...string) (Rule, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("Too few arguments for an EDNS0 rule")
	}

	action := strings.ToLower(args[0])
	switch action {
	case "append":
	case "replace":
	case "set":
	default:
		return nil, fmt.Errorf("invalid action: '%s'", action)
	}

	switch strings.ToLower(args[1]) {
	case "local":
		if len(args) != 3 {
			return nil, fmt.Errorf("EDNS0 local rules require exactly three args")
		}
		return newEdns0LocalRule(action, args[1], args[2])
	case "nsid":
		if len(args) != 2 {
			return nil, fmt.Errorf("EDNS0 NSID rules do not accept args")
		}
		return &edns0NsidRule{action: action}, nil
	default:
		return nil, fmt.Errorf("Invalid rule type '%s'", args[1])
	}
}

func newEdns0LocalRule(action, code, data string) (*edns0LocalRule, error) {
	c, err := strconv.ParseUint(code, 0, 16)
	if err != nil {
		return nil, err
	}

	decoded := []byte(data)
	if strings.HasPrefix(data, "0x") {
		decoded, err = hex.DecodeString(data[2:])
		if err != nil {
			return nil, err
		}
	}

	return &edns0LocalRule{action, uint16(c), decoded, nil}, nil
}
