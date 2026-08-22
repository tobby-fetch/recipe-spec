// SPDX-License-Identifier: Apache-2.0
// Copyright © 2026 infraBuilder SASU and contributors

package v1alpha1

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Profile selects the validation profile of RECIPE-SPEC.md §8: a recipe is
// either a draft (under construction) or cooked (published, fully pinned).
type Profile string

const (
	// ProfileDraft validates a recipe under construction: digests are
	// optional and version constraints are allowed (§8).
	ProfileDraft Profile = "draft"
	// ProfileCooked validates a recipe as required for publication to a
	// cookbook: every ingredient carries a digest and an exact-tag
	// version (§8). Publishing tools must reject recipes that fail this
	// profile.
	ProfileCooked Profile = "cooked"
)

// Validate checks the recipe against the published JSON Schema (including
// the strict rejection of structural violations, §4.3) and the semantic
// rules that the specification delegates to tooling (§16): ingredient name
// uniqueness, version grammar, and — with ProfileCooked — the fully pinned
// profile of §8. It works on any Recipe value, whether obtained from
// ParseRecipe or built in Go; documents returned by ParseRecipe always
// satisfy ProfileDraft.
//
// The returned error, when non-nil, is an ErrorList aggregating every
// violation.
func (r *Recipe) Validate(profile Profile) error {
	if r == nil {
		return ErrorList{{Rule: RuleInternal, Message: "nil recipe"}}
	}
	var errs ErrorList
	switch profile {
	case ProfileDraft, ProfileCooked:
	default:
		return ErrorList{{Rule: RuleProfile, Message: fmt.Sprintf("unknown validation profile %q: use ProfileDraft or ProfileCooked", profile)}}
	}
	errs = append(errs, structSchemaErrors(r, KindRecipe)...)
	errs = append(errs, recipeSemanticErrors(r)...)
	if profile == ProfileCooked {
		errs = append(errs, cookedErrors(r)...)
	}
	return errs.errOrNil()
}

// Validate checks the retriever against the published JSON Schema and the
// semantic rules delegated to tooling (§16): the version grammar of every
// recipe entry. It works on any Retriever value, whether obtained from
// ParseRetriever or built in Go. Retrievers have no draft/cooked profiles.
//
// The returned error, when non-nil, is an ErrorList aggregating every
// violation.
func (r *Retriever) Validate() error {
	if r == nil {
		return ErrorList{{Rule: RuleInternal, Message: "nil retriever"}}
	}
	var errs ErrorList
	errs = append(errs, structSchemaErrors(r, KindRetriever)...)
	errs = append(errs, retrieverSemanticErrors(r)...)
	return errs.errOrNil()
}

// ValidatePublishLocation checks the publication-location consistency rule
// of §11.3: the repository's last path segment must equal metadata.name and
// the tag must equal metadata.version. Consumers must reject a recipe
// artifact whose content disagrees with its location.
func (r *Recipe) ValidatePublishLocation(repositoryLastSegment, tag string) error {
	var errs ErrorList
	if r.Metadata.Name != repositoryLastSegment {
		errs = append(errs, &FieldError{
			Path: "metadata.name", Rule: RulePublishLocation,
			Message: fmt.Sprintf("recipe name %q does not match the repository's last path segment %q (§11.3)", r.Metadata.Name, repositoryLastSegment),
		})
	}
	if r.Metadata.Version != tag {
		errs = append(errs, &FieldError{
			Path: "metadata.version", Rule: RulePublishLocation,
			Message: fmt.Sprintf("recipe version %q does not match the publication tag %q (§11.3)", r.Metadata.Version, tag),
		})
	}
	return errs.errOrNil()
}

// structSchemaErrors validates a Go value against the embedded JSON Schema
// for the given kind, by round-tripping it through its JSON representation.
// Keeping the schema as the single source of structural truth avoids
// duplicating every regex and conditional in Go.
func structSchemaErrors(v any, kindName string) ErrorList {
	sch, err := compiledSchema(kindName)
	if err != nil {
		return ErrorList{{Rule: RuleInternal, Message: err.Error()}}
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return ErrorList{{Rule: RuleInternal, Message: fmt.Sprintf("document cannot be encoded as JSON: %v", err)}}
	}
	var instance any
	if err := json.Unmarshal(raw, &instance); err != nil {
		return ErrorList{{Rule: RuleInternal, Message: fmt.Sprintf("JSON round-trip failed: %v", err)}}
	}
	if err := sch.Validate(instance); err != nil {
		return schemaErrorList(err)
	}
	return nil
}

