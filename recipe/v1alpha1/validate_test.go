// SPDX-License-Identifier: Apache-2.0
// Copyright © 2026 infraBuilder SASU and contributors

package v1alpha1

import (
	"errors"
	"strings"
	"testing"
)

// handBuiltRecipe returns a valid cooked recipe built in Go, without going
// through ParseRecipe. Validate must fully check such values too.
func handBuiltRecipe() *Recipe {
	return &Recipe{
		APIVersion: APIVersion,
		Kind:       KindRecipe,
		Metadata:   Metadata{Name: "demo", Version: "1.0.0"},
		Spec: RecipeSpec{Ingredients: []Ingredient{{
			Name:    "app",
			Kind:    IngredientContainerImage,
			Ref:     "docker.io/library/nginx",
			Version: "1.25.0",
			Digest:  "sha256:8acca98ed81b53b482870d6b2081e60d2aa77293895c90c97d2b0e76f469ffb1",
		}}},
	}
}

func TestValidateHandBuiltRecipe(t *testing.T) {
	r := handBuiltRecipe()
	if err := r.Validate(ProfileDraft); err != nil {
		t.Errorf("draft: %v", err)
	}
	if err := r.Validate(ProfileCooked); err != nil {
		t.Errorf("cooked: %v", err)
	}
}

func TestValidateUnknownProfile(t *testing.T) {
	err := handBuiltRecipe().Validate(Profile("published"))
	if err == nil {
		t.Fatal("unknown profile accepted")
	}
	if !hasError(asList(t, err), RuleProfile, "", "published") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateNilReceivers(t *testing.T) {
	var r *Recipe
	if err := r.Validate(ProfileDraft); err == nil {
		t.Error("nil recipe accepted")
	}
	var rt *Retriever
	if err := rt.Validate(); err == nil {
		t.Error("nil retriever accepted")
	}
}

// TestValidateHandBuiltViolations: structural rules are re-checked from the
// struct, through the schema, so generated documents cannot bypass them.
func TestValidateHandBuiltViolations(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(*Recipe)
		profile  Profile
		wantRule string
		wantPath string
	}{
		{
			name:     "wrong apiVersion",
			mutate:   func(r *Recipe) { r.APIVersion = "recipe.tobby.dev/v9" },
			wantRule: RuleSchema, wantPath: "apiVersion",
		},
		{
			name:     "missing metadata name",
			mutate:   func(r *Recipe) { r.Metadata.Name = "" },
			wantRule: RuleSchema, wantPath: "metadata",
		},
		{
			name:     "invalid metadata version",
			mutate:   func(r *Recipe) { r.Metadata.Version = "1.0" },
			wantRule: RuleSchema, wantPath: "metadata.version",
		},
		{
			name:     "no ingredients",
			mutate:   func(r *Recipe) { r.Spec.Ingredients = nil },
			wantRule: RuleSchema, wantPath: "spec",
		},
		{
			name: "duplicate ingredient names",
			mutate: func(r *Recipe) {
				dup := r.Spec.Ingredients[0]
				dup.Ref = "docker.io/library/redis"
				r.Spec.Ingredients = append(r.Spec.Ingredients, dup)
			},
			wantRule: RuleIngredientNameDuplicate, wantPath: "spec.ingredients[1].name",
		},
		{
			name:     "invalid version syntax",
			mutate:   func(r *Recipe) { r.Spec.Ingredients[0].Version = "1.0 || 2.0" },
			wantRule: RuleVersionSyntax, wantPath: "spec.ingredients[0].version",
		},
		{
			name:     "port out of range",
			mutate:   func(r *Recipe) { r.Spec.Ingredients[0].Ref = "registry.example.com:70000/library/nginx" },
			wantRule: RulePortRange, wantPath: "spec.ingredients[0].ref",
		},
		{
			name:     "port zero",
			mutate:   func(r *Recipe) { r.Spec.Ingredients[0].Ref = "registry.example.com:0/library/nginx" },
			wantRule: RulePortRange, wantPath: "spec.ingredients[0].ref",
		},
		{
			name:     "cooked without digest",
			mutate:   func(r *Recipe) { r.Spec.Ingredients[0].Digest = "" },
			profile:  ProfileCooked,
			wantRule: RuleCookedDigestMissing, wantPath: "spec.ingredients[0].digest",
		},
		{
			name:     "cooked with constraint version",
			mutate:   func(r *Recipe) { r.Spec.Ingredients[0].Version = "^1.25.0" },
			profile:  ProfileCooked,
			wantRule: RuleCookedVersionNotExact, wantPath: "spec.ingredients[0].version",
		},
		{
			name: "extract on ContainerImage",
			mutate: func(r *Recipe) {
				r.Spec.Ingredients[0].Extract = &Extract{Paths: []string{"etc/**"}}
			},
			wantRule: RuleSchema, wantPath: "spec.ingredients[0].extract",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := handBuiltRecipe()
			tc.mutate(r)
			profile := tc.profile
			if profile == "" {
				profile = ProfileDraft
			}
			err := r.Validate(profile)
			if err == nil {
				t.Fatal("Validate accepted the mutated recipe")
			}
			if !hasError(asList(t, err), tc.wantRule, tc.wantPath, "") {
				t.Fatalf("no error with rule %q at path containing %q, got:\n%v", tc.wantRule, tc.wantPath, err)
			}
		})
	}
}

