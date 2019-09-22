package rewrite

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"
	//"github.com/miekg/dns"
)

type ttlRule struct {
	NextAction string
	ResponseRule
	rewriter
}

type exactTTLRewrite struct {
	From string
}

type prefixTTLRewrite struct {
	Prefix string
}

type suffixTTLRewrite struct {
	Suffix string
}

type substringTTLRewrite struct {
	Substring string
}

type regexTTLRewrite struct {
	Pattern *regexp.Regexp
}

// Rewrite rewrites the current request based upon exact match of the name
// in the question section of the request.
func (rule exactTTLRewrite) Rewrite(ctx context.Context, state request.Request) Result {
	if rule.From == state.Name() {
		return RewriteDone
	}
	return RewriteIgnored
}

// Rewrite rewrites the current request when the name begins with the matching string.
func (rule prefixTTLRewrite) Rewrite(ctx context.Context, state request.Request) Result {
	if strings.HasPrefix(state.Name(), rule.Prefix) {
		return RewriteDone
	}
	return RewriteIgnored
}

// Rewrite rewrites the current request when the name ends with the matching string.
func (rule suffixTTLRewrite) Rewrite(ctx context.Context, state request.Request) Result {
	if strings.HasSuffix(state.Name(), rule.Suffix) {
		return RewriteDone
	}
	return RewriteIgnored
}

// Rewrite rewrites the current request based upon partial match of the
// name in the question section of the request.
func (rule substringTTLRewrite) Rewrite(ctx context.Context, state request.Request) Result {
	if strings.Contains(state.Name(), rule.Substring) {
		return RewriteDone
	}
	return RewriteIgnored
}

// Rewrite rewrites the current request when the name in the question
// section of the request matches a regular expression.
func (rule regexTTLRewrite) Rewrite(ctx context.Context, state request.Request) Result {
	regexGroups := rule.Pattern.FindStringSubmatch(state.Name())
	if len(regexGroups) == 0 {
		return RewriteIgnored
	}
	return RewriteDone
}

// newTTLRule creates a name matching rule based on exact, partial, or regex match
func newTTLRule(nextAction string, args ...string) (Rule, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("too few (%d) arguments for a ttl rule", len(args))
	}
	var typ string
	if len(args) == 2 {
		typ = ExactMatch
	} else if len(args) == 3 {
		typ = args[0]
		args = args[1:]
	} else {
		return nil, fmt.Errorf("a ttl rule must have 3 arguments (ttl [exact|prefix|suffix|substring|regex] STRING SECONDS), got %d arguments", len(args))
	}
	s := args[1]
	ttl, valid := isValidTTL(s)
	if !valid {
		return nil, fmt.Errorf("invalid TTL '%s' for a ttl rule", s)
	}
	resp := &ttlRule{
		NextAction: nextAction,
		ResponseRule: ResponseRule{
			Type: "ttl",
			TTL:  ttl,
		},
	}
	switch strings.ToLower(typ) {
	case ExactMatch:
		resp.rewriter = exactTTLRewrite{
			plugin.Name(args[0]).Normalize(),
		}
	case PrefixMatch:
		resp.rewriter = prefixTTLRewrite{
			plugin.Name(args[0]).Normalize(),
		}
	case SuffixMatch:
		resp.rewriter = suffixTTLRewrite{
			plugin.Name(args[0]).Normalize(),
		}
	case SubstringMatch:
		resp.rewriter = substringTTLRewrite{
			plugin.Name(args[0]).Normalize(),
		}
	case RegexMatch:
		regexPattern, err := regexp.Compile(args[0])
		if err != nil {
			return nil, fmt.Errorf("invalid regex pattern in a ttl rule: %s", args[1])
		}
		resp.rewriter = regexTTLRewrite{
			regexPattern,
		}
	default:
		return nil, fmt.Errorf("ttl rule supports only exact, prefix, suffix, substring, and regex name matching")
	}
	return resp, nil
}

// Mode returns the processing nextAction
func (rule *ttlRule) Mode() string { return rule.NextAction }

// GetResponseRule return a rule to rewrite the response with.
func (rule *ttlRule) GetResponseRule() ResponseRule {
	return rule.ResponseRule
}

// validTTL returns true if v is valid TTL value.
func isValidTTL(v string) (uint32, bool) {
	i, err := strconv.Atoi(v)
	if err != nil {
		return uint32(0), false
	}
	if i > 2147483647 {
		return uint32(0), false
	}
	if i < 0 {
		return uint32(0), false
	}
	return uint32(i), true
}
