// SPDX-License-Identifier: Apache-2.0
// Copyright © 2026 infraBuilder SASU and contributors

package v1alpha1

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"gopkg.in/yaml.v3"

	"github.com/tobby-fetch/recipe-spec/schemas"
)

// Parsing is deliberately strict (RECIPE-SPEC.md §4.3): the input is
// bounded by MaxParseBytes, then the YAML document is decoded generically,
// normalized to its JSON representation (§5 requires documents to be
// JSON-representable), validated against the embedded JSON Schema — which
// rejects unknown fields — and only then decoded into the typed structs.
// Documents that parse successfully also satisfy the draft-level semantic
// rules delegated to tooling by §16 (ingredient name uniqueness, version
// grammar); use Recipe.Validate with ProfileCooked to additionally require
// full pinning (§8).

// MaxParseBytes bounds the input accepted by Parse, ParseRecipe, and
// ParseRetriever: 4 MiB, the §5 document bound, which is also the §11.2
// bound on a published document layer (cookbook.MaxDocumentBytes). A
// recipe is a small YAML file; without this bound a hostile or accidental
// multi-megabyte input would cost hundreds of megabytes of memory before
// validation could say anything about it. Larger input is rejected
// immediately with RuleDocumentTooLarge, before any decoding.
const MaxParseBytes = 4 << 20

// ParseRecipe parses and validates a single YAML (or JSON) document of kind
// Recipe. On failure it returns an ErrorList describing every violation.
// A successfully parsed recipe is a valid draft (§8); call
// Validate(ProfileCooked) to check it against the cooked profile.
func ParseRecipe(data []byte) (*Recipe, error) {
	obj, err := parseDocument(data, KindRecipe)
	if err != nil {
		return nil, err
	}
	return obj.(*Recipe), nil
}

// ParseRetriever parses and validates a single YAML (or JSON) document of
// kind Retriever. On failure it returns an ErrorList describing every
// violation.
func ParseRetriever(data []byte) (*Retriever, error) {
	obj, err := parseDocument(data, KindRetriever)
	if err != nil {
		return nil, err
	}
	return obj.(*Retriever), nil
}

// Parse parses a single YAML (or JSON) document of any supported kind,
// detected from its kind field. The result is a *Recipe or a *Retriever.
// Documents with an unsupported apiVersion or kind are rejected with a
// clear error (§4.2).
func Parse(data []byte) (Object, error) {
	return parseDocument(data, "")
}

// parseDocument implements ParseRecipe, ParseRetriever, and Parse.
// wantKind restricts the accepted kind; empty means any supported kind.
func parseDocument(data []byte, wantKind string) (Object, error) {
	if len(data) > MaxParseBytes {
		return nil, ErrorList{{
			Rule: RuleDocumentTooLarge,
			Message: fmt.Sprintf("document is %d bytes, above the %d-byte bound on a recipe document (§5); refusing to decode it",
				len(data), MaxParseBytes),
		}}
	}
	root, jsonBytes, instance, errs := decodeDocument(data)
	if errs != nil {
		return nil, errs.errOrNil()
	}
	kindName, errs := checkEnvelope(root, wantKind)
	if errs != nil {
		return nil, errs.errOrNil()
	}
	sch, err := compiledSchema(kindName)
	if err != nil {
		return nil, ErrorList{{Rule: RuleInternal, Message: err.Error()}}
	}
	if err := sch.Validate(instance); err != nil {
		return nil, schemaErrorList(err).errOrNil()
	}
	dec := json.NewDecoder(bytes.NewReader(jsonBytes))
	dec.DisallowUnknownFields()
	var obj Object
	switch kindName {
	case KindRecipe:
		r := &Recipe{}
		if err := dec.Decode(r); err != nil {
			return nil, internalDecodeError(err)
		}
		if errs := recipeSemanticErrors(r); len(errs) > 0 {
			return nil, errs
		}
		obj = r
	case KindRetriever:
		rt := &Retriever{}
		if err := dec.Decode(rt); err != nil {
			return nil, internalDecodeError(err)
		}
		if errs := retrieverSemanticErrors(rt); len(errs) > 0 {
			return nil, errs
		}
		obj = rt
	}
	return obj, nil
}

func internalDecodeError(err error) ErrorList {
	return ErrorList{{
		Rule:    RuleInternal,
		Message: fmt.Sprintf("document passed schema validation but could not be decoded into the SDK types (schema/SDK inconsistency, please report): %v", err),
	}}
}

