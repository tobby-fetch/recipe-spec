// SPDX-License-Identifier: Apache-2.0
// Copyright © 2026 infraBuilder SASU and contributors

package cookbook

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
)

// The media types of the §11.2 artifact layout. The `v1` in the recipe
// media type versions the envelope — a single YAML document as sole layer
// — not the document schema, which apiVersion versions (§4.4).
const (
	// ArtifactType is the manifest's artifactType and the media type of
	// its single layer.
	ArtifactType = "application/vnd.tobby.recipe.v1+yaml"
	// ConfigMediaType is the OCI empty config every recipe artifact
	// carries in place of an image configuration.
	ConfigMediaType = "application/vnd.oci.empty.v1+json"
	// ManifestMediaType is the OCI image manifest media type of the
	// envelope.
	ManifestMediaType = "application/vnd.oci.image.manifest.v1+json"
)

// LayerTitle is the file name written on the document layer.
//
// §11.2 shows the `org.opencontainers.image.title` annotation in its
// example manifest but states no normative rule for it, and §11.2 also
// promises compatibility with generic OCI tooling — where the title is
// whatever file name the operator happened to push. So publishers write
// it, for `oras pull` to land a sensibly named file, and consumers must
// not depend on it: [VerifyManifest] reports it without requiring it.
const LayerTitle = "recipe.yaml"

// titleAnnotation is the OCI annotation key carrying LayerTitle.
const titleAnnotation = "org.opencontainers.image.title"

// MaxDocumentBytes bounds the recipe document layer. A recipe is a small
// YAML file; anything larger is hostile or wrong, on either side of the
// wire — [Build] refuses to publish one, and [VerifyManifest] refuses to
// let a consumer start the download.
const MaxDocumentBytes = 4 << 20

// emptyConfig is the canonical OCI empty config payload: the two bytes
// "{}", digest sha256:44136fa3…, size 2 (§11.2).
var emptyConfig = []byte("{}")

// digestPattern is the set of well-formed digests, mirroring the pattern
// of the recipe schema's digest field: the two registered algorithms with
// their exact lowercase hexadecimal lengths. A manifest is attacker-supplied
// input; a descriptor digest that does not match is not something to pass
// on to a registry client as a blob reference.
var digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$|^sha512:[a-f0-9]{128}$`)

// Blob is one content-addressed payload of the artifact: the bytes, their
// media type, and the descriptor fields a registry needs to accept them.
type Blob struct {
	MediaType string
	// Digest is the payload's content digest, "sha256:" followed by lower
	// case hexadecimal.
	Digest string
	Size   int64
	// Content is the payload itself. For a Blob read back from a
	// published manifest it is nil: the manifest describes the bytes, it
	// does not carry them.
	Content []byte
}

// manifest mirrors the OCI image manifest, restricted to the fields the
// §11.2 layout uses.
//
// The field order is not cosmetic. It reproduces, byte for byte, the JSON
// that the OCI reference structs emit for the same content
// (github.com/opencontainers/image-spec/specs-go/v1, the structs behind
// oras-go and go-containerregistry), and matches the normative example of
// §11.2. That said, byte-identical manifests across tools are NOT a
// guarantee of the format: generic publishers are free to add manifest
// annotations (oras records org.opencontainers.image.created), so the
// recipe's stable identity is its document layer digest, not its manifest
// digest — see [DecideRepublication].
type manifest struct {
	SchemaVersion int          `json:"schemaVersion"`
	MediaType     string       `json:"mediaType"`
	ArtifactType  string       `json:"artifactType"`
	Config        descriptor   `json:"config"`
	Layers        []descriptor `json:"layers"`
}

// descriptor mirrors an OCI descriptor, in the same wire order.
type descriptor struct {
	MediaType   string            `json:"mediaType"`
	Digest      string            `json:"digest"`
	Size        int64             `json:"size"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// LayoutError reports a §11.2 violation: an artifact that is not shaped
// like a recipe. Consumers MUST reject such artifacts, and publishers must
// not create them.
type LayoutError struct {
	// Field names the offending part of the manifest in dotted notation
	// ("artifactType", "config.mediaType", "layers", "layers[0].size").
	Field string
	// Message states what is wrong and what was expected.
	Message string
}

func (e *LayoutError) Error() string {
	return fmt.Sprintf("recipe artifact layout: %s: %s", e.Field, e.Message)
}

