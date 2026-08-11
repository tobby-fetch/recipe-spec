// SPDX-License-Identifier: Apache-2.0
// Copyright © 2026 infraBuilder SASU and contributors

// Package v1alpha1 implements parsing and validation of Recipe and
// Retriever documents at apiVersion recipe.tobby.dev/v1alpha1, as defined
// by RECIPE-SPEC.md in this repository.
//
// # Parsing
//
// [ParseRecipe], [ParseRetriever], and the kind-detecting [Parse] each
// accept one YAML document (JSON works too, being a YAML subset).
// Parsing is strict, as the specification requires (§4.3): the document is
// validated against the embedded JSON Schemas — which reject unknown
// fields — and against the draft-level semantic rules the specification
// delegates to tooling (§16), such as ingredient name uniqueness and the
// version constraint grammar. A misspelled field like "digset:" is an
// error, never silently ignored.
//
//	r, err := v1alpha1.ParseRecipe(data)
//	if err != nil {
//		var errs v1alpha1.ErrorList
//		if errors.As(err, &errs) {
//			for _, fe := range errs {
//				fmt.Println(fe.Path, fe.Rule, fe.Message) // e.g. spec.ingredients[2].digest
//			}
//		}
//		return err
//	}
//
// # Draft and cooked recipes
//
// A parsed recipe is always a valid draft (§8). Before publication to a
// cookbook, check it against the cooked profile, which requires every
// ingredient to carry a content digest and an exact-tag version:
//
//	if err := r.Validate(v1alpha1.ProfileCooked); err != nil {
//		// not publishable: missing digests or unresolved constraints
//	}
//
// [Recipe.Validate] and [Retriever.Validate] also work on values built in
// Go rather than parsed, revalidating them in full against the schema; use
// them before marshaling a generated document.
//
// # Version constraints
//
// [ParseConstraint] implements the version grammar of §9: exact OCI tags,
// wildcards ("12.x", "*"), caret ("^1.4.0"), tilde ("~1.4.2"), comparisons
// (">=2.0.0", "!=1.5.0"), and comma- or space-separated conjunctions.
// Disjunction ("||") is rejected, as v1alpha1 does not support it.
// [Constraint.Match] tests one tag; [Constraint.Resolve] applies the §9.2
// resolution rules (non-semver tags ignored, pre-releases excluded unless
// mentioned, highest match wins, no silent fallback) to a tag list.
//
// # Errors
//
// Every validation failure is reported as an [ErrorList] of [FieldError]
// values, each carrying the dotted path of the offending field (for
// example "spec.ingredients[2].digest"), a stable rule identifier (the
// Rule* constants), and an actionable message.
//
// One caveat inherited from YAML: unquoted scalars that look like
// timestamps are decoded as timestamps before validation, so annotation
// values such as dates should be quoted in source documents (the examples
// in this repository do so).
package v1alpha1
