// SPDX-License-Identifier: Apache-2.0
// Copyright © 2026 infraBuilder SASU and contributors

package v1alpha1

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// This file implements the version constraint syntax of RECIPE-SPEC.md §9,
// exactly and only: exact OCI tags, wildcards (12.x, 12.4.x, *), caret
// (^1.4.0), tilde (~1.4.2), comparisons (>=2.0.0, >1.2, <=3, <4.0.0,
// =1.2.3, !=1.5.0), and conjunctions separated by whitespace and/or commas.
// Disjunction ("||") is intentionally not supported in v1alpha1 and is
// rejected. The grammar is implemented in-house rather than through a
// general-purpose constraint library because the specification is stricter
// than common libraries: no "||", no hyphen ranges, no "==" alias, no
// wildcard mixed into comparison operators, and no whitespace between an
// operator and its version.

// ociTagPattern is the set of valid OCI tags (§9.1).
var ociTagPattern = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9._-]{0,127}$`)

// maxVersionLength mirrors the maxLength of the version fields in the
// published JSON Schemas.
const maxVersionLength = 256

// Constraint is a parsed version field: either an exact tag, matched
// literally, or a constraint expression matched against tags that parse as
// Semantic Versioning 2.0.0 versions (§9). The zero value is not usable;
// always obtain a Constraint from ParseConstraint.
type Constraint struct {
	raw   string
	exact bool
	terms []constraintTerm
}

// term operators. The comparison operators are stored as written; opWildcard
// marks a wildcard term such as "12.x" or "*".
const opWildcard = "wildcard"

type constraintTerm struct {
	op  string // ">=", "<=", ">", "<", "=", "!=", "^", "~", or opWildcard
	ver partialVersion
}

// partialVersion is a version literal inside a constraint term. Comparison,
// caret, and tilde operators accept 1 to 3 numeric components; a wildcard
// term keeps only its fixed leading components (0 to 2 of them).
type partialVersion struct {
	comps []uint64
	pre   []preIdent // only permitted when all three components are given
}

// version is a fully parsed semver version (candidate tag). Build metadata
// is ignored for ordering per semver and is therefore not retained.
type version struct {
	major, minor, patch uint64
	pre                 []preIdent
}

func (v version) triple() [3]uint64 { return [3]uint64{v.major, v.minor, v.patch} }

type preIdent struct {
	str     string
	num     uint64
	numeric bool
}

// ParseConstraint parses the value of a version field: an ingredient
// version (§6.2), a retriever recipe version (§10.2). Following the
// disambiguation rules of §9.3, the value is a constraint expression if and
// only if it starts with one of "^ ~ > < = !", contains whitespace or a
// comma, or is a wildcard form ("12.x", "1.2.*", "*"); anything else is an
// exact tag, which must then be a valid OCI tag.
func ParseConstraint(s string) (*Constraint, error) {
	if s == "" {
		return nil, fmt.Errorf("version is empty")
	}
	if len(s) > maxVersionLength {
		return nil, fmt.Errorf("version exceeds %d characters", maxVersionLength)
	}
	if strings.Contains(s, "||") {
		return nil, fmt.Errorf("invalid constraint %q: disjunction (\"||\") is not supported in v1alpha1 (§9.2); use a single conjunction", s)
	}
	if !isConstraintExpr(s) {
		if !ociTagPattern.MatchString(s) {
			return nil, fmt.Errorf("%q is not a valid OCI tag: tags start with an alphanumeric or '_' and contain only alphanumerics, '.', '_', '-' (at most 128 characters, §9.1)", s)
		}
		return &Constraint{raw: s, exact: true}, nil
	}
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || r == ','
	})
	if len(fields) == 0 {
		return nil, fmt.Errorf("invalid constraint %q: no terms", s)
	}
	terms := make([]constraintTerm, 0, len(fields))
	for _, f := range fields {
		t, err := parseTerm(f)
		if err != nil {
			return nil, fmt.Errorf("invalid constraint %q: %w", s, err)
		}
		terms = append(terms, t)
	}
	return &Constraint{raw: s, terms: terms}, nil
}

// IsExact reports whether the parsed value is an exact tag (§9.1), as
// opposed to a constraint expression. Cooked recipes only accept exact tags
// (§8).
func (c *Constraint) IsExact() bool { return c.exact }

// String returns the value as written.
func (c *Constraint) String() string { return c.raw }

// Match reports whether the given tag satisfies the constraint.
//
// For an exact tag the comparison is literal (§9.1): "v1.2.3" does not
// match the exact tag "1.2.3". For a constraint expression, the tag is
// parsed as a Semantic Versioning 2.0.0 version after stripping one leading
// "v" (§9.2); tags that do not parse as semver never match. Pre-release
// versions match only when the expression itself mentions a pre-release on
// the same major.minor.patch triple, per the semver range convention (§9.2
// resolution rule 2).
func (c *Constraint) Match(tag string) bool {
	if c.exact {
		return tag == c.raw
	}
	v, ok := parseVersionTag(tag)
	if !ok {
		return false
	}
	if len(v.pre) > 0 && !c.mentionsPreRelease(v) {
		return false
	}
	for _, t := range c.terms {
		if !t.matches(v) {
			return false
		}
	}
	return true
}

// Resolve applies the resolution rules of §9.2 to a list of candidate tags
// and returns the selected tag.
//
// For a constraint expression: candidates that do not parse as semver are
// ignored, pre-releases are excluded unless mentioned by the expression,
// and the highest matching version wins. When several tags denote the same
// version (e.g. "1.2.3" and "v1.2.3"), the lexicographically smallest tag
// is returned, deterministically. For an exact tag, the tag itself is
// returned if present in the candidate list. Resolve fails when nothing
// matches: per §9.2 rule 5 it never falls back to another tag.
func (c *Constraint) Resolve(tags []string) (string, error) {
	if c.exact {
		for _, t := range tags {
			if t == c.raw {
				return t, nil
			}
		}
		return "", fmt.Errorf("no candidate tag equals exact tag %q", c.raw)
	}
	best := ""
	var bestV version
	for _, t := range tags {
		if !c.Match(t) {
			continue
		}
		v, _ := parseVersionTag(t)
		if best == "" {
			best, bestV = t, v
			continue
		}
		switch compareVersions(v, bestV) {
		case 1:
			best, bestV = t, v
		case 0:
			if t < best {
				best = t
			}
		}
	}
	if best == "" {
		return "", fmt.Errorf("no candidate tag matches constraint %q", c.raw)
	}
	return best, nil
}

// mentionsPreRelease reports whether any term carries a pre-release on the
// same major.minor.patch triple as v (§9.2 resolution rule 2).
func (c *Constraint) mentionsPreRelease(v version) bool {
	for _, t := range c.terms {
		if len(t.ver.pre) == 0 || len(t.ver.comps) != 3 {
			continue
		}
		if t.ver.comps[0] == v.major && t.ver.comps[1] == v.minor && t.ver.comps[2] == v.patch {
			return true
		}
	}
	return false
}

// isConstraintExpr implements the disambiguation rules of §9.3.
func isConstraintExpr(s string) bool {
	switch s[0] {
	case '^', '~', '>', '<', '=', '!':
		return true
	}
	for _, r := range s {
		if unicode.IsSpace(r) || r == ',' {
			return true
		}
	}
	return isWildcardForm(s)
}

// isWildcardForm reports whether s consists solely of dotted numeric
// components where at least one component is a placeholder ("x", "X", or
// "*"), or is exactly a placeholder (§9.3).
func isWildcardForm(s string) bool {
	hasPlaceholder := false
	for _, comp := range strings.Split(s, ".") {
		switch {
		case isPlaceholder(comp):
			hasPlaceholder = true
		case isDigits(comp):
		default:
			return false
		}
	}
	return hasPlaceholder
}

func isPlaceholder(s string) bool { return s == "x" || s == "X" || s == "*" }

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// comparison, caret, and tilde operators, longest first so that ">=" is not
// read as ">" followed by "=1.2.3".
var operators = []string{">=", "<=", "!=", ">", "<", "=", "^", "~"}

func parseTerm(t string) (constraintTerm, error) {
	if isWildcardForm(t) {
		return parseWildcardTerm(t)
	}
	if strings.HasPrefix(t, "==") {
		return constraintTerm{}, fmt.Errorf("term %q: use \"=\" for equality, not \"==\"", t)
	}
	for _, op := range operators {
		rest, found := strings.CutPrefix(t, op)
		if !found {
			continue
		}
		if rest == "" {
			return constraintTerm{}, fmt.Errorf("operator %q must be immediately followed by a version, with no whitespace in between", op)
		}
		if strings.ContainsAny(rest, "^~><=!") {
			return constraintTerm{}, fmt.Errorf("term %q: unexpected operator character after %q", t, op)
		}
		pv, err := parsePartialVersion(rest)
		if err != nil {
			return constraintTerm{}, fmt.Errorf("term %q: %w", t, err)
		}
		return constraintTerm{op: op, ver: pv}, nil
	}
	if strings.HasPrefix(t, "!") {
		return constraintTerm{}, fmt.Errorf("term %q: \"!\" is only valid as the \"!=\" operator", t)
	}
	return constraintTerm{}, fmt.Errorf("term %q is neither a wildcard, a caret, a tilde, nor a comparison; a bare version is only valid on its own, as an exact tag (§9.3) — use \"=%s\" for equality inside an expression", t, t)
}

// parseWildcardTerm parses a wildcard form ("*", "12.x", "12.4.x", "1.2.*")
// into its fixed leading components. Placeholders are only meaningful as a
// suffix: "1.x.3" has no defined meaning in §9.2 ("any version with the
// given fixed components") and is rejected.
func parseWildcardTerm(t string) (constraintTerm, error) {
	comps := strings.Split(t, ".")
	if len(comps) > 3 {
		return constraintTerm{}, fmt.Errorf("wildcard %q: a version has at most 3 components (major.minor.patch)", t)
	}
	fixed := make([]uint64, 0, 2)
	seenPlaceholder := false
	for _, comp := range comps {
		if isPlaceholder(comp) {
			seenPlaceholder = true
			continue
		}
		if seenPlaceholder {
			return constraintTerm{}, fmt.Errorf("wildcard %q: a numeric component cannot follow a placeholder; fixed components must come first (§9.2)", t)
		}
		n, err := parseNumericComponent(comp)
		if err != nil {
			return constraintTerm{}, fmt.Errorf("wildcard %q: %w", t, err)
		}
		fixed = append(fixed, n)
	}
	return constraintTerm{op: opWildcard, ver: partialVersion{comps: fixed}}, nil
}

// parsePartialVersion parses the version literal of a comparison, caret, or
// tilde term: 1 to 3 numeric components, with an optional pre-release only
// when all three components are present, and an optional leading "v"
// (ignored for matching, §9.2).
func parsePartialVersion(s string) (partialVersion, error) {
	if strings.Contains(s, "+") {
		return partialVersion{}, fmt.Errorf("build metadata (\"+\") is not allowed in a constraint: it is ignored for ordering per semver and cannot appear in an OCI tag")
	}
	s = strings.TrimPrefix(s, "v")
	core, preStr, hasPre := strings.Cut(s, "-")
	comps := strings.Split(core, ".")
	if len(comps) > 3 {
		return partialVersion{}, fmt.Errorf("version %q has more than 3 components (major.minor.patch)", s)
	}
	pv := partialVersion{comps: make([]uint64, 0, 3)}
	for _, comp := range comps {
		if isPlaceholder(comp) {
			return partialVersion{}, fmt.Errorf("wildcard component %q cannot be combined with an operator; use the wildcard form alone (e.g. \"1.x\") or a full version", comp)
		}
		n, err := parseNumericComponent(comp)
		if err != nil {
			return partialVersion{}, err
		}
		pv.comps = append(pv.comps, n)
	}
	if hasPre {
		if len(pv.comps) != 3 {
			return partialVersion{}, fmt.Errorf("pre-release %q requires a full major.minor.patch version", preStr)
		}
		pre, err := parsePreRelease(preStr)
		if err != nil {
			return partialVersion{}, err
		}
		pv.pre = pre
	}
	return pv, nil
}

func parseNumericComponent(s string) (uint64, error) {
	if !isDigits(s) {
		return 0, fmt.Errorf("component %q is not numeric", s)
	}
	if len(s) > 1 && s[0] == '0' {
		return 0, fmt.Errorf("component %q has a leading zero (invalid per semver)", s)
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("component %q: %w", s, err)
	}
	return n, nil
}

func parsePreRelease(s string) ([]preIdent, error) {
	if s == "" {
		return nil, fmt.Errorf("pre-release is empty")
	}
	idents := strings.Split(s, ".")
	pre := make([]preIdent, 0, len(idents))
	for _, id := range idents {
		if id == "" {
			return nil, fmt.Errorf("pre-release %q contains an empty identifier", s)
		}
		for _, r := range id {
			if !isPreReleaseRune(r) {
				return nil, fmt.Errorf("pre-release identifier %q: only alphanumerics and '-' are allowed", id)
			}
		}
		if isDigits(id) {
			if len(id) > 1 && id[0] == '0' {
				return nil, fmt.Errorf("numeric pre-release identifier %q has a leading zero (invalid per semver)", id)
			}
			n, err := strconv.ParseUint(id, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("pre-release identifier %q: %w", id, err)
			}
			pre = append(pre, preIdent{str: id, num: n, numeric: true})
			continue
		}
		pre = append(pre, preIdent{str: id})
	}
	return pre, nil
}

func isPreReleaseRune(r rune) bool {
	return r == '-' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// parseVersionTag parses a candidate tag as a full Semantic Versioning
// 2.0.0 version, after stripping one leading "v" (§9.2). Build metadata is
// accepted and ignored for ordering, per semver (it cannot occur in an OCI
// tag anyway, since '+' is not a legal tag character). Returns false for
// tags that are not semver: such tags are ignored by constraint matching
// (§9.2 resolution rule 1).
func parseVersionTag(tag string) (version, bool) {
	s := strings.TrimPrefix(tag, "v")
	s, _, _ = strings.Cut(s, "+") // rule 3: build metadata ignored
	core, preStr, hasPre := strings.Cut(s, "-")
	comps := strings.Split(core, ".")
	if len(comps) != 3 {
		return version{}, false
	}
	var nums [3]uint64
	for i, comp := range comps {
		n, err := parseNumericComponent(comp)
		if err != nil {
			return version{}, false
		}
		nums[i] = n
	}
	v := version{major: nums[0], minor: nums[1], patch: nums[2]}
	if hasPre {
		pre, err := parsePreRelease(preStr)
		if err != nil {
			return version{}, false
		}
		v.pre = pre
	}
	return v, true
}

// compareVersions implements semver 2.0.0 precedence (§11 of the semver
// specification): compare the numeric triple, then the pre-release
// identifiers (a version without pre-release is higher than the same
// version with one).
func compareVersions(a, b version) int {
	at, bt := a.triple(), b.triple()
	for i := range at {
		switch {
		case at[i] < bt[i]:
			return -1
		case at[i] > bt[i]:
			return 1
		}
	}
	switch {
	case len(a.pre) == 0 && len(b.pre) == 0:
		return 0
	case len(a.pre) == 0:
		return 1
	case len(b.pre) == 0:
		return -1
	}
	for i := 0; i < len(a.pre) && i < len(b.pre); i++ {
		if c := comparePreIdent(a.pre[i], b.pre[i]); c != 0 {
			return c
		}
	}
	switch {
	case len(a.pre) < len(b.pre):
		return -1
	case len(a.pre) > len(b.pre):
		return 1
	}
	return 0
}

func comparePreIdent(a, b preIdent) int {
	switch {
	case a.numeric && b.numeric:
		switch {
		case a.num < b.num:
			return -1
		case a.num > b.num:
			return 1
		}
		return 0
	case a.numeric: // numeric identifiers have lower precedence
		return -1
	case b.numeric:
		return 1
	}
	return strings.Compare(a.str, b.str)
}

// matches evaluates one term against a fully parsed version. Partial
// versions in comparison operators follow the standard semver range
// convention: the missing components designate the whole series, so
// ">1.2" means ">= 1.3.0", "<=3" means "< 4.0.0", "=1.2" means the 1.2.x
// series, ">=1.2" means ">= 1.2.0", and "<1.2" means "< 1.2.0".
func (t constraintTerm) matches(v version) bool {
	full := len(t.ver.comps) == 3
	switch t.op {
	case opWildcard:
		fixed := t.ver.comps
		vt := v.triple()
		for i, want := range fixed {
			if vt[i] != want {
				return false
			}
		}
		return true
	case "=":
		if full {
			return compareVersions(v, t.ver.filled()) == 0
		}
		return t.ver.containsSeries(v)
	case "!=":
		if full {
			return compareVersions(v, t.ver.filled()) != 0
		}
		return !t.ver.containsSeries(v)
	case ">":
		if full {
			return compareVersions(v, t.ver.filled()) > 0
		}
		return compareVersions(v, t.ver.seriesEnd()) >= 0
	case ">=":
		return compareVersions(v, t.ver.filled()) >= 0
	case "<":
		return compareVersions(v, t.ver.filled()) < 0
	case "<=":
		if full {
			return compareVersions(v, t.ver.filled()) <= 0
		}
		return compareVersions(v, t.ver.seriesEnd()) < 0
	case "^":
		return compareVersions(v, t.ver.filled()) >= 0 && compareVersions(v, t.ver.caretUpper()) < 0
	case "~":
		return compareVersions(v, t.ver.filled()) >= 0 && compareVersions(v, t.ver.tildeUpper()) < 0
	}
	return false
}

// filled completes missing components with zeros: 1.2 becomes 1.2.0.
func (p partialVersion) filled() version {
	v := version{pre: p.pre}
	if len(p.comps) > 0 {
		v.major = p.comps[0]
	}
	if len(p.comps) > 1 {
		v.minor = p.comps[1]
	}
	if len(p.comps) > 2 {
		v.patch = p.comps[2]
	}
	return v
}

// seriesEnd is the first version after the series designated by a partial
// version: for 1.2 it is 1.3.0, for 3 it is 4.0.0.
func (p partialVersion) seriesEnd() version {
	if len(p.comps) == 1 {
		return version{major: p.comps[0] + 1}
	}
	return version{major: p.comps[0], minor: p.comps[1] + 1}
}

// containsSeries reports whether v belongs to the series designated by a
// partial version (1.2 designates 1.2.x, 3 designates 3.x.y).
func (p partialVersion) containsSeries(v version) bool {
	return compareVersions(v, p.filled()) >= 0 && compareVersions(v, p.seriesEnd()) < 0
}

// caretUpper is the exclusive upper bound of a caret range: the leftmost
// non-zero component is bumped (semver caret rule, §9.2: "^1.4.0" allows
// <2.0.0, "^0.3.1" allows <0.4.0). When every given component is zero, the
// last given component is bumped ("^0.0.0" allows <0.0.1, "^0.0" <0.1.0,
// "^0" <1.0.0).
func (p partialVersion) caretUpper() version {
	c := p.comps
	switch {
	case c[0] > 0:
		return version{major: c[0] + 1}
	case len(c) > 1 && c[1] > 0:
		return version{minor: c[1] + 1}
	case len(c) > 2 && c[2] > 0:
		return version{patch: c[2] + 1}
	}
	// All given components are zero: bump the last given one.
	switch len(c) {
	case 1:
		return version{major: 1}
	case 2:
		return version{minor: c[1] + 1}
	default:
		return version{patch: c[2] + 1}
	}
}

// tildeUpper is the exclusive upper bound of a tilde range: patch-level
// changes when a minor version is given (§9.2: "~1.4.2" allows <1.5.0),
// minor-level changes otherwise ("~1" allows <2.0.0).
func (p partialVersion) tildeUpper() version {
	if len(p.comps) >= 2 {
		return version{major: p.comps[0], minor: p.comps[1] + 1}
	}
	return version{major: p.comps[0] + 1}
}
