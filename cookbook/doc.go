// SPDX-License-Identifier: Apache-2.0
// Copyright © 2026 infraBuilder SASU and contributors

// Package cookbook implements the parts of RECIPE-SPEC.md §11 that need no
// registry: what a published recipe artifact must look like, where it may
// be published, and what republishing an existing tag means.
//
// The specification splits cleanly in two. "What the artifact is" is a
// property of the format and belongs here, so that every implementation
// agrees on it. "How to talk to a registry" is the consumer's business:
// this package neither opens a connection nor knows what a registry is.
// It takes bytes and returns bytes.
//
// # Publishing
//
// [Build] validates a recipe document and assembles the §11.2 artifact for
// a given publication location. It refuses a document that is not a valid
// cooked recipe (§8), and one whose metadata disagrees with where it is
// about to be published (§11.3):
//
//	art, err := cookbook.Build(doc, "wordpress", "6.8.2")
//	if err != nil {
//		return err // not publishable, and it says why
//	}
//	// Upload art.Config and art.Document as blobs, then PUT art.Manifest.
//	// art.Manifest.Digest is what `cosign sign` takes.
//
// The caller uploads the two blobs and the manifest with whatever registry
// client it already has.
//
// # Republishing
//
// Cooked recipes are immutable (§8): a `(name, version)` tag must never
// point at a different recipe document. The comparison is made on the
// document layer digest, not the manifest digest: manifest bytes are not
// stable across publishing tools (§11.2), the layer is. Before writing, a
// publisher fetches the tag's current manifest — a registry operation, so
// the caller's — reads its layout, and asks [DecideRepublication] what
// republishing means:
//
//	layout, err := cookbook.VerifyManifest(publishedManifest)
//	if err != nil {
//		return err // whatever holds the tag is not a recipe artifact
//	}
//	switch cookbook.DecideRepublication(layout.Document.Digest, art.Document.Digest) {
//	case cookbook.RepublicationIdentical:
//		return nil // same document already published: a no-op, leave the tag alone
//	case cookbook.RepublicationConflict:
//		return fmt.Errorf("tag already holds document %s; bump metadata.version", layout.Document.Digest)
//	}
//
// # Consuming
//
// [VerifyManifest] enforces the same §11.2 layout on the reading side —
// consumers MUST reject artifacts that do not match it. Verifying the
// signature over the manifest digest comes first, before the document is
// fetched or trusted; this package sits after that step and before the
// document is parsed.
//
//	layout, err := cookbook.VerifyManifest(manifestBytes)
//	if err != nil {
//		var le *cookbook.LayoutError
//		if errors.As(err, &le) {
//			// le.Field names the offending part of the manifest
//		}
//		return err
//	}
//	// Fetch layout.Document.Digest, bounded by layout.Document.Size.
//
// # Format versions
//
// The artifact envelope is versioned by its media type, independently of
// the document schema (§4.4): [ArtifactType] carries `v1` and is expected
// to stay stable across `v1alpha1`, `v1beta1`, and `v1`. This package is
// therefore not versioned alongside the document packages; it dispatches
// on the document's own apiVersion.
package cookbook
