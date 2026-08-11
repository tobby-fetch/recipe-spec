// SPDX-License-Identifier: Apache-2.0
// Copyright © 2026 infraBuilder SASU and contributors

package v1alpha1

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"

	"github.com/tobby-fetch/recipe-spec/schemas"
)

const examplesDir = "../../examples"

// exampleCase describes one example shipped in examples/ and the profiles
// it must satisfy.
type exampleCase struct {
	file      string
	kind      string
	cooked    bool // for recipes: must the cooked profile pass?
	wantRules []string
}

var exampleCases = []exampleCase{
	// Cooked recipe: HelmChart + two multi-platform ContainerImages, all
	// digest-pinned.
	{file: "wordpress.yaml", kind: KindRecipe, cooked: true},
	// Cooked recipe carrying an extractable FileSet.
	{file: "fileset.yaml", kind: KindRecipe, cooked: true},
	// Draft recipe: valid as a draft, but fails the cooked profile on its
	// unpinned constraint-based ingredient (spec.ingredients[1]).
	{file: "ai-model.yaml", kind: KindRecipe, cooked: false,
		wantRules: []string{RuleCookedDigestMissing, RuleCookedVersionNotExact}},
	// A zone Retriever mixing exact versions and constraints.
	{file: "retriever-zone.yaml", kind: KindRetriever},
}

// TestExamples is the milestone gate: every example in examples/ parses and
// validates with this SDK, in the profile its header comment announces.
func TestExamples(t *testing.T) {
	for _, tc := range exampleCases {
		t.Run(tc.file, func(t *testing.T) {
			data := readExample(t, tc.file)
			obj, err := Parse(data)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if got := obj.ObjectKind(); got != tc.kind {
				t.Fatalf("kind = %q, want %q", got, tc.kind)
			}
			switch doc := obj.(type) {
			case *Recipe:
				if err := doc.Validate(ProfileDraft); err != nil {
					t.Errorf("draft profile: %v", err)
				}
				err := doc.Validate(ProfileCooked)
				if tc.cooked {
					if err != nil {
						t.Errorf("cooked profile: %v", err)
					}
					break
				}
				if err == nil {
					t.Fatal("cooked profile unexpectedly passed on a draft example")
				}
				var list ErrorList
				if !errors.As(err, &list) {
					t.Fatalf("error is not an ErrorList: %T", err)
				}
				for _, rule := range tc.wantRules {
					if len(list.ByRule(rule)) == 0 {
						t.Errorf("cooked profile: no error with rule %q in:\n%v", rule, err)
					}
				}
			case *Retriever:
				if err := doc.Validate(); err != nil {
					t.Errorf("Validate: %v", err)
				}
			}
		})
	}
}

// TestExamplesAreExhaustive keeps the table above in sync with examples/.
func TestExamplesAreExhaustive(t *testing.T) {
	entries, err := os.ReadDir(examplesDir)
	if err != nil {
		t.Fatal(err)
	}
	covered := make(map[string]bool, len(exampleCases))
	for _, tc := range exampleCases {
		covered[tc.file] = true
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		if !covered[e.Name()] {
			t.Errorf("examples/%s is not covered by TestExamples", e.Name())
		}
		delete(covered, e.Name())
	}
	for file := range covered {
		t.Errorf("test table references missing file examples/%s", file)
	}
}

// TestExamplesKindDispatch checks the kind-specific parsers on the
// examples, including the mismatch errors.
func TestExamplesKindDispatch(t *testing.T) {
	wordpress := readExample(t, "wordpress.yaml")
	retriever := readExample(t, "retriever-zone.yaml")

	if _, err := ParseRecipe(wordpress); err != nil {
		t.Errorf("ParseRecipe(wordpress): %v", err)
	}
	if _, err := ParseRetriever(retriever); err != nil {
		t.Errorf("ParseRetriever(retriever-zone): %v", err)
	}
	if _, err := ParseRecipe(retriever); err == nil {
		t.Error("ParseRecipe accepted a Retriever document")
	} else if !hasError(asList(t, err), RuleKind, "kind", KindRetriever) {
		t.Errorf("ParseRecipe(retriever): unexpected error: %v", err)
	}
	if _, err := ParseRetriever(wordpress); err == nil {
		t.Error("ParseRetriever accepted a Recipe document")
	}
}

// TestExamplesMatchRawJSONSchema validates every example directly against
// the embedded JSON Schemas, without going through the SDK pipeline: the
// schemas and the SDK must agree on what is valid.
func TestExamplesMatchRawJSONSchema(t *testing.T) {
	compile := func(id string, raw []byte) *jsonschema.Schema {
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		c := jsonschema.NewCompiler()
		if addErr := c.AddResource(id, doc); addErr != nil {
			t.Fatal(addErr)
		}
		sch, err := c.Compile(id)
		if err != nil {
			t.Fatal(err)
		}
		return sch
	}
	bySchemaKind := map[string]*jsonschema.Schema{
		KindRecipe:    compile(schemas.RecipeSchemaID, schemas.RecipeSchemaJSON),
		KindRetriever: compile(schemas.RetrieverSchemaID, schemas.RetrieverSchemaJSON),
	}
	for _, tc := range exampleCases {
		t.Run(tc.file, func(t *testing.T) {
			var raw any
			if err := yaml.Unmarshal(readExample(t, tc.file), &raw); err != nil {
				t.Fatal(err)
			}
			jsonBytes, err := json.Marshal(raw)
			if err != nil {
				t.Fatal(err)
			}
			var instance any
			if err := json.Unmarshal(jsonBytes, &instance); err != nil {
				t.Fatal(err)
			}
			if err := bySchemaKind[tc.kind].Validate(instance); err != nil {
				t.Errorf("raw JSON Schema validation failed: %v", err)
			}
		})
	}
}

func readExample(t *testing.T, name string) []byte {
	t.Helper()
	// #nosec G304 -- test fixture path, repo-local and table-driven.
	data, err := os.ReadFile(filepath.Join(examplesDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func asList(t *testing.T, err error) ErrorList {
	t.Helper()
	var list ErrorList
	if !errors.As(err, &list) {
		t.Fatalf("error is not an ErrorList: %T: %v", err, err)
	}
	return list
}
