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
}

type suffixNameRule struct {
	NextAction  string
	Suffix      string
	Replacement string
}

type substringNameRule struct {
	NextAction  string
	Substring   string
	Replacement string
}

type regexNameRule struct {
	NextAction    string
	Pattern       *regexp.Regexp
	Replacement   string
	ResponseRules []ResponseRule
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
	var matchType, rewriteQuestionFrom, rewriteQuestionTo string
	if len(args) < 2 {
		return nil, fmt.Errorf("too few arguments for a name rule")
	}
	if len(args) == 2 {
		matchType = "exact"
		rewriteQuestionFrom = plugin.Name(args[0]).Normalize()
		rewriteQuestionTo = plugin.Name(args[1]).Normalize()
	}
	if len(args) >= 3 {
		matchType = strings.ToLower(args[0])
		rewriteQuestionFrom = plugin.Name(args[1]).Normalize()
		rewriteQuestionTo = plugin.Name(args[2]).Normalize()
	}
	if matchType == RegexMatch {
		rewriteQuestionFrom = args[1]
		rewriteQuestionTo = args[2]
	}
	if matchType == ExactMatch || matchType == SuffixMatch {
		if !hasClosingDot(rewriteQuestionFrom) {
			rewriteQuestionFrom = rewriteQuestionFrom + "."
		}
		if !hasClosingDot(rewriteQuestionTo) {
			rewriteQuestionTo = rewriteQuestionTo + "."
		}
	}

	//if len(args) > 3 && len(args) != 7 {
	if len(args) > 3 && (len(args) - 3) % 4 != 0 {
		return nil, fmt.Errorf("response rewrites must consist only of a name rule with 3 arguments and one or more answer rules with 3 arguments each")
	}

	if len(args) < 7 {
		switch matchType {
		case ExactMatch:
			rewriteAnswerFromPattern, err := isValidRegexPattern(rewriteQuestionTo, rewriteQuestionFrom)
			if err != nil {
				return nil, err
			}
			return &exactNameRule{
				nextAction,
				rewriteQuestionFrom,
				rewriteQuestionTo,
				ResponseRule{
					Active:      true,
					Type:        "name",
					Pattern:     rewriteAnswerFromPattern,
					Replacement: rewriteQuestionFrom,
				},
			}, nil
		case PrefixMatch:
			return &prefixNameRule{
				nextAction,
				rewriteQuestionFrom,
				rewriteQuestionTo,
			}, nil
		case SuffixMatch:
			return &suffixNameRule{
				nextAction,
				rewriteQuestionFrom,
				rewriteQuestionTo,
			}, nil
		case SubstringMatch:
			return &substringNameRule{
				nextAction,
				rewriteQuestionFrom,
				rewriteQuestionTo,
			}, nil
		case RegexMatch:
			rewriteQuestionFromPattern, err := isValidRegexPattern(rewriteQuestionFrom, rewriteQuestionTo)
			if err != nil {
				return nil, err
			}
			rewriteQuestionTo := plugin.Name(args[2]).Normalize()
			return &regexNameRule{
				nextAction,
				rewriteQuestionFromPattern,
				rewriteQuestionTo,
				[]ResponseRule{{
					Type: "name",
				}},
			}, nil
		default:
			return nil, fmt.Errorf("name rule supports only exact, prefix, suffix, substring, and regex name matching, received: %s", matchType)
		}
	}
	//if len(args) == 7 {
	if (len(args) - 3) % 4 == 0 {
		if matchType == RegexMatch {
			rewriteQuestionFromPattern, err := isValidRegexPattern(rewriteQuestionFrom, rewriteQuestionTo)
			if err != nil {
				return nil, err
			}
			rewriteQuestionTo = plugin.Name(args[2]).Normalize()

			responseRuleCount := (len(args) - 3) / 4
			responseRules := make([]ResponseRule, responseRuleCount)
			for i := 0; i < responseRuleCount; i ++ {
				startIdx := 3 + (i * 4)
				responseRule, err := newResponseRule(args[startIdx:startIdx + 4])
				if err != nil {
					return nil, err
				}
				responseRules[i] = *responseRule
			}

			return &regexNameRule{
				nextAction,
				rewriteQuestionFromPattern,
				rewriteQuestionTo,
				responseRules,
			}, nil
		}
		return nil, fmt.Errorf("the rewrite of response is supported only for name regex rule")
	}
	return nil, fmt.Errorf("the rewrite rule is invalid: %s", args)
}

// newResponseRule creates a new "answer name" or "answer value" response rule.
func newResponseRule(args []string) (responseRule *ResponseRule, err error){
	if args[0] != "answer" {
		return nil, fmt.Errorf("exceeded the number of arguments for a regex name rule")
	}
	rewriteAnswerField := strings.ToLower(args[1])
	switch rewriteAnswerField {
	case "name":
	case "value":
	default:
		return nil, fmt.Errorf("exceeded the number of arguments for a regex name rule")
	}
	rewriteAnswerFrom := args[2]
	rewriteAnswerTo := args[3]
	rewriteAnswerFromPattern, err := isValidRegexPattern(rewriteAnswerFrom, rewriteAnswerTo)
	if err != nil {
		return nil, err
	}
	rewriteAnswerTo = plugin.Name(args[3]).Normalize()

	return &ResponseRule{
		Active:      true,
		Type:        rewriteAnswerField,
		Pattern:     rewriteAnswerFromPattern,
		Replacement: rewriteAnswerTo,
	}, nil
}

// Mode returns the processing nextAction
func (rule *exactNameRule) Mode() string     { return rule.NextAction }
func (rule *prefixNameRule) Mode() string    { return rule.NextAction }
func (rule *suffixNameRule) Mode() string    { return rule.NextAction }
func (rule *substringNameRule) Mode() string { return rule.NextAction }
func (rule *regexNameRule) Mode() string     { return rule.NextAction }

// GetResponseRules returns rules to rewrite the response with. Currently not implemented.
func (rule *exactNameRule) GetResponseRules() []ResponseRule { return []ResponseRule{rule.ResponseRule} }

// GetResponseRules returns rules to rewrite the response with. Currently not implemented.
func (rule *prefixNameRule) GetResponseRules() []ResponseRule { return []ResponseRule{} }

// GetResponseRules returns rules to rewrite the response with. Currently not implemented.
func (rule *suffixNameRule) GetResponseRules() []ResponseRule { return []ResponseRule{} }

// GetResponseRules returns rules to rewrite the response with. Currently not implemented.
func (rule *substringNameRule) GetResponseRules() []ResponseRule { return []ResponseRule{} }

// GetResponseRules returns rules to rewrite the response with.
func (rule *regexNameRule) GetResponseRules() []ResponseRule { return rule.ResponseRules }

// hasClosingDot returns true if s has a closing dot at the end.
func hasClosingDot(s string) bool {
	return strings.HasSuffix(s, ".")
}

// getSubExprUsage returns the number of subexpressions used in s.
func getSubExprUsage(s string) int {
	subExprUsage := 0
	for i := 0; i <= 100; i++ {
		if strings.Contains(s, "{"+strconv.Itoa(i)+"}") {
			subExprUsage++
		}
	}
	return subExprUsage
}

// isValidRegexPattern returns a regular expression for pattern matching or errors, if any.
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