// decodeDocument decodes exactly one YAML document and returns its root
// mapping, its canonical JSON encoding, and the decoded JSON instance used
// for schema validation.
func decodeDocument(data []byte) (root map[string]any, jsonBytes []byte, instance any, errs ErrorList) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var raw any
	if err := dec.Decode(&raw); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil, nil, ErrorList{{Rule: RuleEmptyDocument, Message: "the input contains no document"}}
		}
		return nil, nil, nil, ErrorList{{Rule: RuleYAMLSyntax, Message: fmt.Sprintf("not well-formed YAML: %v", err)}}
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, nil, nil, ErrorList{{Rule: RuleMultipleDocuments, Message: "the input is a multi-document YAML stream; provide exactly one document per call (§5)"}}
		}
		return nil, nil, nil, ErrorList{{Rule: RuleYAMLSyntax, Message: fmt.Sprintf("not well-formed YAML after the first document: %v", err)}}
	}
	if raw == nil {
		return nil, nil, nil, ErrorList{{Rule: RuleEmptyDocument, Message: "the document is empty"}}
	}
	normalized, err := toJSONValue(raw)
	if err != nil {
		return nil, nil, nil, ErrorList{{Rule: RuleYAMLSyntax, Message: err.Error()}}
	}
	jsonBytes, err = json.Marshal(normalized)
	if err != nil {
		return nil, nil, nil, ErrorList{{Rule: RuleYAMLSyntax, Message: fmt.Sprintf("document is not representable as JSON (§5): %v", err)}}
	}
	if err := json.Unmarshal(jsonBytes, &instance); err != nil {
		return nil, nil, nil, ErrorList{{Rule: RuleInternal, Message: fmt.Sprintf("JSON round-trip failed: %v", err)}}
	}
	root, ok := instance.(map[string]any)
	if !ok {
		return nil, nil, nil, ErrorList{{Rule: RuleDocumentRoot, Message: "the document root must be a mapping with apiVersion, kind, metadata, and spec (§5)"}}
	}
	return root, jsonBytes, instance, nil
}

// toJSONValue rewrites the generic YAML decoding into JSON-compatible
// values, rejecting YAML-specific shapes that §5 forbids (non-string
// mapping keys).
func toJSONValue(v any) (any, error) {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			conv, err := toJSONValue(val)
			if err != nil {
				return nil, err
			}
			out[k] = conv
		}
		return out, nil
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			ks, ok := k.(string)
			if !ok {
				return nil, fmt.Errorf("mapping key %v is not a string; documents must be representable as JSON (§5)", k)
			}
			conv, err := toJSONValue(val)
			if err != nil {
				return nil, err
			}
			out[ks] = conv
		}
		return out, nil
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			conv, err := toJSONValue(val)
			if err != nil {
				return nil, err
			}
			out[i] = conv
		}
		return out, nil
	default:
		return v, nil
	}
}

// checkEnvelope validates apiVersion and kind before schema validation, so
// that unsupported documents fail with a targeted message rather than a
// generic schema error (§4.2: implementations must reject documents whose
// apiVersion they do not support).
func checkEnvelope(root map[string]any, wantKind string) (string, ErrorList) {
	var errs ErrorList
	if av, ok := root["apiVersion"]; !ok {
		errs = append(errs, &FieldError{Path: "apiVersion", Rule: RuleAPIVersion, Message: "missing required field"})
	} else if avs, ok := av.(string); !ok {
		errs = append(errs, &FieldError{Path: "apiVersion", Rule: RuleAPIVersion, Message: "must be a string"})
	} else if avs != APIVersion {
		errs = append(errs, &FieldError{
			Path: "apiVersion", Rule: RuleAPIVersion,
			Message: fmt.Sprintf("unsupported apiVersion %q: this package supports %q only (§4.2)", avs, APIVersion),
		})
	}
	kindName := ""
	k, present := root["kind"]
	ks, isString := k.(string)
	switch {
	case !present:
		errs = append(errs, &FieldError{Path: "kind", Rule: RuleKind, Message: "missing required field"})
	case !isString:
		errs = append(errs, &FieldError{Path: "kind", Rule: RuleKind, Message: "must be a string"})
	case ks != KindRecipe && ks != KindRetriever:
		errs = append(errs, &FieldError{
			Path: "kind", Rule: RuleKind,
			Message: fmt.Sprintf("unknown kind %q: supported kinds are %q and %q (§5)", ks, KindRecipe, KindRetriever),
		})
	case wantKind != "" && ks != wantKind:
		errs = append(errs, &FieldError{
			Path: "kind", Rule: RuleKind,
			Message: fmt.Sprintf("document has kind %q, expected %q", ks, wantKind),
		})
	default:
		kindName = ks
	}
	if len(errs) > 0 {
		return "", errs
	}
	return kindName, nil
}

