package rewrite

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"
)

type exactNameRule struct {
	NextAction string
	From       string
	To         string
	ResponseRule
}

type prefixNameRule struct {
	NextAction  string
	Prefix      string
	Replacement string
	ResponseRule
}

type suffixNameRule struct {
	NextAction  string
	Suffix      string
	Replacement string
	ResponseRule
}

type substringNameRule struct {
	NextAction  string
	Substring   string
	Replacement string
	ResponseRule
}

type regexNameRule struct {
	NextAction  string
	Pattern     *regexp.Regexp
	Replacement string
	ResponseRule
}

const (
	// ExactMatch matches only on exact match of the name in the question section of a request
	ExactMatch = "exact"
	// PrefixMatch matches when the name begins with the matching string
	PrefixMatch = "prefix"
	// SuffixMatch matches when the name ends with the matching string
	SuffixMatch = "suffix"
	// SubstringMatch matches on partial match of the name in the question section of a request
	SubstringMatch = "substring"
	// RegexMatch matches when the name in the question section of a request matches a regular expression
	RegexMatch = "regex"
)

// Rewrite rewrites the current request based upon exact match of the name
// in the question section of the request.
func (rule *exactNameRule) Rewrite(ctx context.Context, state request.Request) Result {
	if rule.From == state.Name() {
		state.Req.Question[0].Name = rule.To
		return RewriteDone
	}
	return RewriteIgnored
}

// Rewrite rewrites the current request when the name begins with the matching string.
func (rule *prefixNameRule) Rewrite(ctx context.Context, state request.Request) Result {
	if strings.HasPrefix(state.Name(), rule.Prefix) {
		state.Req.Question[0].Name = rule.Replacement + strings.TrimPrefix(state.Name(), rule.Prefix)
		return RewriteDone
	}
	return RewriteIgnored
}

// Rewrite rewrites the current request when the name ends with the matching string.
func (rule *suffixNameRule) Rewrite(ctx context.Context, state request.Request) Result {
	if strings.HasSuffix(state.Name(), rule.Suffix) {
		state.Req.Question[0].Name = strings.TrimSuffix(state.Name(), rule.Suffix) + rule.Replacement
		return RewriteDone
	}
	return RewriteIgnored
}

// Rewrite rewrites the current request based upon partial match of the
// name in the question section of the request.
func (rule *substringNameRule) Rewrite(ctx context.Context, state request.Request) Result {
	if strings.Contains(state.Name(), rule.Substring) {
		state.Req.Question[0].Name = strings.Replace(state.Name(), rule.Substring, rule.Replacement, -1)
		return RewriteDone
	}
	return RewriteIgnored
}

// Rewrite rewrites the current request when the name in the question
// section of the request matches a regular expression.
func (rule *regexNameRule) Rewrite(ctx context.Context, state request.Request) Result {
	regexGroups := rule.Pattern.FindStringSubmatch(state.Name())
	if len(regexGroups) == 0 {
		return RewriteIgnored
	}
	s := rule.Replacement
	for groupIndex, groupValue := range regexGroups {
		groupIndexStr := "{" + strconv.Itoa(groupIndex) + "}"
		s = strings.Replace(s, groupIndexStr, groupValue, -1)
	}
	state.Req.Question[0].Name = s
	return RewriteDone
}

// newNameRule creates a name matching rule based on exact, partial, or regex match
func newNameRule(nextAction string, args ...string) (Rule, error) {
	// argsIdx is advanced as we parse through args
	argsIdx := 0
	if len(args) < 2 {
		return nil, fmt.Errorf("too few arguments for a name rule")
	}
	var matchType string
	if len(args) == 2 {
		matchType = "exact"
	}
	if len(args) >= 3 {
		matchType = strings.ToLower(args[0])
		argsIdx++
	}

	// A lot of rules have from/to pairs in comment, let's refactor out parsing them
	from, to := "", ""
	var fromRegex *regexp.Regexp

	switch matchType {
	case ExactMatch, PrefixMatch, SuffixMatch, SubstringMatch:
		if len(args[argsIdx:]) < 2 {
			return nil, fmt.Errorf("%v name rule must have two arguments after name", matchType)
		}
		from = plugin.Name(args[argsIdx]).Normalize()
		to = plugin.Name(args[argsIdx+1]).Normalize()
		argsIdx += 2
	case RegexMatch:
		// no normalization on 'from' for this case
		from = args[argsIdx]
		to = plugin.Name(args[argsIdx+1]).Normalize()
		argsIdx += 2
		var err error
		fromRegex, err = isValidRegexPattern(from, to)
		if err != nil {
			return nil, err
		}
	}

	// Multiple rules also want from/to dot normalization
	switch matchType {
	case ExactMatch, SuffixMatch:
		if !hasClosingDot(from) {
			from = from + "."
		}
		if !hasClosingDot(to) {
			to = to + "."
		}
	}

	// Most rule types allow answer rewriting too
	respRule := ResponseRule{}

	switch matchType {
	case ExactMatch:
		// no answer rewrite allowed, it just happens automatically

	case PrefixMatch, SuffixMatch, SubstringMatch, RegexMatch:
		var err error
		respRule, err = parseRespRule(args, &argsIdx)
		if err != nil {
			return nil, err
		}
		// once we've parsed all this, no more trailing stuff is allowed
		if len(args[argsIdx:]) != 0 {
			return nil, fmt.Errorf("unexpected trailing arguments for name %v rule: %+v", matchType, args[argsIdx:])
		}
	}
	// Ideally we'd add 'if len(args[argsIdx:]) != 0 , "error: exact name rule must have exactly two arguments"
	// But for backwards compatibility with the previous parser, we don't
	// and now construct the actual rule
	switch matchType {
	case ExactMatch:
		// hack: use a regex to rewrite back; this is how it was previously done,
		// but really we probably want a ResponseRule that's just exact string
		// substitution
		respRuleRegex, err := isValidRegexPattern(from, to)
		if err != nil {
			return nil, fmt.Errorf("could not construct name response rule for 'exact': %v", err)
		}
		return &exactNameRule{
			nextAction,
			from,
			to,
			ResponseRule{
				Active:      true,
				Type:        ResponseRuleTypeName,
				Pattern:     respRuleRegex,
				Replacement: from,
			},
		}, nil
	case PrefixMatch:
		return &prefixNameRule{
			nextAction,
			from,
			to,
			respRule,
		}, nil
	case SuffixMatch:
		return &suffixNameRule{
			nextAction,
			from,
			to,
			respRule,
		}, nil
	case SubstringMatch:
		return &substringNameRule{
			nextAction,
			from,
			to,
			respRule,
		}, nil
	case RegexMatch:
		return &regexNameRule{
			nextAction,
			fromRegex,
			to,
			respRule,
		}, nil
	default:
		return nil, fmt.Errorf("name rule supports only exact, prefix, suffix, substring, and regex name matching, received: %s", matchType)
	}
}

