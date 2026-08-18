// SPDX-License-Identifier: Apache-2.0
// Copyright © 2026 infraBuilder SASU and contributors

package cookbook_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/tobby-fetch/recipe-spec/cookbook"
	v1alpha1 "github.com/tobby-fetch/recipe-spec/recipe/v1alpha1"
)

// cookedDoc is a minimal valid cooked recipe: one fully pinned ingredient,
// published at "demo:1.0.0".
const cookedDoc = `apiVersion: recipe.tobby.dev/v1alpha1
kind: Recipe
metadata:
  name: demo
  version: 1.0.0
spec:
  ingredients:
    - name: app
      kind: ContainerImage
      ref: docker.io/library/nginx
      version: 1.25.0
      digest: sha256:8acca98ed81b53b482870d6b2081e60d2aa77293895c90c97d2b0e76f469ffb1
`

func TestBuildProducesTheSpecifiedLayout(t *testing.T) {
	art, err := cookbook.Build([]byte(cookedDoc), "demo", "1.0.0")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// The manifest is what §11.2 describes, read back field by field.
	var m struct {
		SchemaVersion int    `json:"schemaVersion"`
		MediaType     string `json:"mediaType"`
		ArtifactType  string `json:"artifactType"`
		Config        struct {
			MediaType string `json:"mediaType"`
			Digest    string `json:"digest"`
			Size      int64  `json:"size"`
		} `json:"config"`
		Layers []struct {
			MediaType   string            `json:"mediaType"`
			Digest      string            `json:"digest"`
			Size        int64             `json:"size"`
			Annotations map[string]string `json:"annotations"`
		} `json:"layers"`
	}
	if err := json.Unmarshal(art.Manifest.Content, &m); err != nil {
		t.Fatalf("manifest is not JSON: %v", err)
	}
	if m.SchemaVersion != 2 {
		t.Errorf("schemaVersion = %d, want 2", m.SchemaVersion)
	}
	if m.MediaType != cookbook.ManifestMediaType {
		t.Errorf("mediaType = %q, want %q", m.MediaType, cookbook.ManifestMediaType)
	}
	if m.ArtifactType != cookbook.ArtifactType {
		t.Errorf("artifactType = %q, want %q", m.ArtifactType, cookbook.ArtifactType)
	}
	if m.Config.MediaType != cookbook.ConfigMediaType {
		t.Errorf("config.mediaType = %q, want %q", m.Config.MediaType, cookbook.ConfigMediaType)
	}
	// The canonical empty config: the two bytes "{}".
	if m.Config.Size != 2 || m.Config.Digest != "sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a" {
		t.Errorf("config descriptor = %d bytes %s, want the canonical empty config", m.Config.Size, m.Config.Digest)
	}
	if len(m.Layers) != 1 {
		t.Fatalf("got %d layers, want exactly one", len(m.Layers))
	}
	if m.Layers[0].MediaType != cookbook.ArtifactType {
		t.Errorf("layer mediaType = %q, want %q", m.Layers[0].MediaType, cookbook.ArtifactType)
	}
	if got, want := m.Layers[0].Size, int64(len(cookedDoc)); got != want {
		t.Errorf("layer size = %d, want %d", got, want)
	}
	if got := m.Layers[0].Annotations["org.opencontainers.image.title"]; got != cookbook.LayerTitle {
		t.Errorf("layer title = %q, want %q", got, cookbook.LayerTitle)
	}
}

func TestBuildReturnsTheBlobsToUpload(t *testing.T) {
	art, err := cookbook.Build([]byte(cookedDoc), "demo", "1.0.0")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if string(art.Document.Content) != cookedDoc {
		t.Error("the document layer is not the document that went in")
	}
	if string(art.Config.Content) != "{}" {
		t.Errorf("config content = %q, want the empty config", art.Config.Content)
	}
	if art.Recipe == nil || art.Recipe.Metadata.Name != "demo" {
		t.Error("Build did not return the parsed recipe")
	}
	// Every blob describes itself consistently: a registry rejects a
	// mismatch between the descriptor and the bytes.
	for _, b := range []struct {
		name string
		blob cookbook.Blob
	}{{"manifest", art.Manifest}, {"config", art.Config}, {"document", art.Document}} {
		if b.blob.Size != int64(len(b.blob.Content)) {
			t.Errorf("%s: size %d does not match %d bytes of content", b.name, b.blob.Size, len(b.blob.Content))
		}
		if !strings.HasPrefix(b.blob.Digest, "sha256:") || len(b.blob.Digest) != 71 {
			t.Errorf("%s: digest %q is not a sha256 digest", b.name, b.blob.Digest)
		}
	}
}

