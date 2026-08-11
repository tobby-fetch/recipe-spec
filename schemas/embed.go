// SPDX-License-Identifier: Apache-2.0
// Copyright © 2026 infraBuilder SASU and contributors

// Package schemas embeds the JSON Schemas (draft 2020-12) published by the
// Recipe specification, so that Go consumers can validate documents without
// any filesystem or network access.
//
// The schemas are strict: they reject unknown fields (additionalProperties:
// false), as required by RECIPE-SPEC.md §4.3. Rules that JSON Schema cannot
// express (ingredient name uniqueness, the cooked profile, constraint
// grammar, publication-location consistency) are enforced by the
// recipe/v1alpha1 package.
package schemas

import _ "embed"

// Canonical identifiers ($id) of the embedded schemas.
const (
	// RecipeSchemaID is the $id of the Recipe JSON Schema.
	RecipeSchemaID = "https://tobby.dev/schemas/v1alpha1/recipe.schema.json"
	// RetrieverSchemaID is the $id of the Retriever JSON Schema.
	RetrieverSchemaID = "https://tobby.dev/schemas/v1alpha1/retriever.schema.json"
)

// RecipeSchemaJSON is the raw JSON Schema for documents of kind Recipe
// (schemas/recipe.schema.json).
//
//go:embed recipe.schema.json
var RecipeSchemaJSON []byte

// RetrieverSchemaJSON is the raw JSON Schema for documents of kind Retriever
// (schemas/retriever.schema.json).
//
//go:embed retriever.schema.json
var RetrieverSchemaJSON []byte
