// SPDX-License-Identifier: Apache-2.0
// Copyright © 2026 infraBuilder SASU and contributors

package v1alpha1

import (
	"strings"
	"testing"
)

// minimalRecipe is the minimal example of RECIPE-SPEC.md §6.3.
const minimalRecipe = `apiVersion: recipe.tobby.dev/v1alpha1
kind: Recipe
metadata:
  name: hello
  version: 1.0.0
spec:
  ingredients:
    - name: hello
      kind: ContainerImage
      ref: docker.io/library/hello-world
      version: linux
      platforms: ["linux/amd64"]
`

func TestParseMinimalRecipe(t *testing.T) {
	r, err := ParseRecipe([]byte(minimalRecipe))
	if err != nil {
		t.Fatalf("ParseRecipe: %v", err)
	}
	if r.Metadata.Name != "hello" || r.Metadata.Version != "1.0.0" {
		t.Errorf("metadata = %+v", r.Metadata)
	}
	if len(r.Spec.Ingredients) != 1 {
		t.Fatalf("ingredients = %d, want 1", len(r.Spec.Ingredients))
	}
	ing := r.Spec.Ingredients[0]
	if ing.Kind != IngredientContainerImage || ing.Version != "linux" {
		t.Errorf("ingredient = %+v", ing)
	}
	if ing.VendorsDependencies() {
		t.Error("VendorsDependencies() must default to false")
	}
	if err := r.Validate(ProfileDraft); err != nil {
		t.Errorf("draft profile: %v", err)
	}
	// An exact tag without digest is a draft, not a cooked recipe.
	if err := r.Validate(ProfileCooked); err == nil {
		t.Error("cooked profile unexpectedly passed without digests")
	}
}

// TestParseJSONInput: JSON is a YAML subset, so JSON documents parse too.
func TestParseJSONInput(t *testing.T) {
	doc := `{
	  "apiVersion": "recipe.tobby.dev/v1alpha1",
	  "kind": "Recipe",
	  "metadata": {"name": "hello", "version": "1.0.0"},
	  "spec": {"ingredients": [
	    {"name": "hello", "kind": "ContainerImage",
	     "ref": "docker.io/library/hello-world", "version": "linux"}
	  ]}
	}`
	if _, err := ParseRecipe([]byte(doc)); err != nil {
		t.Fatalf("ParseRecipe(JSON): %v", err)
	}
}

func TestParseDocumentLevelErrors(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		wantRule string
	}{
		{name: "empty input", in: "", wantRule: RuleEmptyDocument},
		{name: "only comments", in: "# nothing\n", wantRule: RuleEmptyDocument},
		{name: "null document", in: "null\n", wantRule: RuleEmptyDocument},
		{name: "root scalar", in: `"hello"`, wantRule: RuleDocumentRoot},
		{name: "root sequence", in: "- a\n- b\n", wantRule: RuleDocumentRoot},
		{name: "two documents", in: "a: 1\n---\nb: 2\n", wantRule: RuleMultipleDocuments},
		{name: "trailing empty document", in: minimalRecipe + "---\n", wantRule: RuleMultipleDocuments},
		{name: "tabs are not yaml", in: "\ta: 1", wantRule: RuleYAMLSyntax},
		{
			name:     "duplicate mapping key",
			in:       strings.Replace(minimalRecipe, "kind: Recipe", "kind: Recipe\nkind: Recipe", 1),
			wantRule: RuleYAMLSyntax,
		},
		{
			name:     "self-referential alias",
			in:       "a: &a\n  b: *a\n",
			wantRule: RuleYAMLSyntax,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.in))
			if err == nil {
				t.Fatal("Parse accepted the document")
			}
			list := asList(t, err)
			if !hasError(list, tc.wantRule, "", "") {
				t.Fatalf("no error with rule %q, got:\n%v", tc.wantRule, err)
			}
		})
	}
}

// TestParseReportsAllViolations: the SDK aggregates every violation instead
// of stopping at the first one.
func TestParseReportsAllViolations(t *testing.T) {
	doc := `apiVersion: recipe.tobby.dev/v1alpha1
kind: Recipe
metadata:
  name: Demo
  version: not-semver
spec:
  ingredients:
    - name: app
      kind: ContainerImage
      ref: docker.io/library/nginx
      version: 1.25.0
      digset: oops
`
	_, err := ParseRecipe([]byte(doc))
	list := asList(t, err)
	if len(list) < 3 {
		t.Fatalf("expected at least 3 aggregated errors, got %d:\n%v", len(list), err)
	}
	for _, wantPath := range []string{"metadata.name", "metadata.version", "spec.ingredients[0]"} {
		if !hasError(list, RuleSchema, wantPath, "") {
			t.Errorf("no schema error at path containing %q in:\n%v", wantPath, err)
		}
	}
}

// TestParseNonStringMappingKey: documents must be representable as JSON
// (§5), so non-string mapping keys are rejected one way or another.
func TestParseNonStringMappingKey(t *testing.T) {
	doc := "apiVersion: recipe.tobby.dev/v1alpha1\nkind: Recipe\nmetadata:\n  1: x\n"
	if _, err := Parse([]byte(doc)); err == nil {
		t.Fatal("Parse accepted a document with a non-string mapping key")
	}
}

// TestParseRejectsOversizedInput pins the §5 size bound: input above
// MaxParseBytes is refused before any decoding — without the bound, a
// multi-megabyte YAML file costs hundreds of megabytes of memory just to
// be told it is "valid".
func TestParseRejectsOversizedInput(t *testing.T) {
	// A byte-count violation, not a structural one: the content would be
	// a perfectly valid recipe if it were smaller.
	huge := minimalRecipe + "# " + strings.Repeat("x", MaxParseBytes) + "\n"
	for name, parse := range map[string]func([]byte) (any, error){
		"Parse":          func(b []byte) (any, error) { return Parse(b) },
		"ParseRecipe":    func(b []byte) (any, error) { return ParseRecipe(b) },
		"ParseRetriever": func(b []byte) (any, error) { return ParseRetriever(b) },
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parse([]byte(huge))
			if err == nil {
				t.Fatal("oversized input accepted")
			}
			if !hasError(asList(t, err), RuleDocumentTooLarge, "", "bound") {
				t.Fatalf("no %s error, got: %v", RuleDocumentTooLarge, err)
			}
		})
	}
}

// TestParseAcceptsInputAtTheBound: the bound is inclusive — a document of
// exactly MaxParseBytes parses.
func TestParseAcceptsInputAtTheBound(t *testing.T) {
	padding := MaxParseBytes - len(minimalRecipe) - len("# \n")
	doc := minimalRecipe + "# " + strings.Repeat("x", padding) + "\n"
	if len(doc) != MaxParseBytes {
		t.Fatalf("fixture is %d bytes, want exactly %d", len(doc), MaxParseBytes)
	}
	if _, err := ParseRecipe([]byte(doc)); err != nil {
		t.Fatalf("a document of exactly MaxParseBytes must parse: %v", err)
	}
}
