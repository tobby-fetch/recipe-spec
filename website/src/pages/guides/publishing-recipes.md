---
layout: ../../layouts/Guide.astro
title: Publishing recipes with standard OCI tooling
description: >-
  How to publish a cooked recipe to a cookbook with oras, sign it with
  cosign, and move it across zones — no dedicated tooling required.
---

A cookbook is a plain OCI repository, and a published recipe is an ordinary
OCI artifact ([§11](https://github.com/tobby-fetch/recipe-spec/blob/main/RECIPE-SPEC.md#11-the-cookbook)
of the specification). This guide publishes one with off-the-shelf tools:
[`oras`](https://oras.land) to push, [`cosign`](https://github.com/sigstore/cosign)
to sign, and [`crane`](https://github.com/google/go-containerregistry/tree/main/cmd/crane)
or [`regctl`](https://github.com/regclient/regclient) where they help.
Section references (§) point into
[RECIPE-SPEC.md](https://github.com/tobby-fetch/recipe-spec/blob/main/RECIPE-SPEC.md).

## What you need

- A **cooked** recipe (§8): every ingredient pinned by `digest`, every
  `version` an exact tag. The example below uses
  [`examples/fileset.yaml`](https://github.com/tobby-fetch/recipe-spec/blob/main/examples/fileset.yaml),
  whose metadata is `name: site-config`, `version: 2.3.1`.
- Push access to a registry implementing the OCI Distribution Spec v1.1+.
- `oras` ≥ 1.2, `cosign` ≥ 2.0, and optionally `crane` or `regctl`.

Gate the input first — a cookbook MUST only contain cooked recipes, and
publishing tools MUST reject anything less (§8):

```sh
recipe lint --profile cooked recipe.yaml
```

(`recipe` is this repository's CLI:
`go install github.com/tobby-fetch/recipe-spec/cmd/recipe@latest`.)

## The location is dictated by the document

Recipes are published at `<registry>/<cookbook-path>/<name>:<version>`,
where the repository's **last path segment MUST equal `metadata.name`** and
the **tag MUST equal `metadata.version`** (§11.3). Consumers reject recipes
whose content disagrees with their location, so there is exactly one valid
place for our document in a given cookbook:

```sh
REF=registry.example.com/cookbook/site-config:2.3.1
```

## Push with oras

The published artifact must match the layout of §11.2: artifact type
`application/vnd.tobby.recipe.v1+yaml`, an empty OCI config, and exactly
one layer — the YAML document — titled `recipe.yaml`. One `oras push` does
all of it:

```sh
oras push "$REF" \
  --artifact-type application/vnd.tobby.recipe.v1+yaml \
  recipe.yaml:application/vnd.tobby.recipe.v1+yaml
```

Name the file `recipe.yaml`: oras records the file name in the layer's
`org.opencontainers.image.title` annotation, and §11.2 fixes that title.
Check the result — this is also what consumers verify before trusting an
artifact claiming to be a recipe:

```sh
oras manifest fetch "$REF" | jq '{
  artifactType,
  config: .config.mediaType,
  layers: [.layers[] | {mediaType, title: .annotations["org.opencontainers.image.title"]}]
}'
```

```json
{
  "artifactType": "application/vnd.tobby.recipe.v1+yaml",
  "config": "application/vnd.oci.empty.v1+json",
  "layers": [
    {
      "mediaType": "application/vnd.tobby.recipe.v1+yaml",
      "title": "recipe.yaml"
    }
  ]
}
```

And round-trip the document itself:

```sh
oras pull "$REF" -o /tmp/pulled
diff recipe.yaml /tmp/pulled/recipe.yaml   # no output: bit-identical
```

## Alternative: regctl

`regctl artifact put` produces the same layout:

```sh
regctl artifact put \
  --artifact-type application/vnd.tobby.recipe.v1+yaml \
  -m application/vnd.tobby.recipe.v1+yaml \
  --file-title \
  -f recipe.yaml \
  "$REF"
```

`crane`, by contrast, cannot author this layout: `crane append` builds
container images (image layer media types, no `artifactType`), which
consumers MUST reject as recipes (§11.2). Use crane where it shines —
inspecting and resolving:

```sh
crane manifest "$REF" | jq .artifactType   # "application/vnd.tobby.recipe.v1+yaml"
crane digest "$REF"                        # the manifest digest to sign below
```

## Sign with cosign (key-based)

Recipes are signed with cosign in **key-based** mode; keyless signing is
not assumed, because signatures must verify **fully offline** (§12.1). With
an organization key pair (`cosign generate-key-pair` once, key distributed
out of band by configuration — never inside recipes):

```sh
DIGEST=$(crane digest "$REF")

# Sign by digest, never by tag, and skip the transparency log: air-gapped
# consumers cannot reach Rekor, offline verifiability is the requirement.
cosign sign --key cosign.key --tlog-upload=false \
  "registry.example.com/cookbook/site-config@${DIGEST}"
```

The signature is stored **in the same repository**, under the tag
`sha256-<manifest-digest>.sig` (§12.2) — one more ordinary OCI artifact:

```sh
crane ls registry.example.com/cookbook/site-config
# 2.3.1
# sha256-xxxxxxxx…xxxx.sig
```

Verification needs only the public key, no network services:

```sh
cosign verify --key cosign.pub --insecure-ignore-tlog=true \
  "registry.example.com/cookbook/site-config@${DIGEST}"
```

(`--insecure-ignore-tlog` only disables the Rekor transparency-log check
that this key-based, offline model deliberately does not use; the
signature itself is fully verified against the key.)

Because the signature covers the manifest, it transitively covers the YAML
layer and therefore **every ingredient digest pinned inside it**: one
signature attests the exact bytes of the entire delivery (§12.2).

## Copy across zones, signature included

Transfer tools MUST move signatures along with recipes (§12.2). `cosign
copy` does both in one command:

```sh
cosign copy \
  registry.example.com/cookbook/site-config:2.3.1 \
  registry.zone2.example.com/cookbook/site-config:2.3.1
```

The generic equivalent copies the signature tag explicitly (its name is
derived from the manifest digest):

```sh
SIG_TAG="${DIGEST/:/-}.sig"
oras cp "registry.example.com/cookbook/site-config:2.3.1" \
        "registry.zone2.example.com/cookbook/site-config:2.3.1"
oras cp "registry.example.com/cookbook/site-config:${SIG_TAG}" \
        "registry.zone2.example.com/cookbook/site-config:${SIG_TAG}"
```

> **Tags are immutable.** A cooked recipe's `(name, version)` tag MUST
> NOT be republished with different content — any change, even a single
> digest, requires a new `metadata.version` (§8, §11.3). Configure tag
> immutability on the registry where supported, and let consumers resolve
> tags to digests once and operate on digests thereafter.

## What consumers check

The flip side of this guide, for tool authors (§11–§12.3): reject recipe
artifacts that do not match the §11.2 layout; reject documents whose
`metadata` disagrees with the publication location (the Go SDK's
`ValidatePublishLocation` implements §11.3); verify the signature against
out-of-band trust roots **before** acting on a recipe, and re-verify
before pushing into a destination zone. Verification is on by default and
fails closed.
