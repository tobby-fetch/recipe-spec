// SPDX-License-Identifier: Apache-2.0
// Copyright © 2026 infraBuilder SASU and contributors

package v1alpha1

import (
	"strings"
	"testing"
)

// TestParseConstraintClassification covers the disambiguation rules of
// §9.3: what is an exact tag, what is a constraint expression, and what is
// rejected outright.
func TestParseConstraintClassification(t *testing.T) {
	cases := []struct {
		in      string
		exact   bool
		wantErr string // substring of the expected error; empty means valid
	}{
		// Exact tags (§9.1).
		{in: "24.2.8", exact: true},
		{in: "6.8.2-debian-12-r3", exact: true},
		{in: "v1.30.1", exact: true},
		{in: "latest", exact: true},
		{in: "linux", exact: true},
		{in: "1.2", exact: true},         // partial version alone: a tag, not a constraint
		{in: "1.2.3.4", exact: true},     // four components: a tag
		{in: "1.2.x-rc", exact: true},    // 'x-rc' is not a placeholder component
		{in: "x86_64", exact: true},      // contains 'x' but is not a wildcard form
		{in: "_underscore", exact: true}, // valid OCI tag leading character
		{in: "1.2.3-01", exact: true},    // invalid semver, but a valid literal tag

		// Wildcards (§9.2).
		{in: "*"}, {in: "x"}, {in: "X"},
		{in: "12.x"}, {in: "12.X"}, {in: "12.4.x"}, {in: "1.2.*"}, {in: "1.*.*"},

		// Caret, tilde, comparisons.
		{in: "^1.4.0"}, {in: "^0.3.1"}, {in: "^0"}, {in: "^1.2"}, {in: "^1.2.3-rc.1"},
		{in: "~1.4.2"}, {in: "~1.2"}, {in: "~1"},
		{in: ">=2.0.0"}, {in: ">1.2"}, {in: "<=3"}, {in: "<4.0.0"},
		{in: "=1.2.3"}, {in: "=1.2"}, {in: "!=1.5.0"}, {in: ">=v1.2.3"},

		// Conjunctions (whitespace and/or commas, §9.2).
		{in: ">=2.0.0 <3.0.0"},
		{in: ">=2.0.0, <3.0.0"},
		{in: ">=2.0.0,<3.0.0"},
		{in: ">=1.0.0 !=1.5.0 <2.0.0"},
		{in: "12.x <12.5.0"},

		// Rejected.
		{in: "", wantErr: "empty"},
		{in: "||", wantErr: "||"},
		{in: "1.0.0 || 2.0.0", wantErr: "||"},
		{in: ">= 1.2.3", wantErr: "immediately followed"},
		{in: "1.2.3 1.2.4", wantErr: "bare version"},
		{in: "==1.2.3", wantErr: "=="},
		{in: "!1.2.3", wantErr: "!="},
		{in: ">", wantErr: "immediately followed"},
		{in: "^", wantErr: "immediately followed"},
		{in: "~", wantErr: "immediately followed"},
		{in: ">=1.x", wantErr: "wildcard"},
		{in: "^x", wantErr: "wildcard"},
		{in: "1.x.3", wantErr: "placeholder"},
		{in: "1.2.3.4.x", wantErr: "at most 3 components"},
		{in: ">1.2.3+build", wantErr: "build metadata"},
		{in: ">=01.2.3", wantErr: "leading zero"},
		{in: ">=1.2.3-rc..1", wantErr: "empty identifier"},
		{in: ">=1.2.3-rc_1", wantErr: "alphanumerics"},
		{in: ">=1.2-rc.1", wantErr: "full major.minor.patch"},
		{in: ">=1.2.3-01", wantErr: "leading zero"},
		{in: ">=x.y.z", wantErr: "wildcard"},
		{in: ">=1.y", wantErr: "not numeric"},
		{in: "not a tag!", wantErr: "bare version"},
		{in: "><1.2.3", wantErr: "unexpected operator"},
		{in: " , ", wantErr: "no terms"},
		{in: strings.Repeat("a", 300), wantErr: "256"},
		{in: strings.Repeat("a", 129), wantErr: "not a valid OCI tag"}, // tag too long
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			c, err := ParseConstraint(tc.in)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("ParseConstraint(%q) accepted, want error containing %q", tc.in, tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error %q does not contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseConstraint(%q): %v", tc.in, err)
			}
			if c.IsExact() != tc.exact {
				t.Errorf("IsExact() = %v, want %v", c.IsExact(), tc.exact)
			}
			if c.String() != tc.in {
				t.Errorf("String() = %q, want %q", c.String(), tc.in)
			}
		})
	}
}

