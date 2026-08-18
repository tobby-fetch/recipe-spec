// SPDX-License-Identifier: Apache-2.0
// Copyright © 2026 infraBuilder SASU and contributors

package cookbook

import (
	"encoding/json"
	"fmt"

	v1alpha1 "github.com/tobby-fetch/recipe-spec/recipe/v1alpha1"
)

// Artifact is a recipe ready to publish: the two blobs to upload and the
// manifest to PUT, each with the digest and size a registry will ask for.
type Artifact struct {
	// Manifest is the §11.2 image manifest. Its Digest is the artifact's
	// identity — the argument to `cosign sign`, which signs a digest,
	// never a tag.
	Manifest Blob
	// Config is the empty config blob, uploaded like any other.
	Config Blob
	// Document is the recipe YAML, the artifact's single layer.
	Document Blob
	// Recipe is the parsed document, validated under the cooked profile.
	Recipe *v1alpha1.Recipe
}

// Build validates a recipe document and assembles the §11.2 artifact that
// publishes it at repositoryLastSegment:tag.
//
// The validation is the point. Moving bytes into a registry is easy and
// every OCI client already does it; what nothing else does is refuse to
// move the wrong bytes. Build rejects
//   - a document that is not a valid Recipe (§5, §6),
//   - a recipe that is not fully pinned — a cookbook holds cooked recipes
//     only, so every ingredient carries a digest and an exact tag (§8),
//   - a recipe whose name or version contradicts where it is being
//     published (§11.3),
//   - a document above [MaxDocumentBytes], which no conforming consumer
//     would agree to read back.
//
// repositoryLastSegment is the last path segment of the destination
// repository and tag is the destination tag: for
// `registry.example.com/cookbook/wordpress:6.8.2`, "wordpress" and
// "6.8.2". Resolving a reference into those two strings is the caller's
// registry client's job.
//
// Signing is not done here, and not by any implementation of this format's
// tooling: the returned manifest digest is what a signer takes.
func Build(doc []byte, repositoryLastSegment, tag string) (*Artifact, error) {
	if len(doc) > MaxDocumentBytes {
		return nil, &LayoutError{
			Field:   "document",
			Message: fmt.Sprintf("is %d bytes, above the %d-byte bound on a recipe document", len(doc), MaxDocumentBytes),
		}
	}
	recipe, err := v1alpha1.ParseRecipe(doc)
	if err != nil {
		return nil, err
	}
	if err = recipe.Validate(v1alpha1.ProfileCooked); err != nil {
		return nil, err
	}
	if err = recipe.ValidatePublishLocation(repositoryLastSegment, tag); err != nil {
		return nil, err
	}

	config := Blob{MediaType: ConfigMediaType, Digest: digestOf(emptyConfig), Size: int64(len(emptyConfig)), Content: emptyConfig}
	document := Blob{MediaType: ArtifactType, Digest: digestOf(doc), Size: int64(len(doc)), Content: doc}

	// The manifest is written out field by field rather than assembled
	// through an OCI library's image helpers: the layout is fixed by
	// §11.2, and a literal is what a reader can check against the
	// specification line by line.
	raw, err := json.Marshal(manifest{
		SchemaVersion: 2,
		MediaType:     ManifestMediaType,
		Config:        descriptor{MediaType: config.MediaType, Size: config.Size, Digest: config.Digest},
		Layers: []descriptor{{
			MediaType:   document.MediaType,
			Size:        document.Size,
			Digest:      document.Digest,
			Annotations: map[string]string{titleAnnotation: LayerTitle},
		}},
		ArtifactType: ArtifactType,
	})
	if err != nil {
		// Unreachable: every field is a string, an int, or a map of
		// strings. Reported rather than dropped, per the SDK's habit of
		// never swallowing an impossibility.
		return nil, fmt.Errorf("encoding the recipe manifest: %w", err)
	}

	return &Artifact{
		Manifest: Blob{MediaType: ManifestMediaType, Digest: digestOf(raw), Size: int64(len(raw)), Content: raw},
		Config:   config,
		Document: document,
		Recipe:   recipe,
	}, nil
}

// Republication is what writing an artifact to a tag that already exists
// would mean (§8).
type Republication int

const (
	// RepublicationIdentical means the tag already points at this exact
	// content. Publishing again is a no-op, not a conflict: the same
	// document published twice is the same artifact, and refusing it
	// would make every retry an error.
	RepublicationIdentical Republication = iota
	// RepublicationConflict means the tag holds different content. Cooked
	// recipes are immutable: any change, even one digest, requires a new
	// metadata.version. The publisher must refuse.
	RepublicationConflict
)

// DecideRepublication reports what republishing means, given the digest a
// tag currently holds and the digest about to be written.
//
// Reading the published digest is a registry operation and stays with the
// caller; the rule that the two digests imply is the format's, and lives
// here so that every implementation refuses — and permits — the same
// things. Call it only when the tag exists: an absent tag is a plain first
// publication, which this verdict does not describe.
func DecideRepublication(published, candidate string) Republication {
	if published == candidate {
		return RepublicationIdentical
	}
	return RepublicationConflict
}
