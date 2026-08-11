// SPDX-License-Identifier: Apache-2.0
// Copyright © 2026 infraBuilder SASU and contributors

package v1alpha1

// APIVersion is the only apiVersion accepted by this package:
// the recipe.tobby.dev API group at version v1alpha1.
const APIVersion = "recipe.tobby.dev/v1alpha1"

// Document kinds defined by the specification (RECIPE-SPEC.md §5).
const (
	// KindRecipe identifies documents of kind Recipe (§6).
	KindRecipe = "Recipe"
	// KindRetriever identifies documents of kind Retriever (§10).
	KindRetriever = "Retriever"
)

// IngredientKind is the kind of one OCI artifact referenced by a recipe
// (RECIPE-SPEC.md §7).
type IngredientKind string

// Ingredient kinds defined by the specification (§7).
const (
	// IngredientContainerImage is a runnable container image, possibly
	// multi-platform (§7.1).
	IngredientContainerImage IngredientKind = "ContainerImage"
	// IngredientHelmChart is a Helm chart stored as an OCI artifact (§7.2).
	IngredientHelmChart IngredientKind = "HelmChart"
	// IngredientOCIArtifact is any other OCI artifact: AI models, database
	// bundles, SBOM collections, WASM modules... (§7.3).
	IngredientOCIArtifact IngredientKind = "OCIArtifact"
	// IngredientFileSet is a set of arbitrary files packaged as an OCI
	// image, mountable or extractable (§7.4).
	IngredientFileSet IngredientKind = "FileSet"
)

// Object is a parsed specification document: either a *Recipe or a
// *Retriever. Use a type switch to recover the concrete type after Parse.
type Object interface {
	// ObjectKind returns the document kind: KindRecipe or KindRetriever.
	ObjectKind() string
}

// Recipe is a declarative, signable, fully pinnable description of a set of
// OCI artifacts (ingredients) that together make up a software delivery
// (RECIPE-SPEC.md §6).
type Recipe struct {
	// APIVersion declares the schema version of the document. It must be
	// "recipe.tobby.dev/v1alpha1" (§4).
	APIVersion string `json:"apiVersion" yaml:"apiVersion"`
	// Kind is always "Recipe" for this type (§5).
	Kind string `json:"kind" yaml:"kind"`
	// Metadata carries the identity of the recipe (§6.1).
	Metadata Metadata `json:"metadata" yaml:"metadata"`
	// Spec is the recipe content (§6.2).
	Spec RecipeSpec `json:"spec" yaml:"spec"`
}

// ObjectKind implements Object.
func (r *Recipe) ObjectKind() string { return KindRecipe }

// Retriever declares the desired recipe set for one network zone
// (RECIPE-SPEC.md §10).
type Retriever struct {
	// APIVersion declares the schema version of the document. It must be
	// "recipe.tobby.dev/v1alpha1" (§4).
	APIVersion string `json:"apiVersion" yaml:"apiVersion"`
	// Kind is always "Retriever" for this type (§5).
	Kind string `json:"kind" yaml:"kind"`
	// Metadata carries the identity of the retriever (§10.2). A retriever
	// has no metadata version: it is living configuration, not a published
	// versioned artifact, so Metadata.Version must be left empty.
	Metadata Metadata `json:"metadata" yaml:"metadata"`
	// Spec is the desired state of the zone (§10.2).
	Spec RetrieverSpec `json:"spec" yaml:"spec"`
}

// ObjectKind implements Object.
func (r *Retriever) ObjectKind() string { return KindRetriever }

