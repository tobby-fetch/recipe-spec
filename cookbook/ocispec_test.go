// SPDX-License-Identifier: Apache-2.0
// Copyright © 2026 infraBuilder SASU and contributors

package cookbook_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"

	digest "github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/tobby-fetch/recipe-spec/cookbook"
)

// image-spec is a TEST-ONLY dependency: nothing under cookbook/ (or any
// other non-test file of this module) imports it. It is here as the
// reference implementation this package claims wire compatibility with.

// TestBuildMatchesOCIImageSpecSerialization pins the interoperability claim
// of the manifest serializer: the same content run through the OCI
// reference structs (github.com/opencontainers/image-spec/specs-go/v1, the
// structs oras-go and go-containerregistry serialize with) must yield the
// same bytes — and therefore the same manifest digest. A divergence here
// means a recipe published by Build and one published by a generic OCI
// library would disagree on field order, which is exactly the bug this
// test exists to catch.
func TestBuildMatchesOCIImageSpecSerialization(t *testing.T) {
	art, err := cookbook.Build([]byte(cookedDoc), "demo", "1.0.0")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	reference := ocispec.Manifest{
		Versioned:    specs.Versioned{SchemaVersion: 2},
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: cookbook.ArtifactType,
		Config: ocispec.Descriptor{
			MediaType: cookbook.ConfigMediaType,
			Digest:    ocispec.DescriptorEmptyJSON.Digest,
			Size:      ocispec.DescriptorEmptyJSON.Size,
		},
		Layers: []ocispec.Descriptor{{
			MediaType: cookbook.ArtifactType,
			Digest:    ocispec.DescriptorEmptyJSON.Digest, // placeholder, set below
			Size:      art.Document.Size,
			Annotations: map[string]string{
				ocispec.AnnotationTitle: cookbook.LayerTitle,
			},
		}},
	}
	// The document digest is computed independently of the code under
	// test, from the same input bytes.
	reference.Layers[0].Digest = digestOfBytes([]byte(cookedDoc))

	raw, err := json.Marshal(reference)
	if err != nil {
		t.Fatalf("marshaling the reference manifest: %v", err)
	}

	if !bytes.Equal(raw, art.Manifest.Content) {
		t.Errorf("Build's manifest bytes differ from the OCI reference serialization:\nbuild: %s\nocispec: %s",
			art.Manifest.Content, raw)
	}
	if got, want := art.Manifest.Digest, digestOfBytes(raw).String(); got != want {
		t.Errorf("manifest digest = %s, ocispec serialization digests to %s", got, want)
	}
	// The §11.2 media-type constants must agree with the reference ones.
	if cookbook.ManifestMediaType != ocispec.MediaTypeImageManifest {
		t.Errorf("ManifestMediaType = %q, ocispec says %q", cookbook.ManifestMediaType, ocispec.MediaTypeImageManifest)
	}
	if cookbook.ConfigMediaType != ocispec.MediaTypeEmptyJSON {
		t.Errorf("ConfigMediaType = %q, ocispec says %q", cookbook.ConfigMediaType, ocispec.MediaTypeEmptyJSON)
	}
	if got, want := art.Config.Digest, string(ocispec.DescriptorEmptyJSON.Digest); got != want {
		t.Errorf("config digest = %s, ocispec's canonical empty JSON digest is %s", got, want)
	}
}

// digestOfBytes computes a sha256 digest independently of the code under
// test, in the string type the ocispec descriptor carries.
func digestOfBytes(payload []byte) digest.Digest {
	return digest.Digest(fmt.Sprintf("sha256:%x", sha256.Sum256(payload)))
}
