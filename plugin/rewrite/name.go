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

// stringRewriter rewrites a string
type stringRewriter interface {
	rewriteString(src string) string
}

// regexStringRewriter can be used to rewrite strings by regex pattern.
// it contains all the information required to detect and execute a rewrite
// on a string.
type regexStringRewriter struct {
	pattern     *regexp.Regexp
	replacement string
}

var _ stringRewriter = &regexStringRewriter{}

func newStringRewriter(pattern *regexp.Regexp, replacement string) stringRewriter {
	return &regexStringRewriter{pattern, replacement}
}

func (r *regexStringRewriter) rewriteString(src string) string {
	regexGroups := r.pattern.FindStringSubmatch(src)
	if len(regexGroups) == 0 {
		return src
	}
	s := r.replacement
	for groupIndex, groupValue := range regexGroups {
		groupIndexStr := "{" + strconv.Itoa(groupIndex) + "}"
		s = strings.ReplaceAll(s, groupIndexStr, groupValue)
	}
	return s
}

// remapStringRewriter maps a dedicated string to another string
// it also maps a the domain of a sub domain.
type remapStringRewriter struct {
	orig        string
	replacement string
}

var _ stringRewriter = &remapStringRewriter{}

func newRemapStringRewriter(orig, replacement string) stringRewriter {
	return &remapStringRewriter{orig, replacement}
}

func (r *remapStringRewriter) rewriteString(src string) string {
	if src == r.orig {
		return r.replacement
	}
	if strings.HasSuffix(src, "."+r.orig) {
		return src[0:len(src)-len(r.orig)] + r.replacement
	}
	return src
}

// suffixStringRewriter maps a dedicated suffix string to another string
type suffixStringRewriter struct {
	suffix      string
	replacement string
}

var _ stringRewriter = &suffixStringRewriter{}

func newSuffixStringRewriter(orig, replacement string) stringRewriter {
	return &suffixStringRewriter{orig, replacement}
}

func (r *suffixStringRewriter) rewriteString(src string) string {
	if before, ok := strings.CutSuffix(src, r.suffix); ok {
		return before + r.replacement
	}
	return src
}

// nameRewriterResponseRule maps a record name according to a stringRewriter.
type nameRewriterResponseRule struct {
	stringRewriter
}

func (r *nameRewriterResponseRule) RewriteResponse(res *dns.Msg, rr dns.RR) {
	rr.Header().Name = r.rewriteString(rr.Header().Name)
}

// valueRewriterResponseRule maps a record value according to a stringRewriter.
type valueRewriterResponseRule struct {
	stringRewriter
}

func (r *valueRewriterResponseRule) RewriteResponse(res *dns.Msg, rr dns.RR) {
	value := getRecordValueForRewrite(rr)
	if value != "" {
		new := r.rewriteString(value)
		if new != value {
			setRewrittenRecordValue(rr, new)
		}
	}
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

	// AnswerMatch matches an answer rewrite
	AnswerMatch = "answer"
	// AutoMatch matches the auto name answer rewrite
	AutoMatch = "auto"
	// NameMatch matches the name answer rewrite
	NameMatch = "name"
	// ValueMatch matches the value answer rewrite
	ValueMatch = "value"
)

type nameRuleBase struct {
	nextAction  string
	auto        bool
	replacement string
	static      ResponseRules
}

func newNameRuleBase(nextAction string, auto bool, replacement string, staticResponses ResponseRules) nameRuleBase {
	return nameRuleBase{
		nextAction:  nextAction,
		auto:        auto,
		replacement: replacement,
		static:      staticResponses,
	}
}

// responseRuleFor create for auto mode dynamically response rewriters for name and value
// reverting the mapping done by the name rewrite rule, which can be found in the state.
func (rule *nameRuleBase) responseRuleFor(state request.Request) (ResponseRules, Result) {
	if !rule.auto {
		return rule.static, RewriteDone
	}

	rewriter := newRemapStringRewriter(state.Req.Question[0].Name, state.Name())
	rules := ResponseRules{
		&nameRewriterResponseRule{rewriter},
		&valueRewriterResponseRule{rewriter},
	}
	return append(rules, rule.static...), RewriteDone
}

// Mode returns the processing nextAction
func (rule *nameRuleBase) Mode() string { return rule.nextAction }

// exactNameRule rewrites the current request based upon exact match of the name
// in the question section of the request.
type exactNameRule struct {
	nameRuleBase
	from string
}

func newExactNameRule(nextAction string, orig, replacement string, answers ResponseRules) Rule {
	return &exactNameRule{
		newNameRuleBase(nextAction, true, replacement, answers),
		orig,
	}
}

func (rule *exactNameRule) Rewrite(ctx context.Context, state request.Request) (ResponseRules, Result) {
	if rule.from == state.Name() {
		state.Req.Question[0].Name = rule.replacement
		return rule.responseRuleFor(state)
	}
	return nil, RewriteIgnored
}