// Metadata is the identity and metadata block shared by Recipe and Retriever
// documents (RECIPE-SPEC.md §6.1 and §10.2).
type Metadata struct {
	// Name is the document name. It must be a valid OCI repository path
	// segment: lowercase alphanumerics with '.', '_', '-' separators, at
	// most 63 characters. For a recipe it becomes the last path segment of
	// the cookbook repository on publication (§11.3).
	Name string `json:"name" yaml:"name"`
	// Version is the version of the described application (not of any
	// single ingredient). It must be Semantic Versioning 2.0.0 without
	// build metadata ('+' is not a legal OCI tag character) and becomes
	// the OCI tag on publication. Recipe only: a Retriever has no
	// metadata version and must leave this field empty.
	Version string `json:"version,omitempty" yaml:"version,omitempty"`
	// Description is a human-readable, one-paragraph description.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	// Labels are identifying key/value pairs intended for selection and
	// filtering (Kubernetes label syntax; values at most 63 characters).
	Labels map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	// Annotations are non-identifying metadata (provenance URLs,
	// timestamps, tooling data). Keys should be namespaced with a DNS
	// prefix; keys under "recipe.tobby.dev/" are reserved by the
	// specification.
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

// Reserved annotation keys defined by this revision of the specification
// (RECIPE-SPEC.md §6.1).
const (
	// AnnotationCookedAt is the RFC 3339 timestamp at which the recipe was
	// cooked.
	AnnotationCookedAt = "recipe.tobby.dev/cooked-at"
	// AnnotationUpstreamDigestPrefix, suffixed with an ingredient name,
	// records the digest of the upstream artifact when the pinned artifact
	// was rewritten by tooling (e.g. Helm chart dependency vendoring §7.2).
	AnnotationUpstreamDigestPrefix = "recipe.tobby.dev/upstream-digest."
)

// RecipeSpec is the content of a Recipe (RECIPE-SPEC.md §6.2).
type RecipeSpec struct {
	// Ingredients is the non-empty list of OCI artifacts this recipe is
	// made of. Ingredient names must be unique within the recipe.
	Ingredients []Ingredient `json:"ingredients" yaml:"ingredients"`
}

// Ingredient describes one OCI artifact referenced by a recipe
// (RECIPE-SPEC.md §6.2 and §7).
type Ingredient struct {
	// Name is the ingredient name, unique within the recipe (DNS label,
	// at most 63 characters). Used in logs, status reports, and reserved
	// annotation keys.
	Name string `json:"name" yaml:"name"`
	// Kind is one of ContainerImage, HelmChart, OCIArtifact, FileSet (§7).
	Kind IngredientKind `json:"kind" yaml:"kind"`
	// Ref is the fully qualified source OCI reference WITHOUT tag or
	// digest: "<registry-host>[:<port>]/<repository-path>". There is no
	// default registry, and no scheme, userinfo, tag, or digest is
	// allowed: version and digest have dedicated fields (§6.2).
	Ref string `json:"ref" yaml:"ref"`
	// Version is an exact OCI tag, or a version constraint expression
	// (§9). Cooked recipes must use an exact tag (§8).
	Version string `json:"version" yaml:"version"`
	// Digest is the content digest of the artifact's top-level manifest
	// (the image index for multi-platform images), e.g. "sha256:...".
	// Optional in drafts, required in cooked recipes (§8). Registered
	// algorithms: sha256, sha512.
	Digest string `json:"digest,omitempty" yaml:"digest,omitempty"`
	// Platforms (ContainerImage and FileSet only) lists the platforms
	// required at the destination, as "os/arch[/variant]". If absent, all
	// platforms are transferred (§7.1).
	Platforms []string `json:"platforms,omitempty" yaml:"platforms,omitempty"`
	// VendorDependencies (HelmChart only) selects whether the chart is
	// made self-contained at cook time (§7.2). Defaults to false. The
	// pointer distinguishes an absent field from an explicit false so
	// that documents round-trip unchanged.
	VendorDependencies *bool `json:"vendorDependencies,omitempty" yaml:"vendorDependencies,omitempty"`
	// ArtifactType (OCIArtifact only) is the expected artifact type
	// (media type syntax) of the fetched manifest; a mismatch fails the
	// ingredient (§7.3).
	ArtifactType string `json:"artifactType,omitempty" yaml:"artifactType,omitempty"`
	// Extract (FileSet only) carries extraction hints for consumers; it
	// does not change what is transferred (§7.4).
	Extract *Extract `json:"extract,omitempty" yaml:"extract,omitempty"`
}

// VendorsDependencies reports the effective value of VendorDependencies,
// applying the specification default (false) when the field is absent.
func (i *Ingredient) VendorsDependencies() bool {
	return i.VendorDependencies != nil && *i.VendorDependencies
}

// Extract carries FileSet extraction hints (RECIPE-SPEC.md §7.4).
type Extract struct {
	// Paths is a non-empty list of slash-separated glob patterns ('*',
	// '**'), relative to the image root, selecting what to materialize on
	// extraction. Absent paths mean the whole root filesystem. Absolute
	// paths and '..' components are forbidden.
	Paths []string `json:"paths,omitempty" yaml:"paths,omitempty"`
}

// RetrieverSpec is the desired state of one zone (RECIPE-SPEC.md §10.2).
type RetrieverSpec struct {
	// Cookbook is the default cookbook to resolve recipes from:
	// "<registry-host>[:<port>]/<repository-path>" (same syntax rules as
	// an ingredient Ref).
	Cookbook string `json:"cookbook" yaml:"cookbook"`
	// Recipes is the non-empty list of desired recipes. Each entry
	// resolves independently against its cookbook's tags.
	Recipes []RecipeSelector `json:"recipes" yaml:"recipes"`
}

// RecipeSelector selects one recipe (by name) at an exact version or at the
// highest version matching a constraint (RECIPE-SPEC.md §10.2).
type RecipeSelector struct {
	// Name is the recipe name as published in the cookbook.
	Name string `json:"name" yaml:"name"`
	// Version is an exact recipe version or a constraint expression (§9),
	// resolved against the cookbook's tags for that recipe.
	Version string `json:"version" yaml:"version"`
	// Cookbook optionally overrides the retriever's spec.cookbook for
	// this entry, with the same syntax rules.
	Cookbook string `json:"cookbook,omitempty" yaml:"cookbook,omitempty"`
}
