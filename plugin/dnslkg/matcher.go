package dnslkg

import (
	"fmt"
	"strings"
)

// rule is a single include/exclude selector built from a wildcard domain
// pattern.
//
// suffix holds the fixed (non-wildcard) labels in reverse order (TLD first),
// e.g. "*.example.com" -> ["com", "example"] with wildcard=true, and the exact
// pattern "example.com" -> ["com", "example"] with wildcard=false.
type rule struct {
	suffix   []string
	wildcard bool // leftmost label is "*"
	include  bool // true=include, false=exclude
	score    int  // number of fixed labels (len(suffix)); matching specificity
}

// nameMatcher decides whether a query name is tracked, resolving overlapping
// include/exclude rules with most-specific-wins semantics (order independent):
//
//   - more matched labels wins;
//   - at equal depth an exact pattern beats a wildcard;
//   - on a true tie, exclude wins (fail-safe).
//
// If any include rule exists, a name matching no rule is not tracked
// (allow-list); with only exclude rules, an unmatched name is tracked
// (deny-list). With no rules at all, every name is tracked.
type nameMatcher struct {
	rules      []rule
	hasInclude bool
}

// newNameMatcher builds a matcher from the given rules.
func newNameMatcher(rules []rule) *nameMatcher {
	m := &nameMatcher{rules: rules}
	for i := range rules {
		if rules[i].include {
			m.hasInclude = true
			break
		}
	}
	return m
}

// parseRule turns a wildcard domain pattern into a rule.
//
// Accepted forms:
//
//	example.com     exact match of the apex only
//	*.example.com   any subdomain (one or more extra labels), not the apex
//	*               every name
//
// The "*" wildcard is only valid as the leftmost label.
func parseRule(pattern string, include bool) (rule, error) {
	p := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(pattern), "."))
	if p == "" {
		return rule{}, fmt.Errorf("empty domain pattern")
	}

	labels := strings.Split(p, ".")
	wildcard := false
	for i, l := range labels {
		if l == "*" {
			if i != 0 {
				return rule{}, fmt.Errorf("%q: %q wildcard is only allowed as the leftmost label", pattern, "*")
			}
			wildcard = true
			continue
		}
		if l == "" {
			return rule{}, fmt.Errorf("%q: empty label", pattern)
		}
		if strings.Contains(l, "*") {
			return rule{}, fmt.Errorf("%q: %q is only allowed as a whole label", pattern, "*")
		}
	}

	// Fixed labels are everything except a leading "*".
	fixed := labels
	if wildcard {
		fixed = labels[1:]
	}
	suffix := reverse(fixed)

	return rule{suffix: suffix, wildcard: wildcard, include: include, score: len(suffix)}, nil
}

// tracked reports whether qname (a lower-cased FQDN, e.g. "www.example.org.")
// should be handled by the plugin.
func (m *nameMatcher) tracked(qname string) bool {
	if len(m.rules) == 0 {
		return true
	}
	q := splitReverse(qname)

	var best *rule
	for i := range m.rules {
		r := &m.rules[i]
		if !r.matches(q) {
			continue
		}
		if best == nil || moreSpecific(r, best) {
			best = r
		}
	}
	if best == nil {
		return !m.hasInclude
	}
	return best.include
}

// matches reports whether the reversed query labels q satisfy this rule.
func (r *rule) matches(q []string) bool {
	if r.wildcard {
		// A wildcard requires at least one extra label beyond the fixed suffix.
		if len(q) <= len(r.suffix) {
			return false
		}
	} else if len(q) != len(r.suffix) {
		return false
	}
	for i, l := range r.suffix {
		if q[i] != l {
			return false
		}
	}
	return true
}

// moreSpecific reports whether candidate c should win over the current best b.
func moreSpecific(c, b *rule) bool {
	if c.score != b.score {
		return c.score > b.score
	}
	if c.wildcard != b.wildcard {
		return !c.wildcard // exact beats wildcard at equal depth
	}
	if c.include != b.include {
		return !c.include // exclude beats include on a true tie
	}
	return false
}

// splitReverse lowercases qname, drops the trailing dot and returns its labels
// reversed (TLD first). The root name yields an empty slice.
func splitReverse(qname string) []string {
	n := strings.TrimSuffix(strings.ToLower(qname), ".")
	if n == "" {
		return nil
	}
	return reverse(strings.Split(n, "."))
}

func reverse(in []string) []string {
	out := make([]string, len(in))
	for i, l := range in {
		out[len(in)-1-i] = l
	}
	return out
}
