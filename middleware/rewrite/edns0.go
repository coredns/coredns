// Package rewrite is middleware for rewriting requests internally to something different.
package rewrite

import (
	"encoding/hex"
	"errors"
	"strconv"

	"github.com/miekg/dns"
)

// Edns0Rule is a class rewrite rule.
type Edns0Rule struct {
	action string
	code   uint16
	data   []byte
	match  []byte // TODO: add match input for replace rule
}

// SetEDNS0Attrs will alter the request
func (rule *Edns0Rule) SetEDNS0Attrs(r *dns.Msg) {
	o := r.IsEdns0()
	if o == nil {
		r.SetEdns0(4096, true)
		o = r.IsEdns0()
	}

	found := false
	for _, s := range o.Option {
		switch e := s.(type) {
		case *dns.EDNS0_LOCAL:
			if rule.code == e.Code {
				if rule.action == "replace" || rule.action == "replace_or_append" {
					e.Data = rule.data
				}
				found = true
				break
			}
		}
	}

	// add option if not found
	if !found && (rule.action == "append" || rule.action == "replace_or_append") {
		o.SetDo(true)
		var opt *dns.EDNS0_LOCAL
		opt.Code = rule.code
		opt.Data = rule.data
		o.Option = append(o.Option, opt)
	}

}

// Initializer
func (rule Edns0Rule) New(args ...string) (Rule, error) {
	var err error
	if len(args) < 3 {
		return rule, errors.New("Wrong argument count")
	}

	switch args[0] {
	case "append":
	case "replace":
	case "replace_or_append":
	default:
		return rule, errors.New("invalid action")
	}

	c, err := strconv.ParseUint(args[1], 0, 16)
	if err != nil {
		return rule, err
	}

	decoded, err := hex.DecodeString(args[2])
	if err != nil {
		return rule, err
	}

	return &Edns0Rule{args[0], uint16(c), decoded, nil}, nil

}

// Rewrite rewrites the the current request.
func (rule Edns0Rule) Rewrite(r *dns.Msg) Result {
	rule.SetEDNS0Attrs(r)
	return RewriteDone
}