// TestConstraintMatch is the resolution matrix of §9.2: which tags match
// which constraints, with pre-releases excluded unless mentioned on the
// same major.minor.patch triple.
func TestConstraintMatch(t *testing.T) {
	cases := []struct {
		constraint string
		tag        string
		want       bool
	}{
		// Exact tags: literal matching only (§9.1, §9.3).
		{"24.2.8", "24.2.8", true},
		{"24.2.8", "v24.2.8", false},
		{"v1.30.1", "v1.30.1", true},
		{"v1.30.1", "1.30.1", false},
		{"latest", "latest", true},
		{"latest", "Latest", false},

		// Wildcards; leading v ignored on candidates (§9.2).
		{"*", "1.0.0", true},
		{"*", "0.0.1", true},
		{"*", "v2.5.1", true},
		{"*", "1.0.0-rc.1", false}, // pre-release excluded
		{"*", "latest", false},     // non-semver tags never match
		{"12.x", "12.0.0", true},
		{"12.x", "12.9.1", true},
		{"12.x", "v12.3.0", true},
		{"12.x", "13.0.0", false},
		{"12.x", "12.4.0-rc.1", false},
		{"12.4.x", "12.4.7", true},
		{"12.4.x", "12.5.0", false},

		// Caret (§9.2: compatible within the leftmost non-zero component).
		{"^1.4.0", "1.4.0", true},
		{"^1.4.0", "1.9.9", true},
		{"^1.4.0", "1.3.9", false},
		{"^1.4.0", "2.0.0", false},
		{"^1.4.0", "2.0.0-rc.1", false}, // 2.0.0-rc.1 < 2.0.0, but the pre-release gate excludes it
		{"^0.3.1", "0.3.1", true},
		{"^0.3.1", "0.3.9", true},
		{"^0.3.1", "0.4.0", false},
		{"^0.0.3", "0.0.3", true},
		{"^0.0.3", "0.0.4", false},
		{"^1.2", "1.2.0", true},
		{"^1.2", "1.999.0", true},
		{"^1.2", "2.0.0", false},

		// Tilde (§9.2: patch-level changes only).
		{"~1.4.2", "1.4.2", true},
		{"~1.4.2", "1.4.10", true},
		{"~1.4.2", "1.4.1", false},
		{"~1.4.2", "1.5.0", false},
		{"~1.2", "1.2.9", true},
		{"~1.2", "1.3.0", false},
		{"~1", "1.9.0", true},
		{"~1", "2.0.0", false},

		// Comparisons, including partial versions (series semantics).
		{">=2.0.0", "2.0.0", true},
		{">=2.0.0", "1.9.9", false},
		{">1.2", "1.2.5", false}, // >1.2 means "above the 1.2 series"
		{">1.2", "1.3.0", true},
		{"<=3", "3.999.0", true}, // <=3 means "within or below the 3 series"
		{"<=3", "4.0.0", false},
		{"<4.0.0", "3.999.0", true},
		{"<4.0.0", "4.0.0", false},
		{"=1.2.3", "1.2.3", true},
		{"=1.2.3", "v1.2.3", true},
		{"=1.2.3", "1.2.4", false},
		{"=1.2", "1.2.9", true}, // =1.2 designates the 1.2.x series
		{"=1.2", "1.3.0", false},
		{"!=1.5.0", "1.5.0", false},
		{"!=1.5.0", "1.5.1", true},

		// Conjunctions: every term must hold.
		{">=2.0.0 <3.0.0", "2.0.0", true},
		{">=2.0.0 <3.0.0", "2.9.9", true},
		{">=2.0.0 <3.0.0", "3.0.0", false},
		{">=2.0.0 <3.0.0", "1.9.9", false},
		{">=2.0.0, <3.0.0", "2.5.0", true},
		{">=1.0.0 !=1.5.0 <2.0.0", "1.5.0", false},
		{">=1.0.0 !=1.5.0 <2.0.0", "1.5.1", true},
		{"12.x <12.5.0", "12.4.9", true},
		{"12.x <12.5.0", "12.5.0", false},

		// Pre-releases are included only when the expression mentions a
		// pre-release on the same triple (§9.2 rule 2).
		{">=1.5.0-rc.1", "1.5.0-rc.1", true},
		{">=1.5.0-rc.1", "1.5.0-rc.2", true},
		{">=1.5.0-rc.1", "1.5.0-rc.0", false}, // gate passes, comparison fails
		{">=1.5.0-rc.1", "1.5.0", true},
		{">=1.5.0-rc.1", "1.6.0-rc.1", false}, // different triple: gate excludes
		{">=1.5.0-rc.1", "1.6.0", true},
		{"^1.2.3-rc.1", "1.2.3-rc.2", true},
		{"^1.2.3-rc.1", "1.2.3-rc.0", false},
		{"^1.2.3-rc.1", "1.3.0-alpha", false},
		{"^1.2.3-rc.1", "1.9.0", true},
		{"!=1.5.0-rc.1", "1.5.0-rc.2", true},

		// Semver precedence details through comparisons.
		{">=1.0.0-alpha.1", "1.0.0-alpha.1", true},
		{">1.0.0-alpha.1", "1.0.0-alpha.2", true},
		{">1.0.0-alpha.1", "1.0.0-alpha", false},   // fewer identifiers sort lower
		{">1.0.0-alpha.9", "1.0.0-alpha.10", true}, // numeric identifiers compare numerically
		{">1.0.0-1", "1.0.0-alpha", true},          // numeric < alphanumeric
		{">=1.0.0-rc.1", "1.0.0+build", true},      // build metadata ignored (§9.2 rule 3)
	}
	for _, tc := range cases {
		t.Run(tc.constraint+"/"+tc.tag, func(t *testing.T) {
			c, err := ParseConstraint(tc.constraint)
			if err != nil {
				t.Fatalf("ParseConstraint(%q): %v", tc.constraint, err)
			}
			if got := c.Match(tc.tag); got != tc.want {
				t.Errorf("Match(%q) against %q = %v, want %v", tc.tag, tc.constraint, got, tc.want)
			}
		})
	}
}

