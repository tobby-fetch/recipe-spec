// SPDX-License-Identifier: Apache-2.0
// Copyright © 2026 infraBuilder SASU and contributors

package v1alpha1

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// invalidCase describes one crafted invalid manifest of testdata/invalid
// and the violation the SDK must report for it.
type invalidCase struct {
	file     string
	wantRule string // a FieldError with this rule must be present
	wantPath string // ... at a path containing this substring
	wantMsg  string // ... with a message containing this substring
}

var invalidCases = []invalidCase{
	// Document-level failures.
	{file: "malformed.yaml", wantRule: RuleYAMLSyntax},
	{file: "empty.yaml", wantRule: RuleEmptyDocument},
	{file: "multiple-documents.yaml", wantRule: RuleMultipleDocuments},
	{file: "root-sequence.yaml", wantRule: RuleDocumentRoot},

	// Envelope failures (§4.2, §5).
	{file: "apiversion-unknown.yaml", wantRule: RuleAPIVersion, wantPath: "apiVersion", wantMsg: "recipe.tobby.dev/v2"},
	{file: "apiversion-missing.yaml", wantRule: RuleAPIVersion, wantPath: "apiVersion", wantMsg: "missing"},
	{file: "kind-unknown.yaml", wantRule: RuleKind, wantPath: "kind", wantMsg: "Cookbook"},
	{file: "kind-missing.yaml", wantRule: RuleKind, wantPath: "kind", wantMsg: "missing"},

	// Strict validation: unknown fields are rejected wherever they appear
	// (§4.3).
	{file: "unknown-field-root.yaml", wantRule: RuleSchema, wantMsg: "status"},
	{file: "unknown-field-metadata.yaml", wantRule: RuleSchema, wantPath: "metadata", wantMsg: "owner"},
	{file: "unknown-field-ingredient.yaml", wantRule: RuleSchema, wantPath: "spec.ingredients[0]", wantMsg: "digset"},

	// Kind-specific fields on the wrong ingredient kind (§7).
	{file: "containerimage-vendordependencies.yaml", wantRule: RuleSchema, wantPath: "spec.ingredients[0].vendorDependencies", wantMsg: "HelmChart"},
	{file: "helmchart-platforms.yaml", wantRule: RuleSchema, wantPath: "spec.ingredients[0].platforms", wantMsg: "ContainerImage and FileSet"},
	{file: "fileset-artifacttype.yaml", wantRule: RuleSchema, wantPath: "spec.ingredients[0].artifactType", wantMsg: "OCIArtifact"},
	{file: "ociartifact-extract.yaml", wantRule: RuleSchema, wantPath: "spec.ingredients[0].extract", wantMsg: "FileSet"},

	// Metadata rules (§6.1).
	{file: "metadata-name-uppercase.yaml", wantRule: RuleSchema, wantPath: "metadata.name"},
	{file: "metadata-name-too-long.yaml", wantRule: RuleSchema, wantPath: "metadata.name"},
	{file: "metadata-version-build-metadata.yaml", wantRule: RuleSchema, wantPath: "metadata.version"},
	{file: "metadata-version-not-semver.yaml", wantRule: RuleSchema, wantPath: "metadata.version"},

	// Ingredient rules (§6.2, §7).
	{file: "duplicate-ingredient-names.yaml", wantRule: RuleIngredientNameDuplicate, wantPath: "spec.ingredients[1].name", wantMsg: "unique"},
	{file: "ingredient-name-invalid.yaml", wantRule: RuleSchema, wantPath: "spec.ingredients[0].name"},
	{file: "ingredient-kind-unknown.yaml", wantRule: RuleSchema, wantPath: "spec.ingredients[0].kind"},
	{file: "ingredients-empty.yaml", wantRule: RuleSchema, wantPath: "spec.ingredients"},
	{file: "ingredient-version-empty.yaml", wantRule: RuleSchema, wantPath: "spec.ingredients[0].version"},
	{file: "digest-malformed.yaml", wantRule: RuleSchema, wantPath: "spec.ingredients[0].digest"},
	{file: "digest-sha512-short.yaml", wantRule: RuleSchema, wantPath: "spec.ingredients[0].digest"},
	{file: "platform-invalid.yaml", wantRule: RuleSchema, wantPath: "spec.ingredients[0].platforms[0]"},
	{file: "extract-absolute-path.yaml", wantRule: RuleSchema, wantPath: "spec.ingredients[0].extract.paths[0]"},
	{file: "extract-dotdot-path.yaml", wantRule: RuleSchema, wantPath: "spec.ingredients[0].extract.paths[0]"},
	{file: "extract-empty-paths.yaml", wantRule: RuleSchema, wantPath: "spec.ingredients[0].extract.paths"},

	// Ref rules (§6.2): host+repository only, fully qualified.
	{file: "ref-with-tag.yaml", wantRule: RuleSchema, wantPath: "spec.ingredients[0].ref"},
	{file: "ref-with-digest.yaml", wantRule: RuleSchema, wantPath: "spec.ingredients[0].ref"},
	{file: "ref-no-registry.yaml", wantRule: RuleSchema, wantPath: "spec.ingredients[0].ref"},
	{file: "ref-with-scheme.yaml", wantRule: RuleSchema, wantPath: "spec.ingredients[0].ref"},
	{file: "ref-port-out-of-range.yaml", wantRule: RulePortRange, wantPath: "spec.ingredients[0].ref", wantMsg: "99999"},

	// Version grammar (§9).
	{file: "constraint-disjunction.yaml", wantRule: RuleVersionSyntax, wantPath: "spec.ingredients[0].version", wantMsg: "||"},
	{file: "constraint-invalid.yaml", wantRule: RuleVersionSyntax, wantPath: "spec.ingredients[0].version"},
	{file: "constraint-space-after-operator.yaml", wantRule: RuleVersionSyntax, wantPath: "spec.ingredients[0].version", wantMsg: "immediately followed"},
	{file: "constraint-wildcard-midposition.yaml", wantRule: RuleVersionSyntax, wantPath: "spec.ingredients[0].version", wantMsg: "placeholder"},
	{file: "ingredient-version-build-metadata.yaml", wantRule: RuleVersionSyntax, wantPath: "spec.ingredients[0].version", wantMsg: "not a valid OCI tag"},

	// Retriever rules (§10).
	{file: "retriever-missing-cookbook.yaml", wantRule: RuleSchema, wantPath: "spec", wantMsg: "cookbook"},
	{file: "retriever-empty-recipes.yaml", wantRule: RuleSchema, wantPath: "spec.recipes"},
	{file: "retriever-metadata-version.yaml", wantRule: RuleSchema, wantPath: "metadata", wantMsg: "version"},
	{file: "retriever-unknown-field-spec.yaml", wantRule: RuleSchema, wantPath: "spec", wantMsg: "mode"},
	{file: "retriever-constraint-invalid.yaml", wantRule: RuleVersionSyntax, wantPath: "spec.recipes[0].version", wantMsg: "||"},
}