// prefixNameRule rewrites the current request when the name begins with the matching string.
type prefixNameRule struct {
	nameRuleBase
	prefix string
}

func newPrefixNameRule(nextAction string, auto bool, prefix, replacement string, answers ResponseRules) Rule {
	return &prefixNameRule{
		newNameRuleBase(nextAction, auto, replacement, answers),
		prefix,
	}
}

func (rule *prefixNameRule) Rewrite(ctx context.Context, state request.Request) (ResponseRules, Result) {
	if after, ok := strings.CutPrefix(state.Name(), rule.prefix); ok {
		state.Req.Question[0].Name = rule.replacement + after
		return rule.responseRuleFor(state)
	}
	return nil, RewriteIgnored
}

// suffixNameRule rewrites the current request when the name ends with the matching string.
type suffixNameRule struct {
	nameRuleBase
	suffix string
}

func newSuffixNameRule(nextAction string, auto bool, suffix, replacement string, answers ResponseRules) Rule {
	var rules ResponseRules
	if auto {
		// for a suffix rewriter better standard response rewrites can be done
		// just by using the original suffix/replacement in the opposite order
		rewriter := newSuffixStringRewriter(replacement, suffix)
		rules = ResponseRules{
			&nameRewriterResponseRule{rewriter},
			&valueRewriterResponseRule{rewriter},
		}
	}
	return &suffixNameRule{
		newNameRuleBase(nextAction, false, replacement, append(rules, answers...)),
		suffix,
	}
}

func (rule *suffixNameRule) Rewrite(ctx context.Context, state request.Request) (ResponseRules, Result) {
	if before, ok := strings.CutSuffix(state.Name(), rule.suffix); ok {
		state.Req.Question[0].Name = before + rule.replacement
		return rule.responseRuleFor(state)
	}
	return nil, RewriteIgnored
}

// substringNameRule rewrites the current request based upon partial match of the
// name in the question section of the request.
type substringNameRule struct {
	nameRuleBase
	substring string
}

func newSubstringNameRule(nextAction string, auto bool, substring, replacement string, answers ResponseRules) Rule {
	return &substringNameRule{
		newNameRuleBase(nextAction, auto, replacement, answers),
		substring,
	}
}

func (rule *substringNameRule) Rewrite(ctx context.Context, state request.Request) (ResponseRules, Result) {
	if strings.Contains(state.Name(), rule.substring) {
		state.Req.Question[0].Name = strings.ReplaceAll(state.Name(), rule.substring, rule.replacement)
		return rule.responseRuleFor(state)
	}
	return nil, RewriteIgnored
}

// regexNameRule rewrites the current request when the name in the question
// section of the request matches a regular expression.
type regexNameRule struct {
	nameRuleBase
	pattern *regexp.Regexp
}

func newRegexNameRule(nextAction string, auto bool, pattern *regexp.Regexp, replacement string, answers ResponseRules) Rule {
	return &regexNameRule{
		newNameRuleBase(nextAction, auto, replacement, answers),
		pattern,
	}
}

func (rule *regexNameRule) Rewrite(ctx context.Context, state request.Request) (ResponseRules, Result) {
	regexGroups := rule.pattern.FindStringSubmatch(state.Name())
	if len(regexGroups) == 0 {
		return nil, RewriteIgnored
	}
	s := rule.replacement
	for groupIndex, groupValue := range regexGroups {
		groupIndexStr := "{" + strconv.Itoa(groupIndex) + "}"
		s = strings.ReplaceAll(s, groupIndexStr, groupValue)
	}
	state.Req.Question[0].Name = s
	return rule.responseRuleFor(state)
}