// recipeSemanticErrors implements the draft-level rules delegated to
// tooling by §16: ingredient name uniqueness (§6.2) and the constraint
// grammar of every ingredient version (§9), plus the registry port sanity
// check on refs and the OCI-tag validity of metadata.version (§6.1, §11.3).
func recipeSemanticErrors(r *Recipe) ErrorList {
	var errs ErrorList
	// metadata.version becomes the publication tag verbatim (§11.3), so
	// beyond being semver it must be a valid OCI tag — in practice this
	// adds the 128-character tag length limit, which the semver grammar
	// alone does not impose. The published schema carries the same bound
	// (maxLength 128); this check keeps hand-built Recipe values honest
	// through the same rule.
	if r.Metadata.Version != "" && !ociTagPattern.MatchString(r.Metadata.Version) {
		errs = append(errs, &FieldError{
			Path: "metadata.version", Rule: RuleVersionSyntax,
			Message: fmt.Sprintf("recipe version (%d characters) is not a valid OCI tag: it becomes the publication tag, at most 128 characters of alphanumerics, '.', '_', '-' (§6.1, §11.3)",
				len(r.Metadata.Version)),
		})
	}
	seen := make(map[string]int, len(r.Spec.Ingredients))
	for i, ing := range r.Spec.Ingredients {
		if first, dup := seen[ing.Name]; dup && ing.Name != "" {
			errs = append(errs, &FieldError{
				Path: ingredientPath(i, "name"), Rule: RuleIngredientNameDuplicate,
				Message: fmt.Sprintf("ingredient name %q is already used by spec.ingredients[%d]; names must be unique within the recipe (§6.2)", ing.Name, first),
			})
		} else {
			seen[ing.Name] = i
		}
		if ing.Version != "" {
			if _, err := ParseConstraint(ing.Version); err != nil {
				errs = append(errs, &FieldError{Path: ingredientPath(i, "version"), Rule: RuleVersionSyntax, Message: err.Error()})
			}
		}
		errs = append(errs, portRangeError(ing.Ref, ingredientPath(i, "ref"))...)
	}
	return errs
}

// retrieverSemanticErrors implements the tooling-delegated rules for
// retrievers: the constraint grammar of every recipe entry (§10.2, §9) and
// the registry port sanity check on cookbook references.
func retrieverSemanticErrors(r *Retriever) ErrorList {
	var errs ErrorList
	errs = append(errs, portRangeError(r.Spec.Cookbook, "spec.cookbook")...)
	for i, sel := range r.Spec.Recipes {
		if sel.Version != "" {
			if _, err := ParseConstraint(sel.Version); err != nil {
				errs = append(errs, &FieldError{
					Path: fmt.Sprintf("spec.recipes[%d].version", i), Rule: RuleVersionSyntax,
					Message: err.Error(),
				})
			}
		}
		if sel.Cookbook != "" {
			errs = append(errs, portRangeError(sel.Cookbook, fmt.Sprintf("spec.recipes[%d].cookbook", i))...)
		}
	}
	return errs
}

// cookedErrors implements the cooked profile of §8: every ingredient must
// carry a digest and an exact-tag version.
func cookedErrors(r *Recipe) ErrorList {
	var errs ErrorList
	for i, ing := range r.Spec.Ingredients {
		if ing.Digest == "" {
			errs = append(errs, &FieldError{
				Path: ingredientPath(i, "digest"), Rule: RuleCookedDigestMissing,
				Message: "a cooked recipe pins every ingredient by digest; record the content digest before publication (§8)",
			})
		}
		if ing.Version == "" {
			continue // already reported by the schema (required field)
		}
		c, err := ParseConstraint(ing.Version)
		if err != nil {
			continue // already reported by recipeSemanticErrors
		}
		if !c.IsExact() {
			errs = append(errs, &FieldError{
				Path: ingredientPath(i, "version"), Rule: RuleCookedVersionNotExact,
				Message: fmt.Sprintf("%q is a constraint expression; a cooked recipe uses exact tags only — resolve the constraint before publication (§8)", ing.Version),
			})
		}
	}
	return errs
}

// portRangeError flags registry ports outside 1-65535 in a ref or cookbook
// reference ("<registry-host>[:<port>]/<repository-path>"). The published
// schemas only constrain the port to 1-5 digits; the SDK tightens it to the
// valid TCP port range. Values whose port is not numeric are left to the
// schema pattern check.
func portRangeError(ref, path string) ErrorList {
	host, _, found := strings.Cut(ref, "/")
	if !found {
		return nil
	}
	idx := strings.LastIndexByte(host, ':')
	if idx < 0 {
		return nil
	}
	port := host[idx+1:]
	if !isDigits(port) {
		return nil // non-numeric port: left to the schema pattern check
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 { // Atoi only fails on overflow here
		return ErrorList{{
			Path: path, Rule: RulePortRange,
			Message: fmt.Sprintf("registry port %s is outside the valid range 1-65535", port),
		}}
	}
	return nil
}