// TestConstraintResolve covers the selection rules of §9.2: highest match
// wins, non-semver tags are ignored, and resolution fails rather than
// falling back.
func TestConstraintResolve(t *testing.T) {
	tags := []string{"1.0.0", "1.4.0", "1.5.0-rc.1", "1.9.3", "2.0.0", "banana", "latest"}
	cases := []struct {
		constraint string
		want       string
		wantErr    bool
	}{
		{constraint: "^1.4.0", want: "1.9.3"},
		{constraint: "*", want: "2.0.0"},
		{constraint: "1.x", want: "1.9.3"}, // 1.5.0-rc.1 excluded
		{constraint: ">=1.5.0-rc.1 <1.6.0", want: "1.5.0-rc.1"},
		{constraint: ">=3", wantErr: true},     // rule 5: no silent fallback
		{constraint: "latest", want: "latest"}, // exact tag: literal membership
		{constraint: "banana", want: "banana"},
		{constraint: "1.4.0", want: "1.4.0"},
		{constraint: "9.9.9", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.constraint, func(t *testing.T) {
			c, err := ParseConstraint(tc.constraint)
			if err != nil {
				t.Fatal(err)
			}
			got, err := c.Resolve(tags)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Resolve() = %q, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve(): %v", err)
			}
			if got != tc.want {
				t.Errorf("Resolve() = %q, want %q", got, tc.want)
			}
		})
	}

	// Deterministic tie-break: equal versions resolve to the
	// lexicographically smallest tag.
	c, err := ParseConstraint("~1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Resolve([]string{"v1.2.3", "1.2.3"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "1.2.3" {
		t.Errorf("tie-break Resolve() = %q, want %q", got, "1.2.3")
	}
}