// handBuiltRetriever returns a valid retriever built in Go, without going
// through ParseRetriever. Validate must fully check such values too.
func handBuiltRetriever() *Retriever {
	return &Retriever{
		APIVersion: APIVersion,
		Kind:       KindRetriever,
		Metadata:   Metadata{Name: "restricted-zone"},
		Spec: RetrieverSpec{
			Cookbook: "registry.example.com/cookbook",
			Recipes: []RecipeSelector{
				{Name: "wordpress", Version: "6.8.2"},
				{Name: "postgresql", Version: "16.x", Cookbook: "registry.tools.example.com/cookbook"},
			},
		},
	}
}

func TestValidateHandBuiltRetriever(t *testing.T) {
	rt := handBuiltRetriever()
	if err := rt.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	// A retriever has no metadata.version (§10.2): the shared Metadata
	// struct marshals it only when set, and the schema then rejects it.
	rt.Metadata.Version = "1.0.0"
	err := rt.Validate()
	if err == nil {
		t.Fatal("retriever with metadata.version accepted")
	}
	if !hasError(asList(t, err), RuleSchema, "metadata", "version") {
		t.Errorf("unexpected error: %v", err)
	}

	rt.Metadata.Version = ""
	rt.Spec.Recipes[1].Cookbook = "registry.tools.example.com:99999/cookbook"
	if !hasError(asList(t, rt.Validate()), RulePortRange, "spec.recipes[1].cookbook", "") {
		t.Errorf("per-entry cookbook port not checked: %v", rt.Validate())
	}
}

// TestMetadataVersionIsBoundedLikeAnOCITag: metadata.version becomes the
// publication tag (§11.3), and OCI tags stop at 128 characters. A ~170
// character semver is perfectly valid semver and must still be rejected —
// by the schema's maxLength and by the SDK's semantic OCI-tag check alike.
func TestMetadataVersionIsBoundedLikeAnOCITag(t *testing.T) {
	long := "1.0.0-" + strings.Repeat("a", 164) // 170 characters of valid semver
	if len(long) != 170 {
		t.Fatalf("fixture is %d characters, want 170", len(long))
	}

	t.Run("via ParseRecipe", func(t *testing.T) {
		// Parsing stops at the schema stage: the maxLength of the
		// published schema is what rejects the document.
		doc := strings.Replace(minimalRecipe, "version: 1.0.0", "version: "+long, 1)
		_, err := ParseRecipe([]byte(doc))
		if err == nil {
			t.Fatal("a 170-character metadata.version was accepted")
		}
		if !hasError(asList(t, err), RuleSchema, "metadata.version", "") {
			t.Errorf("no schema error at metadata.version in: %v", err)
		}
	})

	t.Run("via Validate on a hand-built recipe", func(t *testing.T) {
		r := handBuiltRecipe()
		r.Metadata.Version = long
		err := r.Validate(ProfileDraft)
		if err == nil {
			t.Fatal("a 170-character metadata.version was accepted")
		}
		if !hasError(asList(t, err), RuleVersionSyntax, "metadata.version", "128") {
			t.Errorf("no OCI-tag error at metadata.version in: %v", err)
		}
	})

	t.Run("128 characters are fine", func(t *testing.T) {
		r := handBuiltRecipe()
		r.Metadata.Version = "1.0.0-" + strings.Repeat("a", 122) // exactly 128
		if err := r.Validate(ProfileDraft); err != nil {
			t.Errorf("a 128-character version must pass: %v", err)
		}
	})
}