func TestInvalidManifestsRejected(t *testing.T) {
	for _, tc := range invalidCases {
		t.Run(tc.file, func(t *testing.T) {
			data := readInvalid(t, tc.file)
			obj, err := Parse(data)
			if err == nil {
				t.Fatalf("Parse accepted invalid manifest (parsed a %T)", obj)
			}
			var list ErrorList
			if !errors.As(err, &list) {
				t.Fatalf("error is not an ErrorList: %T: %v", err, err)
			}
			if !hasError(list, tc.wantRule, tc.wantPath, tc.wantMsg) {
				t.Fatalf("no error with rule %q, path containing %q, message containing %q; got:\n%v",
					tc.wantRule, tc.wantPath, tc.wantMsg, err)
			}
		})
	}
}

// TestInvalidCorpusIsExhaustive keeps the table above and the files on disk
// in sync: every crafted manifest must be asserted on.
func TestInvalidCorpusIsExhaustive(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("testdata", "invalid"))
	if err != nil {
		t.Fatal(err)
	}
	covered := make(map[string]bool, len(invalidCases))
	for _, tc := range invalidCases {
		covered[tc.file] = true
	}
	for _, e := range entries {
		if !covered[e.Name()] {
			t.Errorf("testdata/invalid/%s is not covered by TestInvalidManifestsRejected", e.Name())
		}
		delete(covered, e.Name())
	}
	for file := range covered {
		t.Errorf("test table references missing file testdata/invalid/%s", file)
	}
}

func readInvalid(t *testing.T, name string) []byte {
	t.Helper()
	// #nosec G304 -- test fixture path, repo-local and table-driven.
	data, err := os.ReadFile(filepath.Join("testdata", "invalid", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func hasError(list ErrorList, rule, pathSub, msgSub string) bool {
	for _, fe := range list {
		if fe.Rule == rule && strings.Contains(fe.Path, pathSub) && strings.Contains(fe.Message, msgSub) {
			return true
		}
	}
	return false
}