// newNameRule creates a name matching rule based on exact, partial, or regex match
func newNameRule(nextAction string, args ...string) (Rule, error) {
	var matchType, rewriteQuestionFrom, rewriteQuestionTo string
	if len(args) < 2 {
		return nil, fmt.Errorf("too few arguments for a name rule")
	}
	if len(args) == 2 {
		matchType = ExactMatch
		rewriteQuestionFrom = plugin.Name(args[0]).Normalize()
		rewriteQuestionTo = plugin.Name(args[1]).Normalize()
	}
	if len(args) >= 3 {
		matchType = strings.ToLower(args[0])
		if matchType == RegexMatch {
			rewriteQuestionFrom = args[1]
			rewriteQuestionTo = args[2]
		} else {
			rewriteQuestionFrom = plugin.Name(args[1]).Normalize()
			rewriteQuestionTo = plugin.Name(args[2]).Normalize()
		}
	}
	if matchType == ExactMatch || matchType == SuffixMatch {
		if !hasClosingDot(rewriteQuestionFrom) {
			rewriteQuestionFrom = rewriteQuestionFrom + "."
		}
		if !hasClosingDot(rewriteQuestionTo) {
			rewriteQuestionTo = rewriteQuestionTo + "."
		}
	}

	var err error
	var answers ResponseRules
	auto := false
	if len(args) > 3 {
		auto, answers, err = parseAnswerRules(matchType, args[3:])
		if err != nil {
			return nil, err
		}
	}

	switch matchType {
	case ExactMatch:
		if _, err := isValidRegexPattern(rewriteQuestionTo, rewriteQuestionFrom); err != nil {
			return nil, err
		}
		return newExactNameRule(nextAction, rewriteQuestionFrom, rewriteQuestionTo, answers), nil
	case PrefixMatch:
		return newPrefixNameRule(nextAction, auto, rewriteQuestionFrom, rewriteQuestionTo, answers), nil
	case SuffixMatch:
		return newSuffixNameRule(nextAction, auto, rewriteQuestionFrom, rewriteQuestionTo, answers), nil
	case SubstringMatch:
		return newSubstringNameRule(nextAction, auto, rewriteQuestionFrom, rewriteQuestionTo, answers), nil
	case RegexMatch:
		rewriteQuestionFromPattern, err := isValidRegexPattern(rewriteQuestionFrom, rewriteQuestionTo)
		if err != nil {
			return nil, err
		}
		rewriteQuestionTo := plugin.Name(args[2]).Normalize()
		return newRegexNameRule(nextAction, auto, rewriteQuestionFromPattern, rewriteQuestionTo, answers), nil
	default:
		return nil, fmt.Errorf("name rule supports only exact, prefix, suffix, substring, and regex name matching, received: %s", matchType)
	}
}

func parseAnswerRules(name string, args []string) (auto bool, rules ResponseRules, err error) {
	auto = false
	arg := 0
	nameRules := 0
	last := ""
	if len(args) < 2 {
		return false, nil, fmt.Errorf("invalid arguments for %s rule", name)
	}
	for arg < len(args) {
		if last == "" && args[arg] != AnswerMatch {
			if last == "" {
				return false, nil, fmt.Errorf("exceeded the number of arguments for a non-answer rule argument for %s rule", name)
			}
			return false, nil, fmt.Errorf("exceeded the number of arguments for %s answer rule for %s rule", last, name)
		}
		if args[arg] == AnswerMatch {
			arg++
		}
		if len(args)-arg == 0 {
			return false, nil, fmt.Errorf("type missing for answer rule for %s rule", name)
		}
		last = args[arg]
		arg++
		switch last {
		case AutoMatch:
			auto = true
			continue
		case NameMatch:
			if len(args)-arg < 2 {
				return false, nil, fmt.Errorf("%s answer rule for %s rule: 2 arguments required", last, name)
			}
			rewriteAnswerFrom := args[arg]
			rewriteAnswerTo := args[arg+1]
			rewriteAnswerFromPattern, err := isValidRegexPattern(rewriteAnswerFrom, rewriteAnswerTo)
			rewriteAnswerTo = plugin.Name(rewriteAnswerTo).Normalize()
			if err != nil {
				return false, nil, fmt.Errorf("%s answer rule for %s rule: %s", last, name, err)
			}
			rules = append(rules, &nameRewriterResponseRule{newStringRewriter(rewriteAnswerFromPattern, rewriteAnswerTo)})
			arg += 2
			nameRules++
		case ValueMatch:
			if len(args)-arg < 2 {
				return false, nil, fmt.Errorf("%s answer rule for %s rule: 2 arguments required", last, name)
			}
			rewriteAnswerFrom := args[arg]
			rewriteAnswerTo := args[arg+1]
			rewriteAnswerFromPattern, err := isValidRegexPattern(rewriteAnswerFrom, rewriteAnswerTo)
			rewriteAnswerTo = plugin.Name(rewriteAnswerTo).Normalize()
			if err != nil {
				return false, nil, fmt.Errorf("%s answer rule for %s rule: %s", last, name, err)
			}
			rules = append(rules, &valueRewriterResponseRule{newStringRewriter(rewriteAnswerFromPattern, rewriteAnswerTo)})
			arg += 2
		default:
			return false, nil, fmt.Errorf("invalid type %q for answer rule for %s rule", last, name)
		}
	}

	if auto && nameRules > 0 {
		return false, nil, fmt.Errorf("auto name answer rule cannot be combined with explicit name anwer rules")
	}
	return auto, rules, nil
}

// hasClosingDot returns true if s has a closing dot at the end.
func hasClosingDot(s string) bool {
	return strings.HasSuffix(s, ".")
}

// getSubExprUsage returns the number of subexpressions used in s.
func getSubExprUsage(s string) int {
	subExprUsage := 0
	for i := range 101 {
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
