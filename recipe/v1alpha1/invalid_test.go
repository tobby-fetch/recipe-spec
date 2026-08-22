// SPDX-License-Identifier: Apache-2.0
// Copyright © 2026 infraBuilder SASU and contributors

package v1alpha1

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"

	"github.com/tobby-fetch/recipe-spec/schemas"
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
	{file: "extract-dotdot-path.yaml", wantRule: RuleSchema, wantPath: "spec.ingredients[0].extract.paths[0]", wantMsg: "'..'"},
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

// rawSchemaExempt lists the fixtures of testdata/invalid that the raw JSON
// Schemas alone can NOT reject, with the reason each one is out of a JSON
// Schema's reach. Everything else in the corpus must be rejected by the
// published schemas directly — a third-party implementation validating with
// nothing but schemas/*.json gets that protection, and this list is the
// exact statement of what it does not get (the §16 rules delegated to
// tooling).
var rawSchemaExempt = map[string]string{
	// Never reaches schema validation: there is no instance to validate.
	"malformed.yaml": "not well-formed YAML; rejected before any schema sees it",
	// A stream of two schema-valid documents: the one-document-per-parse
	// rule is §5 parsing behavior, not document structure.
	"multiple-documents.yaml": "each document is schema-valid; the single-document rule is §5 parsing",
	// §16: semantic rules delegated to tooling, inexpressible in JSON
	// Schema.
	"duplicate-ingredient-names.yaml":        "ingredient name uniqueness is §6.2 semantics (tooling)",
	"constraint-invalid.yaml":                "constraint grammar beyond surface syntax is §9 semantics (tooling)",
	"constraint-disjunction.yaml":            "constraint grammar beyond surface syntax is §9 semantics (tooling)",
	"constraint-space-after-operator.yaml":   "constraint grammar beyond surface syntax is §9 semantics (tooling)",
	"constraint-wildcard-midposition.yaml":   "constraint grammar beyond surface syntax is §9 semantics (tooling)",
	"ingredient-version-build-metadata.yaml": "the schema bounds version to a string; OCI tag validity is §9 semantics (tooling)",
	"retriever-constraint-invalid.yaml":      "constraint grammar beyond surface syntax is §9 semantics (tooling)",
	// The schema pattern stops at 1-5 digits; the TCP range is SDK
	// semantics (RulePortRange).
	"ref-port-out-of-range.yaml": "the ref pattern only bounds the port to 1-5 digits; the TCP range is tooling",
}

// TestInvalidCorpusRejectedByRawSchemas passes every applicable fixture of
// testdata/invalid straight through the published JSON Schemas — no SDK
// pipeline, no semantic checks — and requires the rejection. The corpus
// otherwise only proves that the SDK rejects these documents; this proves
// the schemas themselves do, which is what every non-Go consumer of
// schemas/*.json relies on.
func TestInvalidCorpusRejectedByRawSchemas(t *testing.T) {
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
	recipeSch := compile(schemas.RecipeSchemaID, schemas.RecipeSchemaJSON)
	retrieverSch := compile(schemas.RetrieverSchemaID, schemas.RetrieverSchemaJSON)

	used := make(map[string]bool, len(rawSchemaExempt))
	for _, tc := range invalidCases {
		t.Run(tc.file, func(t *testing.T) {
			if reason, exempt := rawSchemaExempt[tc.file]; exempt {
				used[tc.file] = true
				t.Skipf("outside JSON Schema's reach: %s", reason)
			}
			var raw any
			if err := yaml.Unmarshal(readInvalid(t, tc.file), &raw); err != nil {
				t.Fatalf("fixture does not YAML-decode (add it to rawSchemaExempt if that is the point): %v", err)
			}
			jsonBytes, err := json.Marshal(raw)
			if err != nil {
				t.Fatalf("fixture is not JSON-representable: %v", err)
			}
			var instance any
			if err := json.Unmarshal(jsonBytes, &instance); err != nil {
				t.Fatal(err)
			}
			// Validate against the schema of the kind the document claims;
			// anything else (unknown kind, missing kind, non-mapping root)
			// goes to the Recipe schema, which must reject it anyway.
			sch, schID := recipeSch, schemas.RecipeSchemaID
			if root, ok := instance.(map[string]any); ok && root["kind"] == KindRetriever {
				sch, schID = retrieverSch, schemas.RetrieverSchemaID
			}
			if err := sch.Validate(instance); err == nil {
				t.Errorf("the raw schema %s accepted this document; either the schema lost a rule or the fixture belongs in rawSchemaExempt", schID)
			}
		})
	}
	for file := range rawSchemaExempt {
		if !used[file] {
			t.Errorf("rawSchemaExempt entry %q matches no fixture in the test table (stale exemption)", file)
		}
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
