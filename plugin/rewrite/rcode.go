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

type rcodeResponseRule struct {
	old int
	new int
}

func (r *rcodeResponseRule) RewriteResponse(res *dns.Msg, rr dns.RR) {
	if r.old == res.MsgHdr.Rcode {
		res.MsgHdr.Rcode = r.new
	}
}

type rcodeRuleBase struct {
	nextAction string
	response   rcodeResponseRule
}

func newRCodeRuleBase(nextAction string, old, new int) rcodeRuleBase {
	return rcodeRuleBase{
		nextAction: nextAction,
		response:   rcodeResponseRule{old: old, new: new},
	}
}

func (rule *rcodeRuleBase) responseRule(match bool) (ResponseRules, Result) {
	if match {
		return ResponseRules{&rule.response}, RewriteDone
	}
	return nil, RewriteIgnored
}

// Mode returns the processing nextAction
func (rule *rcodeRuleBase) Mode() string { return rule.nextAction }

type exactRCodeRule struct {
	rcodeRuleBase
	From string
}

type prefixRCodeRule struct {
	rcodeRuleBase
	Prefix string
}

type suffixRCodeRule struct {
	rcodeRuleBase
	Suffix string
}

type substringRCodeRule struct {
	rcodeRuleBase
	Substring string
}

type regexRCodeRule struct {
	rcodeRuleBase
	Pattern *regexp.Regexp
}

// Rewrite rewrites the current request based upon exact match of the name
// in the question section of the request.
func (rule *exactRCodeRule) Rewrite(ctx context.Context, state request.Request) (ResponseRules, Result) {
	return rule.responseRule(rule.From == state.Name())
}

// Rewrite rewrites the current request when the name begins with the matching string.
func (rule *prefixRCodeRule) Rewrite(ctx context.Context, state request.Request) (ResponseRules, Result) {
	return rule.responseRule(strings.HasPrefix(state.Name(), rule.Prefix))
}

// Rewrite rewrites the current request when the name ends with the matching string.
func (rule *suffixRCodeRule) Rewrite(ctx context.Context, state request.Request) (ResponseRules, Result) {
	return rule.responseRule(strings.HasSuffix(state.Name(), rule.Suffix))
}

// Rewrite rewrites the current request based upon partial match of the
// name in the question section of the request.
func (rule *substringRCodeRule) Rewrite(ctx context.Context, state request.Request) (ResponseRules, Result) {
	return rule.responseRule(strings.Contains(state.Name(), rule.Substring))
}

// Rewrite rewrites the current request when the name in the question
// section of the request matches a regular expression.
func (rule *regexRCodeRule) Rewrite(ctx context.Context, state request.Request) (ResponseRules, Result) {
	return rule.responseRule(len(rule.Pattern.FindStringSubmatch(state.Name())) != 0)
}

// newRCodeRule creates a name matching rule based on exact, partial, or regex match
func newRCodeRule(nextAction string, args ...string) (Rule, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("too few (%d) arguments for a rcode rule", len(args))
	}
	var oldStr, newStr string
	if len(args) == 3 {
		oldStr, newStr = args[1], args[2]
	}
	if len(args) == 4 {
		oldStr, newStr = args[2], args[3]
	}
	old, valid := isValidRCode(oldStr)
	if !valid {
		return nil, fmt.Errorf("invalid matching RCODE '%s' for a rcode rule", oldStr)
	}
	new, valid := isValidRCode(newStr)
	if !valid {
		return nil, fmt.Errorf("invalid replacement RCODE '%s' for a rcode rule", newStr)
	}
	if len(args) == 4 {
		switch strings.ToLower(args[0]) {
		case ExactMatch:
			return &exactRCodeRule{
				newRCodeRuleBase(nextAction, old, new),
				plugin.Name(args[1]).Normalize(),
			}, nil
		case PrefixMatch:
			return &prefixRCodeRule{
				newRCodeRuleBase(nextAction, old, new),
				plugin.Name(args[1]).Normalize(),
			}, nil
		case SuffixMatch:
			return &suffixRCodeRule{
				newRCodeRuleBase(nextAction, old, new),
				plugin.Name(args[1]).Normalize(),
			}, nil
		case SubstringMatch:
			return &substringRCodeRule{
				newRCodeRuleBase(nextAction, old, new),
				plugin.Name(args[1]).Normalize(),
			}, nil
		case RegexMatch:
			regexPattern, err := regexp.Compile(args[1])
			if err != nil {
				return nil, fmt.Errorf("invalid regex pattern in a rcode rule: %s", args[1])
			}
			return &regexRCodeRule{
				newRCodeRuleBase(nextAction, old, new),
				regexPattern,
			}, nil
		default:
			return nil, fmt.Errorf("rcode rule supports only exact, prefix, suffix, substring, and regex name matching")
		}
	}
	if len(args) > 4 {
		return nil, fmt.Errorf("many few arguments for a rcode rule")
	}
	return &exactRCodeRule{
		newRCodeRuleBase(nextAction, old, new),
		plugin.Name(args[0]).Normalize(),
	}, nil
}

// validRCode returns true if v is valid RCode value.
func isValidRCode(v string) (int, bool) {
	i, err := strconv.ParseUint(v, 10, 32)
	// try parsing integer based rcode
	if err == nil && i <= 23 {
		return int(i), true
	}

	if RCodeInt, ok := dns.StringToRcode[strings.ToUpper(v)]; ok {
		return RCodeInt, true
	}
	return 0, false
}
