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

type nameRule struct {
	nextAction   string
	responseRule ResponseRule

	rewriter
}

// rewriter is an internal interface that handles the specific rewrite behavior
// of each rewrite rule type
type rewriter interface {
	Rewrite(ctx context.Context, state request.Request) Result
}

type exactRewriter struct {
	From string
	To   string
}

type prefixRewriter struct {
	Prefix      string
	Replacement string
}

type suffixRewriter struct {
	Suffix      string
	Replacement string
}

type substringRewriter struct {
	Substring   string
	Replacement string
}

type regexRewriter struct {
	Pattern     *regexp.Regexp
	Replacement string
}

// Rewrite rewrites the current request based upon exact match of the name
// in the question section of the request.
func (rule exactRewriter) Rewrite(ctx context.Context, state request.Request) Result {
	if rule.From == state.Name() {
		state.Req.Question[0].Name = rule.To
		return RewriteDone
	}
	return RewriteIgnored
}

// Rewrite rewrites the current request when the name begins with the matching string.
func (rule prefixRewriter) Rewrite(ctx context.Context, state request.Request) Result {
	if strings.HasPrefix(state.Name(), rule.Prefix) {
		state.Req.Question[0].Name = rule.Replacement + strings.TrimPrefix(state.Name(), rule.Prefix)
		return RewriteDone
	}
	return RewriteIgnored
}

// Rewrite rewrites the current request when the name ends with the matching string.
func (rule suffixRewriter) Rewrite(ctx context.Context, state request.Request) Result {
	if strings.HasSuffix(state.Name(), rule.Suffix) {
		state.Req.Question[0].Name = strings.TrimSuffix(state.Name(), rule.Suffix) + rule.Replacement
		return RewriteDone
	}
	return RewriteIgnored
}

// Rewrite rewrites the current request based upon partial match of the
// name in the question section of the request.
func (rule substringRewriter) Rewrite(ctx context.Context, state request.Request) Result {
	if strings.Contains(state.Name(), rule.Substring) {
		state.Req.Question[0].Name = strings.Replace(state.Name(), rule.Substring, rule.Replacement, -1)
		return RewriteDone
	}
	return RewriteIgnored
}

// Rewrite rewrites the current request when the name in the question
// section of the request matches a regular expression.
func (rule regexRewriter) Rewrite(ctx context.Context, state request.Request) Result {
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
		// backwards compatibility for 'name X Y' vs 'name exact X Y'
		matchType = "exact"
	}
	if len(args) >= 3 {
		matchType = strings.ToLower(args[0])
		argsIdx++
	}

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

	respRule, err := parseRespRule(args, &argsIdx)
	if err != nil {
		return nil, err
	}
	// once we've parsed all this, no more trailing stuff is allowed
	if len(args[argsIdx:]) != 0 {
		return nil, fmt.Errorf("unexpected trailing arguments for name %v rule: %+v", matchType, args[argsIdx:])
	}
	// and now construct the actual rule
	switch matchType {
	case ExactMatch:
		return &nameRule{
			nextAction,
			respRule,
			exactRewriter{
				from,
				to,
			},
		}, nil
	case PrefixMatch:
		return &nameRule{
			nextAction,
			respRule,
			prefixRewriter{
				from,
				to,
			},
		}, nil
	case SuffixMatch:
		return &nameRule{
			nextAction,
			respRule,
			suffixRewriter{
				from,
				to,
			},
		}, nil
	case SubstringMatch:
		return &nameRule{
			nextAction,
			respRule,
			substringRewriter{
				from,
				to,
			},
		}, nil
	case RegexMatch:
		return &nameRule{
			nextAction,
			respRule,
			regexRewriter{
				fromRegex,
				to,
			},
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
		// If no response rule is set, enable the default response rule.
		return ResponseRule{}, nil
	}
	typ := args[*argsIdx]
	*argsIdx++
	if typ != "answer" {
		return ResponseRule{}, fmt.Errorf("response rules must be of type 'answer'; got %v", typ)
	}
	if len(args[*argsIdx:]) == 0 {
		return ResponseRule{}, fmt.Errorf("answer rule must have a type of 'name', was blank")
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
			Type:        ResponseRuleTypeName,
			Pattern:     rewriteAnswerFromPattern,
			Replacement: rewriteAnswerTo,
		}, nil
	default:
		return ResponseRule{}, fmt.Errorf("unexpected answer rule type %q; only 'name' is supported", respType)
	}
}

// Mode returns the processing nextAction
func (rule *nameRule) Mode() string { return rule.nextAction }

// GetResponseRule return a rule to rewrite the response with.
func (rule *nameRule) GetResponseRule() ResponseRule { return rule.responseRule }

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