// parseRespRule parses the response rule out of a rule
// That is to say, given 'answer name x y', it parses out a response rule.
// It understands the format 'answer name x y' and 'answer question'
// It assumes the arguments passed to it are already trimmed down to contain
// only the answer portion.
func parseRespRule(args []string, argsIdx *int) (ResponseRule, error) {
	if len(args[*argsIdx:]) == 0 {
		return ResponseRule{}, nil
	}
	typ := args[*argsIdx]
	*argsIdx++
	if typ != "answer" {
		return ResponseRule{}, fmt.Errorf("response rules must be of type 'answer'; got %v", typ)
	}
	if len(args[*argsIdx:]) == 0 {
		return ResponseRule{}, fmt.Errorf("answer rule must have a type of 'name' or 'question', was blank")
	}
	respType := args[*argsIdx]
	*argsIdx++

	switch respType {
	case ResponseRuleTypeName:
		if len(args[*argsIdx:]) < 2 {
			return ResponseRule{}, fmt.Errorf("answer rule of type 'name' must have at least two arguments to 'name'; got %d(%+v) instead", len(args[*argsIdx:]), args[*argsIdx:])
		}
		rewriteAnswerFrom := args[*argsIdx]
		rewriteAnswerTo := plugin.Name(args[*argsIdx+1]).Normalize()
		*argsIdx += 2
		rewriteAnswerFromPattern, err := isValidRegexPattern(rewriteAnswerFrom, rewriteAnswerTo)
		if err != nil {
			return ResponseRule{}, err
		}
		return ResponseRule{
			Active:      true,
			Type:        ResponseRuleTypeName,
			Pattern:     rewriteAnswerFromPattern,
			Replacement: rewriteAnswerTo,
		}, nil
	case ResponseRuleTypeQuestion:
		return ResponseRule{
			Active: true,
			Type:   ResponseRuleTypeQuestion,
		}, nil
	default:
		return ResponseRule{}, fmt.Errorf("unexpected answer rule type %q; only 'name' and 'question' are supported", respType)
	}
}

// Mode returns the processing nextAction
func (rule *exactNameRule) Mode() string     { return rule.NextAction }
func (rule *prefixNameRule) Mode() string    { return rule.NextAction }
func (rule *suffixNameRule) Mode() string    { return rule.NextAction }
func (rule *substringNameRule) Mode() string { return rule.NextAction }
func (rule *regexNameRule) Mode() string     { return rule.NextAction }

// GetResponseRule return a rule to rewrite the response with.
func (rule *exactNameRule) GetResponseRule() ResponseRule { return rule.ResponseRule }

// GetResponseRule return a rule to rewrite the response with.
func (rule *prefixNameRule) GetResponseRule() ResponseRule { return rule.ResponseRule }

// GetResponseRule return a rule to rewrite the response with.
func (rule *suffixNameRule) GetResponseRule() ResponseRule { return rule.ResponseRule }

// GetResponseRule return a rule to rewrite the response with.
func (rule *substringNameRule) GetResponseRule() ResponseRule { return rule.ResponseRule }

// GetResponseRule return a rule to rewrite the response with.
func (rule *regexNameRule) GetResponseRule() ResponseRule { return rule.ResponseRule }

// hasClosingDot return true if s has a closing dot at the end.
func hasClosingDot(s string) bool {
	return strings.HasSuffix(s, ".")
}

// getSubExprUsage return the number of subexpressions used in s.
func getSubExprUsage(s string) int {
	subExprUsage := 0
	for i := 0; i <= 100; i++ {
		if strings.Contains(s, "{"+strconv.Itoa(i)+"}") {
			subExprUsage++
		}
	}
	return subExprUsage
}

// isValidRegexPattern return a regular expression for pattern matching or errors, if any.
func isValidRegexPattern(rewriteFrom, rewriteTo string) (*regexp.Regexp, error) {
	rewriteFromPattern, err := regexp.Compile(rewriteFrom)
	if err != nil {
		return nil, fmt.Errorf("invalid regex matching pattern: %s", rewriteFrom)
	}
	if getSubExprUsage(rewriteTo) > rewriteFromPattern.NumSubexp() {
		return nil, fmt.Errorf("the rewrite regex pattern (%s) uses more subexpressions than its corresponding matching regex pattern (%s)", rewriteTo, rewriteFrom)
	}
	return rewriteFromPattern, nil
}