// Layout is the §11.2 shape read back from a published manifest: where the
// document is and how big, so the caller can fetch it.
type Layout struct {
	// Config describes the empty config blob.
	Config Blob
	// Document describes the recipe document layer — the blob to fetch,
	// bounded by its own Size, which VerifyManifest has already checked
	// against MaxDocumentBytes.
	Document Blob
	// Title is the layer's file-name annotation, empty when the publisher
	// set none. Informational: nothing may depend on it (see LayerTitle).
	Title string
}

// VerifyManifest enforces the §11.2 layout on a published manifest and
// reports where the recipe document lives.
//
// It is the reading counterpart of [Build], and deliberately checks the
// same four things that make an artifact a recipe: the artifactType, an
// empty config, exactly one layer, and that layer's media type — plus
// everything a consumer is about to act on: the config must be the one
// canonical empty config of §11.2 (size 2, the known digest of "{}"),
// both descriptor digests must be well-formed (a digest is the consumer's
// next blob request; a malformed one must never reach a registry client),
// and the layer size must be positive and within [MaxDocumentBytes], so a
// consumer never starts a download it would have to abandon. It says
// nothing about the document itself: parsing and validating that is
// [github.com/tobby-fetch/recipe-spec/recipe/v1alpha1]'s job, once the
// bytes are in hand and their signature verified.
func VerifyManifest(manifestBytes []byte) (*Layout, error) {
	var m manifest
	if err := json.Unmarshal(manifestBytes, &m); err != nil {
		return nil, &LayoutError{Field: "manifest", Message: "unparseable JSON: " + err.Error()}
	}
	if m.ArtifactType != ArtifactType {
		return nil, &LayoutError{
			Field:   "artifactType",
			Message: fmt.Sprintf("is %q, want %q", m.ArtifactType, ArtifactType),
		}
	}
	if m.Config.MediaType != ConfigMediaType {
		return nil, &LayoutError{
			Field:   "config.mediaType",
			Message: fmt.Sprintf("is %q, want the empty config %q", m.Config.MediaType, ConfigMediaType),
		}
	}
	if m.Config.Digest != digestOf(emptyConfig) {
		return nil, &LayoutError{
			Field:   "config.digest",
			Message: fmt.Sprintf("is %q, want the canonical empty config digest %q (§11.2)", m.Config.Digest, digestOf(emptyConfig)),
		}
	}
	if m.Config.Size != int64(len(emptyConfig)) {
		return nil, &LayoutError{
			Field:   "config.size",
			Message: fmt.Sprintf("is %d, want %d — the canonical empty config (§11.2)", m.Config.Size, len(emptyConfig)),
		}
	}
	if len(m.Layers) != 1 {
		return nil, &LayoutError{
			Field:   "layers",
			Message: fmt.Sprintf("holds %d layers, want exactly one recipe document layer", len(m.Layers)),
		}
	}
	layer := m.Layers[0]
	if layer.MediaType != ArtifactType {
		return nil, &LayoutError{
			Field:   "layers[0].mediaType",
			Message: fmt.Sprintf("is %q, want %q", layer.MediaType, ArtifactType),
		}
	}
	if !digestPattern.MatchString(layer.Digest) {
		return nil, &LayoutError{
			Field:   "layers[0].digest",
			Message: fmt.Sprintf("%q is not a well-formed sha256 or sha512 digest", layer.Digest),
		}
	}
	if layer.Size <= 0 {
		return nil, &LayoutError{
			Field:   "layers[0].size",
			Message: fmt.Sprintf("is %d bytes, want a positive size — there is no empty recipe document", layer.Size),
		}
	}
	if layer.Size > MaxDocumentBytes {
		return nil, &LayoutError{
			Field:   "layers[0].size",
			Message: fmt.Sprintf("is %d bytes, above the %d-byte bound on a recipe document", layer.Size, MaxDocumentBytes),
		}
	}
	return &Layout{
		Config:   Blob{MediaType: m.Config.MediaType, Digest: m.Config.Digest, Size: m.Config.Size},
		Document: Blob{MediaType: layer.MediaType, Digest: layer.Digest, Size: layer.Size},
		Title:    layer.Annotations[titleAnnotation],
	}, nil
}

// digestOf returns the "sha256:<hex>" content digest of payload.
func digestOf(payload []byte) string {
	return fmt.Sprintf("sha256:%x", sha256.Sum256(payload))
}
