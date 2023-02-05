package rewrite

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

type ttlResponseRule struct {
	minTTL uint32
	maxTTL uint32
}

func (r *ttlResponseRule) RewriteResponse(res *dns.Msg, rr dns.RR) {
	if rr.Header().Ttl < r.minTTL {
		rr.Header().Ttl = r.minTTL
	} else if rr.Header().Ttl > r.maxTTL {
		rr.Header().Ttl = r.maxTTL
	}
}

type ttlRuleBase struct {
	nextAction string
	response   ttlResponseRule
}

func newTTLRuleBase(nextAction string, minTtl, maxTtl uint32) ttlRuleBase {
	return ttlRuleBase{
		nextAction: nextAction,
		response:   ttlResponseRule{minTTL: minTtl, maxTTL: maxTtl},
	}
}

func (rule *ttlRuleBase) responseRule(match bool) (ResponseRules, Result) {
	if match {
		return ResponseRules{&rule.response}, RewriteDone
	}
	return nil, RewriteIgnored
}

// Mode returns the processing nextAction
func (rule *ttlRuleBase) Mode() string { return rule.nextAction }

type exactTTLRule struct {
	ttlRuleBase
	From string
}

type prefixTTLRule struct {
	ttlRuleBase
	Prefix string
}

type suffixTTLRule struct {
	ttlRuleBase
	Suffix string
}

type substringTTLRule struct {
	ttlRuleBase
	Substring string
}

type regexTTLRule struct {
	ttlRuleBase
	Pattern *regexp.Regexp
}

// Rewrite rewrites the current request based upon exact match of the name
// in the question section of the request.
func (rule *exactTTLRule) Rewrite(ctx context.Context, state request.Request) (ResponseRules, Result) {
	return rule.responseRule(rule.From == state.Name())
}

// Rewrite rewrites the current request when the name begins with the matching string.
func (rule *prefixTTLRule) Rewrite(ctx context.Context, state request.Request) (ResponseRules, Result) {
	return rule.responseRule(strings.HasPrefix(state.Name(), rule.Prefix))
}

// Rewrite rewrites the current request when the name ends with the matching string.
func (rule *suffixTTLRule) Rewrite(ctx context.Context, state request.Request) (ResponseRules, Result) {
	return rule.responseRule(strings.HasSuffix(state.Name(), rule.Suffix))
}

// Rewrite rewrites the current request based upon partial match of the
// name in the question section of the request.
func (rule *substringTTLRule) Rewrite(ctx context.Context, state request.Request) (ResponseRules, Result) {
	return rule.responseRule(strings.Contains(state.Name(), rule.Substring))
}

// Rewrite rewrites the current request when the name in the question
// section of the request matches a regular expression.
func (rule *regexTTLRule) Rewrite(ctx context.Context, state request.Request) (ResponseRules, Result) {
	return rule.responseRule(len(rule.Pattern.FindStringSubmatch(state.Name())) != 0)
}

// newTTLRule creates a name matching rule based on exact, partial, or regex match
func newTTLRule(nextAction string, args ...string) (Rule, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("too few (%d) arguments for a ttl rule", len(args))
	}
	var s string
	if len(args) == 2 {
		s = args[1]
	}
	if len(args) == 3 {
		s = args[2]
	}
	minTtl, maxTtl, valid := isValidTTL(s)
	if !valid {
		return nil, fmt.Errorf("invalid TTL '%s' for a ttl rule", s)
	}
	if len(args) == 3 {
		switch strings.ToLower(args[0]) {
		case ExactMatch:
			return &exactTTLRule{
				newTTLRuleBase(nextAction, minTtl, maxTtl),
				plugin.Name(args[1]).Normalize(),
			}, nil
		case PrefixMatch:
			return &prefixTTLRule{
				newTTLRuleBase(nextAction, minTtl, maxTtl),
				plugin.Name(args[1]).Normalize(),
			}, nil
		case SuffixMatch:
			return &suffixTTLRule{
				newTTLRuleBase(nextAction, minTtl, maxTtl),
				plugin.Name(args[1]).Normalize(),
			}, nil
		case SubstringMatch:
			return &substringTTLRule{
				newTTLRuleBase(nextAction, minTtl, maxTtl),
				plugin.Name(args[1]).Normalize(),
			}, nil
		case RegexMatch:
			regexPattern, err := regexp.Compile(args[1])
			if err != nil {
				return nil, fmt.Errorf("invalid regex pattern in a ttl rule: %s", args[1])
			}
			return &regexTTLRule{
				newTTLRuleBase(nextAction, minTtl, maxTtl),
				regexPattern,
			}, nil
		default:
			return nil, fmt.Errorf("ttl rule supports only exact, prefix, suffix, substring, and regex name matching")
		}
	}
	if len(args) > 3 {
		return nil, fmt.Errorf("many few arguments for a ttl rule")
	}
	return &exactTTLRule{
		newTTLRuleBase(nextAction, minTtl, maxTtl),
		plugin.Name(args[0]).Normalize(),
	}, nil
}

// validTTL returns true if v is valid TTL value.
func isValidTTL(v string) (uint32, uint32, bool) {
	s := strings.Split(v, "-")
	if len(s) == 1 {
		i, err := strconv.ParseUint(s[0], 10, 32)
		if err != nil {
			return 0, 0, false
		}
		return uint32(i), uint32(i), true
	}
	if len(s) == 2 {
		var min, max uint64
		var err error
		if s[0] == "" {
			min = 0
		} else {
			min, err = strconv.ParseUint(s[0], 10, 32)
			if err != nil {
				return 0, 0, false
			}
		}
		if s[1] == "" {
			if s[0] == "" {
				// explicitly reject ttl directive "-" that would otherwise be interpreted
				// as 0-2147483647 which is pretty useless
				return 0, 0, false
			}
			max = 2147483647
		} else {
			max, err = strconv.ParseUint(s[1], 10, 32)
			if err != nil {
				return 0, 0, false
			}
		}
		if min > max {
			// reject invalid range
			return 0, 0, false
		}
		return uint32(min), uint32(max), true
	}
	return 0, 0, false
}
