package rewrite

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/coredns/coredns/plugin/metadata"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// metadataNameRule expands a replacement compiled at setup time. Values from
// captures and metadata are inserted literally, without interpreting them again.
type metadataNameRule struct {
	nameRuleBase
	pattern *regexp.Regexp
	parts   []nameReplacementPart
}

type nameReplacementPart struct {
	literal string
	label   string
	group   int // -1 for literals and metadata labels
}

func newMetadataNameRule(nextAction string, auto bool, from, to string, answers ResponseRules) (Rule, error) {
	pattern, err := isValidRegexPattern(from, "")
	if err != nil {
		return nil, err
	}
	parts, err := parseNameReplacement(dns.Fqdn(to), pattern.NumSubexp())
	if err != nil {
		return nil, err
	}
	return &metadataNameRule{
		nameRuleBase: newNameRuleBase(nextAction, auto, "", answers),
		pattern:      pattern,
		parts:        parts,
	}, nil
}

func parseNameReplacement(replacement string, groups int) ([]nameReplacementPart, error) {
	var parts []nameReplacementPart
	for replacement != "" {
		before, rest, found := strings.Cut(replacement, "{")
		if strings.Contains(before, "}") {
			return nil, fmt.Errorf("unmatched closing brace in metadata name replacement")
		}
		if before != "" {
			parts = append(parts, nameReplacementPart{literal: strings.ToLower(before), group: -1})
		}
		if !found {
			break
		}
		label, after, closed := strings.Cut(rest, "}")
		if !closed {
			return nil, fmt.Errorf("unclosed placeholder in metadata name replacement")
		}
		part := nameReplacementPart{group: -1}
		if group, err := strconv.Atoi(label); err == nil {
			if group < 0 || group > groups {
				return nil, fmt.Errorf("capture group %s does not exist in name matching pattern", label)
			}
			part.group = group
		} else {
			if !metadata.IsLabel(label) || strings.ContainsAny(label, "{}") {
				return nil, fmt.Errorf("invalid metadata label %q in name replacement", label)
			}
			part.label = label
		}
		parts = append(parts, part)
		replacement = after
	}
	return parts, nil
}

func (rule *metadataNameRule) Rewrite(ctx context.Context, state request.Request) (ResponseRules, Result) {
	groups := rule.pattern.FindStringSubmatch(state.Name())
	if len(groups) == 0 {
		return nil, RewriteIgnored
	}
	var replacement strings.Builder
	for _, part := range rule.parts {
		switch {
		case part.group >= 0:
			replacement.WriteString(groups[part.group])
		case part.label != "":
			value := metadata.ValueFunc(ctx, part.label)
			if value == nil {
				return nil, RewriteIgnored
			}
			text := value()
			if text == "" {
				return nil, RewriteIgnored
			}
			replacement.WriteString(strings.ToLower(text))
		default:
			replacement.WriteString(part.literal)
		}
	}
	state.Req.Question[0].Name = replacement.String()
	return rule.responseRuleFor(state)
}