// The manifest digest is the artifact's identity and what a signer signs:
// the same document at the same location must always yield the same one.
func TestBuildIsDeterministic(t *testing.T) {
	first, err := cookbook.Build([]byte(cookedDoc), "demo", "1.0.0")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	second, err := cookbook.Build([]byte(cookedDoc), "demo", "1.0.0")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if first.Manifest.Digest != second.Manifest.Digest {
		t.Errorf("two builds of one document: %s then %s", first.Manifest.Digest, second.Manifest.Digest)
	}
}

// Build's output must round-trip through the consumer-side check: what one
// implementation publishes, another must accept.
func TestBuildOutputPassesVerifyManifest(t *testing.T) {
	art, err := cookbook.Build([]byte(cookedDoc), "demo", "1.0.0")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	layout, err := cookbook.VerifyManifest(art.Manifest.Content)
	if err != nil {
		t.Fatalf("VerifyManifest on our own output: %v", err)
	}
	if layout.Document.Digest != art.Document.Digest {
		t.Errorf("document digest %s read back as %s", art.Document.Digest, layout.Document.Digest)
	}
	if layout.Document.Size != art.Document.Size {
		t.Errorf("document size %d read back as %d", art.Document.Size, layout.Document.Size)
	}
	if layout.Title != cookbook.LayerTitle {
		t.Errorf("title read back as %q, want %q", layout.Title, cookbook.LayerTitle)
	}
}

func TestBuildRefusals(t *testing.T) {
	draft := strings.Replace(cookedDoc,
		"      digest: sha256:8acca98ed81b53b482870d6b2081e60d2aa77293895c90c97d2b0e76f469ffb1\n", "", 1)
	constrained := strings.Replace(cookedDoc, "version: 1.25.0", "version: ^1.25.0", 1)

	for _, tc := range []struct {
		name    string
		doc     string
		segment string
		tag     string
		rule    string
	}{
		{"not a recipe at all", "hello: world\n", "demo", "1.0.0", v1alpha1.RuleAPIVersion},
		{"no digest: still a draft (§8)", draft, "demo", "1.0.0", v1alpha1.RuleCookedDigestMissing},
		{"a constraint, not an exact tag (§8)", constrained, "demo", "1.0.0", v1alpha1.RuleCookedVersionNotExact},
		{"repository disagrees with the name (§11.3)", cookedDoc, "wordpress", "1.0.0", v1alpha1.RulePublishLocation},
		{"tag disagrees with the version (§11.3)", cookedDoc, "demo", "2.0.0", v1alpha1.RulePublishLocation},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := cookbook.Build([]byte(tc.doc), tc.segment, tc.tag)
			if err == nil {
				t.Fatal("published something that must be refused")
			}
			var list v1alpha1.ErrorList
			if !errors.As(err, &list) {
				t.Fatalf("error is not an ErrorList, callers cannot inspect it: %v", err)
			}
			for _, fe := range list {
				if fe.Rule == tc.rule {
					return
				}
			}
			t.Errorf("no %s violation reported; got %v", tc.rule, list)
		})
	}
}

func TestBuildRefusesAnOversizedDocument(t *testing.T) {
	// Valid up to the padding, so the size bound is what refuses it.
	huge := cookedDoc + "# " + strings.Repeat("x", cookbook.MaxDocumentBytes) + "\n"
	_, err := cookbook.Build([]byte(huge), "demo", "1.0.0")
	var le *cookbook.LayoutError
	if !errors.As(err, &le) {
		t.Fatalf("error = %v, want a LayoutError", err)
	}
	if le.Field != "document" {
		t.Errorf("LayoutError.Field = %q, want %q", le.Field, "document")
	}
}

func TestDecideRepublication(t *testing.T) {
	const digest = "sha256:8acca98ed81b53b482870d6b2081e60d2aa77293895c90c97d2b0e76f469ffb1"
	const other = "sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"

	if got := cookbook.DecideRepublication(digest, digest); got != cookbook.RepublicationIdentical {
		t.Errorf("same content: got %v, want identical — republishing must be a no-op", got)
	}
	if got := cookbook.DecideRepublication(other, digest); got != cookbook.RepublicationConflict {
		t.Errorf("different content: got %v, want conflict — §8 forbids re-pointing a tag", got)
	}
}