// Compiled schemas are process-wide singletons: compilation is pure,
// deterministic, and fed exclusively by the embedded schema documents.
var (
	schemaOnce      sync.Once
	recipeSchema    *jsonschema.Schema
	retrieverSchema *jsonschema.Schema
	schemaErr       error
)

func compiledSchema(kindName string) (*jsonschema.Schema, error) {
	schemaOnce.Do(func() {
		recipeSchema, schemaErr = compileSchema(schemas.RecipeSchemaID, schemas.RecipeSchemaJSON)
		if schemaErr != nil {
			return
		}
		retrieverSchema, schemaErr = compileSchema(schemas.RetrieverSchemaID, schemas.RetrieverSchemaJSON)
	})
	if schemaErr != nil {
		return nil, fmt.Errorf("embedded JSON Schema failed to compile (please report): %w", schemaErr)
	}
	if kindName == KindRetriever {
		return retrieverSchema, nil
	}
	return recipeSchema, nil
}

func compileSchema(id string, raw []byte) (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", id, err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(id, doc); err != nil {
		return nil, fmt.Errorf("%s: %w", id, err)
	}
	return c.Compile(id)
}

// schemaMessagePrinter renders the validator's localized messages.
var schemaMessagePrinter = message.NewPrinter(language.English)

// schemaErrorList flattens a jsonschema validation error into one
// FieldError per leaf violation, each carrying the path of the offending
// field.
func schemaErrorList(err error) ErrorList {
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return ErrorList{{Rule: RuleInternal, Message: fmt.Sprintf("unexpected schema validation failure: %v", err)}}
	}
	var errs ErrorList
	seen := make(map[string]bool)
	var walk func(e *jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if len(e.Causes) > 0 {
			for _, c := range e.Causes {
				walk(c)
			}
			return
		}
		fe := &FieldError{
			Path:    fieldPath(e.InstanceLocation),
			Rule:    RuleSchema,
			Message: schemaLeafMessage(e),
		}
		key := fe.Path + "\x00" + fe.Message
		if !seen[key] {
			seen[key] = true
			errs = append(errs, fe)
		}
	}
	walk(ve)
	if len(errs) == 0 {
		errs = append(errs, &FieldError{Rule: RuleSchema, Message: ve.Error()})
	}
	return errs
}

// ingredientKindOnlyFields maps each kind-specific ingredient field to the
// only ingredient kind that accepts it (§7). Used to turn the schema's
// bare "false schema" violation into an actionable message.
var ingredientKindOnlyFields = map[string]IngredientKind{
	"platforms":          "", // ContainerImage and FileSet
	"vendorDependencies": IngredientHelmChart,
	"artifactType":       IngredientOCIArtifact,
	"extract":            IngredientFileSet,
}

func schemaLeafMessage(e *jsonschema.ValidationError) string {
	if _, ok := e.ErrorKind.(*kind.FalseSchema); ok {
		if len(e.InstanceLocation) > 0 {
			field := e.InstanceLocation[len(e.InstanceLocation)-1]
			if only, isKindField := ingredientKindOnlyFields[field]; isKindField {
				allowed := "ContainerImage and FileSet ingredients"
				if only != "" {
					allowed = string(only) + " ingredients"
				}
				return fmt.Sprintf("field %q is only valid for %s (§7)", field, allowed)
			}
		}
	}
	if _, ok := e.ErrorKind.(*kind.Not); ok {
		// The validator's message for a violated `not` subschema is a bare
		// "'not' failed", which tells the author nothing. The published
		// schemas contain exactly one `not`: the ban on '..' components in
		// extract.paths entries (§7.4). Name that rule; keep the generic
		// message as a fallback should a future schema revision add
		// another `not` without extending this mapping.
		if instanceLocationHas(e.InstanceLocation, "paths") {
			return "path must not contain '..' components (§7.4)"
		}
	}
	return e.ErrorKind.LocalizedString(schemaMessagePrinter)
}

// instanceLocationHas reports whether one of the segments of a schema
// violation's instance location is the given field name.
func instanceLocationHas(segments []string, field string) bool {
	for _, seg := range segments {
		if seg == field {
			return true
		}
	}
	return false
}
