// Package rewrite is middleware for rewriting requests internally to something different.
package rewrite

import (
	"encoding/hex"
	"log"
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
		var opt *dns.EDNS0_LOCAL
		opt.Code = rule.code
		opt.Data = rule.data
		o.Option = append(o.Option, opt)
	}

}

// Initializer
func (rule Edns0Rule) New(args ...string) Rule {
	if len(args) < 3 {
		log.Printf("[WARN] %s is invalid", args)
		return rule
	}

	switch args[0] {
	case "append":
	case "replace":
	case "set":
	default:
		log.Printf("[WARN] %s is invalid", args[0])
		return rule
	}

	c, err := strconv.ParseUint(args[1], 0, 16)
	if err != nil {
		log.Printf("[WARN] %s is invalid", args[1])
		return rule
	}

	decoded, err := hex.DecodeString(args[2])
	if err != nil {
		log.Printf("[WARN] %s is invalid", args[2])
		return rule
	}

	return &Edns0Rule{args[0], uint16(c), decoded, nil}

}

// Rewrite rewrites the the current request.
func (rule Edns0Rule) Rewrite(r *dns.Msg) Result {
	rule.SetEDNS0Attrs(r)
	return RewriteDone
}