// TestMetadataStringBounds: metadata.description and annotation values are
// bounded by the schemas, so a document cannot smuggle megabytes of text
// through its free-form metadata fields.
func TestMetadataStringBounds(t *testing.T) {
	t.Run("recipe description above 2048", func(t *testing.T) {
		r := handBuiltRecipe()
		r.Metadata.Description = strings.Repeat("d", 2049)
		if err := r.Validate(ProfileDraft); err == nil {
			t.Fatal("a 2049-character description was accepted")
		} else if !hasError(asList(t, err), RuleSchema, "metadata.description", "") {
			t.Errorf("no schema error at metadata.description in: %v", err)
		}
	})
	t.Run("recipe annotation value above 4096", func(t *testing.T) {
		r := handBuiltRecipe()
		r.Metadata.Annotations = map[string]string{"example.com/note": strings.Repeat("a", 4097)}
		if err := r.Validate(ProfileDraft); err == nil {
			t.Fatal("a 4097-character annotation value was accepted")
		} else if !hasError(asList(t, err), RuleSchema, "metadata.annotations", "") {
			t.Errorf("no schema error at metadata.annotations in: %v", err)
		}
	})
	t.Run("retriever description above 2048", func(t *testing.T) {
		rt := handBuiltRetriever()
		rt.Metadata.Description = strings.Repeat("d", 2049)
		if err := rt.Validate(); err == nil {
			t.Fatal("a 2049-character description was accepted")
		} else if !hasError(asList(t, err), RuleSchema, "metadata.description", "") {
			t.Errorf("no schema error at metadata.description in: %v", err)
		}
	})
	t.Run("retriever annotation value above 4096", func(t *testing.T) {
		rt := handBuiltRetriever()
		rt.Metadata.Annotations = map[string]string{"example.com/note": strings.Repeat("a", 4097)}
		if err := rt.Validate(); err == nil {
			t.Fatal("a 4097-character annotation value was accepted")
		} else if !hasError(asList(t, err), RuleSchema, "metadata.annotations", "") {
			t.Errorf("no schema error at metadata.annotations in: %v", err)
		}
	})
	t.Run("values at the bounds are fine", func(t *testing.T) {
		r := handBuiltRecipe()
		r.Metadata.Description = strings.Repeat("d", 2048)
		r.Metadata.Annotations = map[string]string{"example.com/note": strings.Repeat("a", 4096)}
		if err := r.Validate(ProfileDraft); err != nil {
			t.Errorf("bounds are inclusive: %v", err)
		}
	})
}

func TestValidatePublishLocation(t *testing.T) {
	r := handBuiltRecipe()
	if err := r.ValidatePublishLocation("demo", "1.0.0"); err != nil {
		t.Errorf("matching location rejected: %v", err)
	}
	err := r.ValidatePublishLocation("other", "2.0.0")
	if err == nil {
		t.Fatal("mismatching location accepted")
	}
	list := asList(t, err)
	if !hasError(list, RulePublishLocation, "metadata.name", "other") {
		t.Errorf("missing name mismatch: %v", err)
	}
	if !hasError(list, RulePublishLocation, "metadata.version", "2.0.0") {
		t.Errorf("missing version mismatch: %v", err)
	}
}

func TestErrorListBehavior(t *testing.T) {
	fe1 := &FieldError{Path: "metadata.name", Rule: RuleSchema, Message: "bad"}
	fe2 := &FieldError{Rule: RuleEmptyDocument, Message: "empty"}
	list := ErrorList{fe1, fe2}

	var asFieldErr *FieldError
	if !errors.As(error(list), &asFieldErr) {
		t.Error("errors.As cannot reach the individual *FieldError values")
	}
	if len(list.Unwrap()) != 2 {
		t.Errorf("Unwrap() length = %d", len(list.Unwrap()))
	}
	if got := list.ByRule(RuleSchema); len(got) != 1 || got[0] != fe1 {
		t.Errorf("ByRule = %v", got)
	}
	if msg := list.Error(); !strings.Contains(msg, "2 validation errors") || !strings.Contains(msg, "metadata.name") {
		t.Errorf("Error() = %q", msg)
	}
	if msg := (ErrorList{fe2}).Error(); !strings.Contains(msg, "empty (rule empty-document)") {
		t.Errorf("single Error() = %q", msg)
	}
}
