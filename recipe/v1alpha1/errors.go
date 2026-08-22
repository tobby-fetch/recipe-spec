// SPDX-License-Identifier: Apache-2.0
// Copyright © 2026 infraBuilder SASU and contributors

package v1alpha1

import (
	"fmt"
	"strconv"
	"strings"
)

// Validation rule identifiers carried by FieldError.Rule. They are stable,
// machine-readable names for the specification rule that was violated.
const (
	// RuleYAMLSyntax: the input is not well-formed YAML (or not decodable
	// as a JSON-representable document, RECIPE-SPEC.md §5).
	RuleYAMLSyntax = "yaml-syntax"
	// RuleEmptyDocument: the input contains no document (empty input,
	// only comments, or a null document).
	RuleEmptyDocument = "empty-document"
	// RuleMultipleDocuments: the input is a multi-document YAML stream;
	// this package parses exactly one document per call (§5).
	RuleMultipleDocuments = "multiple-documents"
	// RuleDocumentTooLarge: the input exceeds MaxParseBytes, the §5 bound
	// on a recipe document. It is rejected before any decoding.
	RuleDocumentTooLarge = "document-too-large"
	// RuleDocumentRoot: the document root is not a mapping (§5).
	RuleDocumentRoot = "document-root"
	// RuleAPIVersion: apiVersion is missing, not a string, or not
	// supported by this package (§4.1, §4.2).
	RuleAPIVersion = "api-version"
	// RuleKind: kind is missing, unknown, or does not match the kind
	// requested by the caller (§5).
	RuleKind = "kind"
	// RuleSchema: the document violates the published JSON Schema,
	// including the strict-validation rejection of unknown fields (§4.3).
	RuleSchema = "schema"
	// RuleIngredientNameDuplicate: two ingredients share the same name
	// (§6.2).
	RuleIngredientNameDuplicate = "ingredient-name-duplicate"
	// RuleVersionSyntax: a version field is neither a valid exact OCI tag
	// nor a valid constraint expression (§9).
	RuleVersionSyntax = "version-syntax"
	// RulePortRange: a registry port in a ref or cookbook field is outside
	// 1-65535.
	RulePortRange = "port-range"
	// RuleCookedDigestMissing: an ingredient of a cooked recipe has no
	// digest (§8).
	RuleCookedDigestMissing = "cooked-digest-missing"
	// RuleCookedVersionNotExact: an ingredient of a cooked recipe uses a
	// constraint expression instead of an exact tag (§8).
	RuleCookedVersionNotExact = "cooked-version-not-exact"
	// RuleProfile: the validation profile requested by the caller is
	// unknown.
	RuleProfile = "profile"
	// RulePublishLocation: metadata does not agree with the publication
	// location (repository last path segment and tag, §11.3).
	RulePublishLocation = "publish-location"
	// RuleInternal: the SDK detected an internal inconsistency (for
	// example between the embedded schemas and the Go types). This should
	// never happen; please report it.
	RuleInternal = "internal"
)

// FieldError is one validation error, tied to the document field that
// violates a rule.
type FieldError struct {
	// Path locates the offending field in the document, in dotted form
	// with array indices, e.g. "spec.ingredients[2].digest". It is empty
	// when the error concerns the document as a whole.
	Path string
	// Rule is the stable identifier of the violated rule (one of the
	// Rule* constants).
	Rule string
	// Message is a human-readable explanation of the violation.
	Message string
}

// Error implements the error interface.
func (e *FieldError) Error() string {
	if e.Path == "" {
		return fmt.Sprintf("%s (rule %s)", e.Message, e.Rule)
	}
	return fmt.Sprintf("%s: %s (rule %s)", e.Path, e.Message, e.Rule)
}

// ErrorList aggregates the field errors found in one document. It
// implements error and unwraps to the individual *FieldError values, so it
// composes with errors.As and errors.Is:
//
//	var list v1alpha1.ErrorList
//	if errors.As(err, &list) {
//		for _, fe := range list { ... }
//	}
type ErrorList []*FieldError

// Error implements the error interface.
func (l ErrorList) Error() string {
	switch len(l) {
	case 0:
		return "no validation errors"
	case 1:
		return l[0].Error()
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%d validation errors:", len(l))
	for _, fe := range l {
		sb.WriteString("\n  - ")
		sb.WriteString(fe.Error())
	}
	return sb.String()
}

// Unwrap makes the aggregated errors visible to errors.Is and errors.As.
func (l ErrorList) Unwrap() []error {
	errs := make([]error, len(l))
	for i, fe := range l {
		errs[i] = fe
	}
	return errs
}

// ByRule returns the subset of errors that violate the given rule (one of
// the Rule* constants).
func (l ErrorList) ByRule(rule string) ErrorList {
	var out ErrorList
	for _, fe := range l {
		if fe.Rule == rule {
			out = append(out, fe)
		}
	}
	return out
}

// errOrNil returns the list as an error, or nil when it is empty. It exists
// so that functions never return a non-nil error interface holding an empty
// list.
func (l ErrorList) errOrNil() error {
	if len(l) == 0 {
		return nil
	}
	return l
}

// fieldPath renders a JSON instance location (path segments as produced by
// the schema validator) in dotted form: ["spec","ingredients","2","digest"]
// becomes "spec.ingredients[2].digest".
func fieldPath(segments []string) string {
	var sb strings.Builder
	for _, seg := range segments {
		if _, err := strconv.Atoi(seg); err == nil && seg != "" {
			fmt.Fprintf(&sb, "[%s]", seg)
			continue
		}
		if sb.Len() > 0 {
			sb.WriteByte('.')
		}
		sb.WriteString(seg)
	}
	return sb.String()
}

// ingredientPath builds the dotted path of an ingredient field:
// ingredientPath(2, "digest") returns "spec.ingredients[2].digest".
func ingredientPath(index int, field string) string {
	p := fmt.Sprintf("spec.ingredients[%d]", index)
	if field != "" {
		p += "." + field
	}
	return p
}
