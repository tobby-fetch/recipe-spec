# Tobby Recipe Specification

**Version:** `v1alpha1` (draft)
**API group:** `recipe.tobby.dev`
**Status:** Work in progress — breaking changes possible until `v1beta1`
**License:** Apache-2.0

This document specifies the **Recipe** format: a declarative, signable, fully
pinnable description of a set of OCI artifacts that together make up a software
delivery. Recipes are designed to be moved between network zones with different
trust levels — from fully connected zones down to air-gapped zones — by transfer
tools such as [Tobby](https://github.com/tobby-fetch/tobby-fetch), and to be consumed
by any third-party tooling through the schemas published in this repository
([`schemas/`](schemas/)) and its Go SDK ([`recipe/v1alpha1`](recipe/v1alpha1/)
and [`cookbook/`](cookbook/)).

---

## Table of contents

1. [Introduction](#1-introduction)
2. [Notational conventions](#2-notational-conventions)
3. [Terminology](#3-terminology)
4. [Format versioning](#4-format-versioning)
5. [Document structure](#5-document-structure)
6. [The `Recipe` kind](#6-the-recipe-kind)
7. [Ingredient kinds](#7-ingredient-kinds)
8. [Draft and cooked recipes](#8-draft-and-cooked-recipes)
9. [Version constraint syntax](#9-version-constraint-syntax)
10. [The `Retriever` kind](#10-the-retriever-kind)
11. [The cookbook](#11-the-cookbook)
12. [Signing and verification](#12-signing-and-verification)
13. [Registry access and credentials](#13-registry-access-and-credentials)
14. [Security considerations](#14-security-considerations)
15. [Examples](#15-examples)
16. [JSON Schemas](#16-json-schemas)

---

## 1. Introduction

### 1.1 Purpose

Industrial environments (energy, defense, naval, manufacturing…) commonly
segment their networks into zones of decreasing connectivity: a **connected
zone** with controlled internet access, one or more **restricted zones**, and
**air-gapped zones** with no network path at all. Software still has to flow
across these boundaries — container images, Helm charts, AI models, offline
databases, configuration bundles — and it has to flow in a way that is
**explicit, reviewable, verifiable, and reproducible**.

A **Recipe** answers the question *“what exactly is this application made
of?”* in a single YAML document:

- every constituent (**ingredient**) is an OCI artifact, referenced by
  registry, repository, version, and — once the recipe is published —
  content digest;
- recipes are themselves stored and distributed **as OCI artifacts** in an
  OCI repository called a **cookbook**, so the same registries, transport
  tools, and signing machinery apply to recipes and to their contents;
- a published (**cooked**) recipe is fully pinned and immutable: signing the
  recipe transitively attests the exact bytes of every ingredient.

A **Retriever** answers the complementary question *“what does this zone
want?”*: it lists, for one zone, the recipes (and acceptable versions) to be
made available there.

### 1.2 Non-goals

This specification deliberately does **not** cover:

- **Deployment.** A recipe describes *what to transfer*, not *how to install
  or run it*. Deployment (e.g. GitOps reconciliation of the Helm charts a
  recipe carries) is downstream tooling’s concern.
- **Acquisition from non-OCI sources.** Every ingredient reference is an OCI
  reference. Content that originates elsewhere (plain HTTP downloads, git
  repositories, package mirrors) MUST be packaged as an OCI artifact — for
  arbitrary files, see the [`FileSet`](#74-fileset) kind — before a recipe can
  reference it.
- **Secrets.** Recipes never contain credentials, tokens, or key material.
  See [§13](#13-registry-access-and-credentials).
- **Vulnerability and admission policy.** Whether a CVE blocks a transfer,
  which registries are allow-listed, and similar policy decisions belong to
  the consuming tool’s configuration, not to the format.
- **Garbage collection / purge semantics.** The format enables external
  mark-and-sweep (a zone’s desired state is fully enumerable from its
  retriever and the referenced recipes), but does not specify a purge
  process.
- **Registry authentication protocols.** Recipes reference registries by
  hostname only; how a consumer authenticates is out of band ([§13](#13-registry-access-and-credentials)).

## 2. Notational conventions

The key words **MUST**, **MUST NOT**, **REQUIRED**, **SHALL**, **SHALL NOT**,
**SHOULD**, **SHOULD NOT**, **RECOMMENDED**, **NOT RECOMMENDED**, **MAY**, and
**OPTIONAL** in this document are to be interpreted as described in
[RFC 2119](https://www.rfc-editor.org/rfc/rfc2119) and
[RFC 8174](https://www.rfc-editor.org/rfc/rfc8174) when, and only when, they
appear in all capitals, as shown here.

“Implementations” designates any software that produces, validates,
transfers, or consumes documents conforming to this specification.

## 3. Terminology

| Term | Definition |
| --- | --- |
| **Recipe** | A YAML document of kind `Recipe` describing a named, versioned set of ingredients. |
| **Ingredient** | One OCI artifact referenced by a recipe: a container image, Helm chart, generic OCI artifact, or file set. |
| **Cookbook** | An OCI repository namespace in which recipes are published as OCI artifacts. The cookbook of a zone is the authoritative catalog of software approved for that zone. |
| **Retriever** | A YAML document of kind `Retriever` listing the recipes (and version constraints) desired for one zone. |
| **Draft recipe** | A recipe under construction: digests optional, version constraints allowed. |
| **Cooked recipe** | A recipe published to a cookbook: fully pinned by digest, exact versions only, immutable. |
| **Connected zone** | A network zone with (controlled) access to upstream registries. |
| **Restricted zone** | A zone reachable only through controlled, one-way promotion from a more connected zone. |
| **Air-gapped zone** | A zone with no network path; content arrives on removable media. |
| **Production registry** | The registry, in the connected zone, that hosts the qualified cookbook fed by the organization’s qualification pipeline. |
| **Zone registry** | The registry inside a restricted or air-gapped zone that receives promoted content. |

## 4. Format versioning

### 4.1 API group and version

Every document declares its schema through the `apiVersion` field, following
the Kubernetes convention `<group>/<version>`:

```yaml
apiVersion: recipe.tobby.dev/v1alpha1
```

The planned lifecycle is `v1alpha1` → `v1beta1` → `v1`. The in-document
`apiVersion` is the **single authoritative** indicator of schema version.

### 4.2 Compatibility policy

| Stage | Guarantees |
| --- | --- |
| `v1alpha1` | Experimental. Fields MAY be renamed, retyped, or removed between alpha revisions. Not for long-lived archival. |
| `v1beta1` | Fields MAY be deprecated but MUST NOT be removed or change meaning within beta. Documents valid in an earlier beta revision remain valid in later ones. |
| `v1` | Stable. Changes are strictly additive (new OPTIONAL fields). Any breaking change would require a new API version, which is **not planned**: format stability is a design goal. |

Implementations:

- MUST reject a document whose `apiVersion` they do not support;
- MUST reject unknown fields (strict validation — see [§4.3](#43-strict-validation));
- SHOULD, once `v1` exists, read all prior supported versions and write the
  newest one (one-way conversion on re-publication).

### 4.3 Strict validation

Recipes are security-relevant inputs: a silently ignored misspelled field
(e.g. `digset:` instead of `digest:`) could weaken pinning without any error.
Validators MUST therefore reject documents containing properties not defined
by this specification. The published JSON Schemas enforce this with
`additionalProperties: false`. Extension data belongs in
`metadata.annotations`, which is an open string map by design.

### 4.4 Media type versioning

Recipes published as OCI artifacts use the artifact type
`application/vnd.tobby.recipe.v1+yaml` ([§11.2](#112-artifact-layout)). The
`v1` in the media type versions the *artifact envelope* (a single YAML
document as sole layer); the schema of the document itself is versioned by
`apiVersion`. The media type is expected to remain stable across
`v1alpha1`/`v1beta1`/`v1`.

## 5. Document structure

- Documents are [YAML 1.2](https://yaml.org/spec/1.2.2/) and MUST also be
  representable as JSON (no YAML-specific types: no anchors resolving to
  cycles, no custom tags, no non-string mapping keys).
- Encoding MUST be UTF-8.
- A file SHOULD contain a single document. Tools MAY accept multi-document
  streams (`---` separators) and MUST then treat each document independently.
- A document MUST NOT exceed **4 MiB** (4 194 304 bytes) — the size bound of
  the published artifact layer ([§11.2](#112-artifact-layout)). Parsers MAY
  refuse larger input before decoding it: a recipe is a small YAML file, and
  an oversized document is hostile or wrong, not big.
- When published to a cookbook, an OCI artifact contains **exactly one**
  `Recipe` document ([§11.2](#112-artifact-layout)).

Every document has four top-level fields, in the Kubernetes style:

```yaml
apiVersion: recipe.tobby.dev/v1alpha1   # REQUIRED — schema version (§4)
kind: Recipe                            # REQUIRED — "Recipe" or "Retriever"
metadata: {}                            # REQUIRED — identity and metadata
spec: {}                                # REQUIRED — kind-specific content
```

## 6. The `Recipe` kind

### 6.1 `metadata`

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `name` | string | yes | Recipe name. MUST be a valid OCI repository path segment: lowercase alphanumerics with `.`, `_`, `-` separators (`^[a-z0-9]+((\.|_|__|-+)[a-z0-9]+)*$`), at most 63 characters. Becomes the last path segment of the cookbook repository ([§11.3](#113-naming-and-tags)). |
| `version` | string | yes | Version of the **described application** (not of any single ingredient). MUST be valid [Semantic Versioning 2.0.0](https://semver.org) **without build metadata** — `+` is not a legal OCI tag character ([§11.3](#113-naming-and-tags)). Becomes the OCI tag on publication, and MUST therefore also be a valid OCI tag: at most **128 characters** ([§11.3](#113-naming-and-tags)). |
| `description` | string | no | Human-readable, one-paragraph description. At most 2048 characters. |
| `labels` | map[string]string | no | Identifying key/value pairs intended for selection and filtering. Keys and values follow Kubernetes label syntax (values ≤ 63 characters). |
| `annotations` | map[string]string | no | Non-identifying metadata (provenance URLs, timestamps, tooling data). Keys SHOULD be namespaced with a DNS prefix (e.g. `recipe.tobby.dev/…`). Values are at most 4096 characters. |

Annotation keys under the `recipe.tobby.dev/` prefix are reserved for this
specification and its reference tooling. Keys defined by this revision:

| Annotation | Meaning |
| --- | --- |
| `recipe.tobby.dev/cooked-at` | RFC 3339 timestamp at which the recipe was cooked. |
| `recipe.tobby.dev/upstream-digest.<ingredient-name>` | Digest of the upstream artifact when the pinned artifact was rewritten by tooling (e.g. Helm chart dependency vendoring, [§7.2](#72-helmchart)). |

### 6.2 `spec.ingredients`

`spec.ingredients` is a non-empty array. Each entry describes one OCI
artifact. Common fields for all ingredient kinds:

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `name` | string | yes | Ingredient name, unique within the recipe (DNS label: `^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`, ≤ 63 chars). Used in logs, status reports, and reserved annotation keys. |
| `kind` | string | yes | One of `ContainerImage`, `HelmChart`, `OCIArtifact`, `FileSet` ([§7](#7-ingredient-kinds)). |
| `ref` | string | yes | Source OCI reference **without tag or digest**: `<registry-host>[:<port>]/<repository-path>`. See rules below. |
| `version` | string | yes | Exact tag, or version constraint ([§9](#9-version-constraint-syntax)). Cooked recipes MUST use an exact tag ([§8](#8-draft-and-cooked-recipes)). |
| `digest` | string | no (draft) / **yes (cooked)** | Content digest of the artifact’s top-level manifest (or index), e.g. `sha256:…`. Registered algorithms: `sha256`, `sha512`. |
| `platforms` | []string | no | `ContainerImage` and `FileSet` only. Platforms required at destination, as `os/arch[/variant]` (e.g. `linux/amd64`). See per-kind semantics. |

Rules for `ref`:

- `ref` MUST be fully qualified: it MUST start with a registry hostname
  (a dotted name, `localhost`, or a name with an explicit port). There is
  **no default registry**: `library/alpine` is invalid, use
  `docker.io/library/alpine`.
- `ref` MUST NOT contain a tag (`:v1`), a digest (`@sha256:…`), a URL scheme
  (`https://`), userinfo (`user@`), query strings, or path traversal.
  Version and digest have dedicated fields so that they can be validated,
  resolved, and rewritten independently.
- The hostname is the key used for out-of-band credential lookup
  ([§13](#13-registry-access-and-credentials)).

Rules for `digest`:

- The digest pins the **top-level** manifest fetched at `ref:version` — for a
  multi-platform image this is the image index, not a per-platform manifest.
- Consumers MUST verify that fetched content matches the recorded digest and
  MUST abort the operation for that ingredient on mismatch.

### 6.3 Minimal example

```yaml
apiVersion: recipe.tobby.dev/v1alpha1
kind: Recipe
metadata:
  name: hello
  version: 1.0.0
spec:
  ingredients:
    - name: hello
      kind: ContainerImage
      ref: docker.io/library/hello-world
      version: linux                      # exact tag (draft: no digest yet)
      platforms: ["linux/amd64"]
```

## 7. Ingredient kinds

Fields marked *kind-specific* below are only valid for that kind; validators
MUST reject them elsewhere (enforced by the JSON Schemas).

### 7.1 `ContainerImage`

A runnable container image, possibly multi-platform.

- `ref:version` MAY resolve to a single image manifest or to an OCI image
  index / Docker manifest list.
- For multi-platform images, `digest` MUST be the digest of the **index**.
- `platforms` (OPTIONAL) lists the platforms that MUST be present and usable
  at the destination. Implementations MAY omit unlisted platforms from
  transfer (sparse index) **provided** the original index — and therefore the
  pinned digest — is preserved at the destination; if the destination
  registry rejects indexes referencing absent manifests, implementations
  MUST transfer all platforms instead. If `platforms` is absent, all
  platforms MUST be transferred.
- Transfers MUST be bit-exact: layers, config, and manifests are copied
  unmodified, so digests remain valid across zones and any upstream image
  signatures remain verifiable.

```yaml
- name: mariadb
  kind: ContainerImage
  ref: docker.io/bitnami/mariadb
  version: 11.4.7-debian-12-r0
  digest: sha256:30442ceb6e26d1d27216a8d75820636c3cbed010482ea2a81f82d097872ee627
  platforms: ["linux/amd64", "linux/arm64"]
```

### 7.2 `HelmChart`

A Helm chart stored as an OCI artifact (Helm ≥ 3.8 native format,
`application/vnd.cncf.helm.chart.content.v1.tar+gzip` layer). Charts hosted
in legacy `index.yaml` HTTP(S) repositories are out of scope for the format:
they MUST be pushed to an OCI registry before being referenced (tooling MAY
automate that conversion upstream of recipe authoring).

Kind-specific field:

| Field | Type | Default | Description |
| --- | --- | --- | --- |
| `vendorDependencies` | boolean | `false` | Whether the chart is made self-contained at cook time. |

Semantics:

- `vendorDependencies: false` (default, RECOMMENDED): the chart archive is
  transferred bit-for-bit; `digest` is the upstream chart digest; upstream
  signatures and provenance remain verifiable. Chart dependencies that are
  not already embedded in the archive are **not** fetched; if needed at the
  destination, they MUST be listed as separate `HelmChart` ingredients.
- `vendorDependencies: true`: at cook time, the dependencies declared in the
  chart’s `Chart.yaml`/`Chart.lock` are resolved (recursively) and embedded
  in the archive’s `charts/` directory, producing a **new** artifact.
  Consequences implementations and authors MUST be aware of:
  - the recorded `digest` is the digest of the **vendored** artifact as
    republished by the cooking process — not the upstream digest;
  - upstream chart signatures and provenance no longer apply to the vendored
    artifact;
  - the cooking process SHOULD record the upstream digest in the recipe
    annotation `recipe.tobby.dev/upstream-digest.<ingredient-name>` for
    traceability.
- A chart never implies its container images. Images referenced by chart
  values MUST be listed explicitly as `ContainerImage` ingredients: transfers
  stay fully explicit and reviewable, with no templating or values evaluation
  required of implementations.

```yaml
- name: wordpress-chart
  kind: HelmChart
  ref: registry-1.docker.io/bitnamicharts/wordpress
  version: 24.2.8
  digest: sha256:fbe62c6b0a4b37e55b5231269c4a0aab9f30bbf69fe454c148988d2b654f46d3
  vendorDependencies: false
```

### 7.3 `OCIArtifact`

Any other OCI artifact: AI/ML models and weights (e.g. pushed with ORAS or
model registries), vulnerability database bundles, SBOM collections, WASM
modules, policy bundles, signed metadata… If it lives in an OCI registry and
is not a runnable image, a chart, or a file set, it is an `OCIArtifact`.

Kind-specific field:

| Field | Type | Default | Description |
| --- | --- | --- | --- |
| `artifactType` | string | — | Expected artifact type (media type syntax) of the fetched manifest, e.g. `application/vnd.example.mlmodel.v1+gguf`. |

Semantics:

- If `artifactType` is set, implementations MUST verify that the fetched
  manifest’s `artifactType` (or, for compatibility with older producers, its
  config `mediaType`) equals the declared value, and MUST fail the ingredient
  otherwise. This guards against tag-reuse mistakes and repository confusion.
- If `artifactType` is unset, any artifact type is accepted.
- Transfers MUST be bit-exact (as for `ContainerImage`).

```yaml
- name: sentiment-model
  kind: OCIArtifact
  ref: registry.example.com/models/sentiment-analyzer
  version: 1.4.2
  digest: sha256:fb9ff3ee975e25f95b21f6e9605f32c9658ede17089f61000dd540ece55e8761
  artifactType: application/vnd.example.mlmodel.v1+gguf
```

### 7.4 `FileSet`

Arbitrary files packaged **as an OCI image**: configuration bundles, offline
documentation, scripts, certificates, datasets. Packaging files as a standard
image (filesystem layers) rather than a bespoke artifact type gives two
consumption modes for free:

1. **Mountable** — the image can be mounted read-only in Kubernetes via
   [image volumes](https://kubernetes.io/docs/concepts/storage/volumes/#image)
   (`volumes[].image`), referenced by the pinned digest, with no extraction
   step and no drift.
2. **Extractable** — any consumer can materialize the files on disk using the
   extraction semantics below.

Kind-specific field:

| Field | Type | Default | Description |
| --- | --- | --- | --- |
| `extract` | object | — | Extraction hints. `extract.paths` is a non-empty list of slash-separated glob patterns (`*`, `**`), relative to the image root, selecting what to materialize. Absent `extract`, or absent `paths`, means the whole root filesystem. |

`extract` is a **hint for consumers**; its presence or absence does not change
what is transferred (the image is always copied bit-exact and in full).

**Extraction semantics.** An implementation that extracts a `FileSet` MUST:

1. select the platform manifest if the reference resolves to an index
   (`platforms` applies as for `ContainerImage`; platform-independent
   content SHOULD be published as a single-manifest image);
2. apply the layers **in manifest order**, honoring OCI/overlayfs whiteout
   entries (`.wh.` prefixed files and `.wh..wh..opq` opaque markers), to
   obtain the merged root filesystem;
3. materialize the paths selected by `extract.paths` (or everything) under
   the destination directory, preserving file modes and symlinks where the
   platform permits;
4. enforce the safety rules of [§14.5](#145-fileset-extraction-safety)
   (path traversal, link escape, resource limits).

```yaml
- name: site-config
  kind: FileSet
  ref: registry.example.com/filesets/site-config
  version: 2.3.1
  digest: sha256:4c679da467e9bfa631ae93135c215bdb46205a90364acf4e5510ba39f4fa0965
  extract:
    paths:
      - "etc/**"
      - "opt/scripts/**"
```

## 8. Draft and cooked recipes

A recipe exists in exactly one of two states. The state is not a field: it is
determined by where the document lives and what it contains.

| | **Draft** | **Cooked** |
| --- | --- | --- |
| Where | Authoring workspace, git, review systems | Published in a cookbook ([§11](#11-the-cookbook)) |
| `ingredient.version` | Exact tag **or** constraint ([§9](#9-version-constraint-syntax)) | Exact tag only |
| `ingredient.digest` | OPTIONAL | **REQUIRED** on every ingredient |
| Mutability | Freely editable | Immutable |
| Signature | Not required | REQUIRED ([§12](#12-signing-and-verification)) |

Normative rules:

- **Cooking** is the act of resolving every version constraint to an exact
  tag, recording the digest of every ingredient, and publishing the result to
  a cookbook. A qualification pipeline typically performs cooking after its
  checks (scans, tests, reviews) pass.
- A cookbook MUST only contain cooked recipes: publishing a recipe with any
  missing `digest`, or any `version` that is a constraint rather than an
  exact tag, is invalid and MUST be rejected by publishing tools.
- A cooked recipe is **totally pinned**: fetching its ingredients at their
  recorded digests yields bit-identical content anywhere, forever (or fails
  verifiably). Two zones holding the same cooked recipe hold, by
  construction, descriptions of identical bytes.
- A cooked recipe is **immutable**: the `(name, version)` tag MUST NOT be
  republished with different content. Any change — even one digest — REQUIRES
  a new `metadata.version`. Consumers SHOULD resolve tags to digests once and
  operate on digests thereafter.
- Verification duties of consumers are listed in [§12.3](#123-verification).

## 9. Version constraint syntax

The `version` field of an ingredient (and of a retriever’s recipe entry)
accepts either an **exact tag** or a **constraint expression**.

### 9.1 Exact tags

Any valid OCI tag (`^[a-zA-Z0-9_][a-zA-Z0-9._-]{0,127}$`) that is not parsed
as a constraint (see [§9.3](#93-disambiguation)) is an exact tag, matched
literally: `24.2.8`, `6.8.2-debian-12-r3`, `v1.30.1`, `latest`.
Mutable tags such as `latest` are NOT RECOMMENDED even in drafts, and are
meaningless once cooked (the digest governs).

### 9.2 Constraint expressions

Constraints operate on tags that parse as Semantic Versioning 2.0.0 versions.
A leading `v` on a tag is ignored for matching (`v1.2.3` ≡ `1.2.3`).

| Form | Example | Matches |
| --- | --- | --- |
| Wildcard | `12.x`, `12.4.x`, `*` | Any version with the given fixed components (`x`, `X`, and `*` are equivalent placeholders). |
| Caret | `^1.4.0` | `>=1.4.0 <2.0.0` — compatible within the leftmost non-zero component (semver caret rule: `^0.3.1` means `>=0.3.1 <0.4.0`). |
| Tilde | `~1.4.2` | `>=1.4.2 <1.5.0` — patch-level changes only. |
| Comparison | `>=2.0.0`, `>1.2`, `<=3`, `<4.0.0`, `=1.2.3`, `!=1.5.0` | Standard semver comparison. Partial versions designate a **series** (see below). |
| Conjunction | `>=2.0.0 <3.0.0` or `>=2.0.0, <3.0.0` | All constraints must hold (whitespace and/or commas separate terms; AND semantics). |

**Version literals in operators.** The version following a comparison,
caret, or tilde operator has 1 to 3 numeric components, optionally preceded
by a `v` (ignored, as on tags). Components MUST be numeric with no leading
zero. A pre-release suffix (`-rc.1`) is only permitted when all three
components are given (`>=1.2-rc.1` is invalid). Build metadata (`+…`),
wildcard placeholders combined with an operator (`>1.x`), more than three
components, and `==` (write `=`) MUST be rejected.

**Partial versions designate series.** When a comparison operator is given
fewer than three components, the missing components do NOT default to zero
on both sides: the partial version designates the whole series it names
(`1.2` designates `1.2.x`; `3` designates `3.x.y`), and the operator
applies to that series. Normatively, with `X.Y` standing for a two-component
literal (one-component literals `X` behave identically with the series
`X.*.*` and the series end `(X+1).0.0`):

| Term | Meaning | Example |
| --- | --- | --- |
| `=X.Y` | Within the series: `>=X.Y.0 <X.(Y+1).0` | `=1.2` matches `1.2.9`, not `1.3.0` |
| `!=X.Y` | Outside the series | `!=1.2` matches `1.3.0`, not `1.2.9` |
| `>X.Y` | Above the series: `>=X.(Y+1).0` | `>1.2` matches `1.3.0`, **not** `1.2.5` |
| `>=X.Y` | `>=X.Y.0` | `>=1.2` matches `1.2.0` |
| `<X.Y` | `<X.Y.0` | `<1.2` rejects `1.2.0` |
| `<=X.Y` | Within or below the series: `<X.(Y+1).0` | `<=3` matches `3.999.0`, not `4.0.0` |

Implementations MUST apply these series semantics. Note that several
common constraint libraries read `>1.2` as `>1.2.0` (matching `1.2.5`);
that reading does not conform to this specification.

**Caret and tilde bounds.** The lower bound is always the literal with
missing components filled with zeros, inclusive. The exclusive upper bound
is:

- `^`: the leftmost non-zero component is bumped — `^1.4.0` allows
  `<2.0.0`, `^0.3.1` allows `<0.4.0`, `^0.0.3` allows `<0.0.4`, `^1.2`
  allows `<2.0.0`. When every given component is zero, the **last given**
  component is bumped: `^0` allows `<1.0.0`, `^0.0` allows `<0.1.0`,
  `^0.0.0` allows `<0.0.1`.
- `~`: patch-level changes when a minor component is given — `~1.4.2`
  allows `<1.5.0`, `~1.2` allows `<1.3.0`; minor-level changes otherwise —
  `~1` allows `<2.0.0`.

Resolution rules:

1. Candidates are the tags of `ref` that parse as semver (after `v`
   stripping); non-semver tags are ignored by constraint matching.
2. Pre-release versions (`1.5.0-rc.1`) are excluded unless the constraint
   expression itself mentions a pre-release on the same
   `major.minor.patch` triple (semver range convention).
3. Build metadata (`+…`) is ignored for ordering, per semver.
4. The **highest** matching version is selected.
5. If no candidate matches, resolution MUST fail (it MUST NOT silently fall
   back to another tag).

Disjunction (`||`) is intentionally not supported in `v1alpha1`.

### 9.3 Disambiguation

A `version` value is a **constraint expression** if and only if:

- it starts with one of `^ ~ > < = !`, or
- it contains whitespace or a comma, or
- it consists solely of dotted numeric components where at least one
  component is `x`, `X`, or `*` (e.g. `12.x`, `1.2.*`), or it is exactly
  `*`, `x`, or `X`.

Anything else is an exact tag. Note that a plain `1.2.3` is an exact **tag**
(matched literally), not the constraint `=1.2.3`; the two differ only when a
registry carries equivalent tags like `v1.2.3`.

## 10. The `Retriever` kind

A `Retriever` declares the desired recipe set for **one zone**. It is the
input a transfer tool consumes: in a connected/restricted zone (passthrough
mode) the tool periodically re-resolves the retriever against the cookbook
and pushes what is missing; toward an air-gapped zone (mirror mode) the tool
resolves it once per synchronization onto removable media.

### 10.1 Structure

```yaml
apiVersion: recipe.tobby.dev/v1alpha1
kind: Retriever
metadata:
  name: restricted-zone            # conventionally, the zone name
  description: Desired recipe set for the restricted zone.
spec:
  cookbook: registry.example.com/cookbook    # default source cookbook
  recipes:
    - name: wordpress
      version: "6.8.2"                       # exact recipe version…
    - name: postgresql
      version: "16.x"                        # …or a constraint (§9)
```

### 10.2 Fields

`metadata`: `name` (REQUIRED, same syntax as recipe names), plus OPTIONAL
`description`, `labels`, `annotations` (as in [§6.1](#61-metadata); there is
no `metadata.version` — a retriever is living configuration, tracked in
version control, not a published versioned artifact).

`spec`:

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `cookbook` | string | yes | Default cookbook to resolve recipes from: `<registry-host>[:<port>]/<repository-path>` (same syntax rules as ingredient `ref`). |
| `recipes` | array | yes (non-empty) | Desired recipes. |
| `recipes[].name` | string | yes | Recipe name, as published ([§11.3](#113-naming-and-tags)). |
| `recipes[].version` | string | yes | Exact recipe version or constraint ([§9](#9-version-constraint-syntax)), resolved against the cookbook’s tags for that recipe. |
| `recipes[].cookbook` | string | no | Per-entry override of `spec.cookbook`. |

### 10.3 Semantics

- Each entry resolves independently: list the recipe’s tags in its cookbook,
  apply [§9](#9-version-constraint-syntax), select the highest match, resolve
  the tag to a digest, verify the recipe signature
  ([§12](#12-signing-and-verification)), then transfer the recipe artifact
  **and** all its ingredients.
- Recipes are transferred **with** their ingredients: the destination zone’s
  cookbook receives the cooked recipe artifact itself, so the zone remains
  self-describing and can be audited, re-verified, or garbage-collected
  offline.
- A retriever is the complete desired state of its zone. Content in the zone
  not reachable from any retriever entry is, by definition, eligible for
  external mark-and-sweep cleanup (out of scope, [§1.2](#12-non-goals)).
- Duplicate `name` entries are permitted only with disjoint resolved
  versions (e.g. keeping `6.8.2` while introducing `7.x`); tools SHOULD warn
  when two entries resolve to the same version.

## 11. The cookbook

### 11.1 Definition

A **cookbook** is an OCI repository namespace holding cooked recipes as OCI
artifacts. It requires no dedicated server software: any registry conforming
to the [OCI Distribution Specification](https://github.com/opencontainers/distribution-spec)
v1.1+ (artifact support) can host a cookbook. Because recipes are ordinary
OCI artifacts, a cookbook — or any subset of it — can itself be mirrored
across zones with the same tools, signatures included.

### 11.2 Artifact layout

A published recipe is an OCI image manifest with:

- `artifactType`: `application/vnd.tobby.recipe.v1+yaml`
- an empty config (`application/vnd.oci.empty.v1+json`)
- exactly **one** layer: the recipe YAML document, with media type
  `application/vnd.tobby.recipe.v1+yaml`

```json
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
  "artifactType": "application/vnd.tobby.recipe.v1+yaml",
  "config": {
    "mediaType": "application/vnd.oci.empty.v1+json",
    "digest": "sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a",
    "size": 2
  },
  "layers": [
    {
      "mediaType": "application/vnd.tobby.recipe.v1+yaml",
      "digest": "sha256:<digest-of-the-yaml-document>",
      "size": 1742,
      "annotations": { "org.opencontainers.image.title": "recipe.yaml" }
    }
  ],
  "annotations": {
    "org.opencontainers.image.created": "2026-07-09T14:32:00Z"
  }
}
```

Publishers MUST NOT attach additional layers; consumers MUST reject recipe
artifacts that do not match this layout. The layout is deliberately
compatible with generic OCI tooling (`oras push`, `oras pull`, `crane`,
`skopeo`).

The recipe document layer MUST NOT exceed **4 MiB** (4 194 304 bytes). A
recipe is a small YAML file; consumers MUST reject artifacts whose layer
size exceeds this bound (and SHOULD do so before fetching the layer), and
parsers MAY refuse larger documents outright ([§5](#5-document-structure)).

Two conforming publishers given the same recipe document do **not**
necessarily produce byte-identical manifests: the compatibility promised
above means generic tooling participates, and generic tooling is free to
add manifest annotations (`oras push` records
`org.opencontainers.image.created`, as shown in the example) or serialize
fields differently. The **manifest digest** therefore identifies one
published artifact — it is what gets signed ([§12](#12-signing-and-verification)) —
but it is NOT a stable identity for the recipe across tools. The recipe's
semantic identity is the digest of its single document layer
(`layers[0].digest`): two recipe artifacts carrying the same layer digest
carry the same recipe.

The `org.opencontainers.image.title` annotation shown above is
illustrative, not normative. Publishers SHOULD set it to `recipe.yaml`, so
that a generic `oras pull` writes a sensibly named file; consumers MUST
NOT depend on it, and MUST NOT reject an artifact over its value or its
absence. Requiring it would contradict the compatibility promised in the
paragraph above: generic tooling writes whatever file name it was handed.

### 11.3 Naming and tags

```
<registry>/<cookbook-path>/<name>:<version>
└───┬────┘ └─────┬───────┘ └──┬─┘ └───┬───┘
 registry   cookbook (one    metadata  metadata
 hostname   or more path     .name     .version
 [:port]    segments)
```

Example: `registry.example.com/cookbook/wordpress:6.8.2`.

- The repository’s last path segment MUST equal the recipe’s
  `metadata.name`, and the tag MUST equal its `metadata.version`. A recipe
  artifact whose content disagrees with its location MUST be rejected.
  (This is why `metadata.version` excludes semver build metadata and is
  capped at 128 characters, [§6.1](#61-metadata): a version containing `+`,
  or longer than an OCI tag may be, would have no publishable tag.)
- Tags are immutable ([§8](#8-draft-and-cooked-recipes)): re-pointing an
  existing `(name, version)` tag is a policy violation; registries SHOULD be
  configured to enforce tag immutability where supported. Whether a
  republication carries "the same content" is decided on the **document
  layer digest** ([§11.2](#112-artifact-layout)), never on the manifest
  digest: the same document republished through a different conforming tool
  may produce a different manifest. When the layer digests match, the
  publisher MUST treat the publication as already done and MUST NOT
  re-point the tag (the existing manifest, and any signature over its
  digest, stays untouched); when they differ, it MUST refuse.
- Publishers MAY additionally maintain a floating `latest` tag for human
  convenience; automated consumers MUST NOT depend on it.

### 11.4 Discovery and listing

- **Versions of a known recipe**: `GET /v2/<cookbook-path>/<name>/tags/list`
  (standard Distribution API). This is the basis for constraint resolution
  ([§9](#9-version-constraint-syntax)).
- **Recipes in a cookbook**: registries expose repository listing with
  varying fidelity (`GET /v2/_catalog` where enabled, or vendor APIs).
  Implementations SHOULD use `_catalog` filtered by the cookbook path prefix
  when available.
- Recipe artifacts are distinguishable from other artifacts in shared
  registries by their `artifactType` ([§11.2](#112-artifact-layout)).
- A signed, self-contained **cookbook index** artifact (single document
  enumerating the cookbook, for registries without listing and for offline
  audit) is planned for a future revision of this specification, and is
  intentionally not specified in `v1alpha1`.

### 11.5 Ingredient relocation convention

When a transfer tool copies an ingredient into a store or a zone registry, it
SHOULD publish it under the repository path formed by prefixing the
ingredient's source repository path with its **nominal** source registry host —
the host written in the recipe's `ref`, regardless of the endpoint actually
contacted. Endpoint substitution (mirrors, cascaded zone registries) never
changes the relocated path, which is therefore invariant across any number of
hops:

```
<destination>[/<base>]/<canonical-source-host>[_<port>]/<repository-path>
```

Host canonicalization: hosts are lowercased; the Docker Hub aliases
`index.docker.io` and `registry-1.docker.io` canonicalize to `docker.io`; no
other implicit normalization is permitted. `:<port>` is rewritten `_<port>` —
unambiguous, as a valid hostname cannot contain `_`. IPv6 literal hosts are not
relocatable under this convention and MUST be rejected explicitly. If a
relocated name exceeds a destination's limits, the transfer MUST fail
explicitly; names are never truncated.

Content is copied bit-exact (§7.1): the recorded `digest` remains the
verification key regardless of location, and attached signature artifacts
(§12.2) are copied into the relocated repository. Tools following this
convention are mutually predictable: given a recipe and a destination, the
expected location of every ingredient is computable with no additional
metadata — which is what external cleanup tooling (§1.2, mark-and-sweep)
needs. Recipe artifacts themselves follow the cookbook convention (§11.3), not
this section.

## 12. Signing and verification

### 12.1 Model

Recipes are signed with [Sigstore cosign](https://github.com/sigstore/cosign)
in **key-based** mode. Keyless signing (Fulcio certificates, Rekor
transparency log) is NOT assumed and NOT required: it depends on online
services that are unreachable from restricted and air-gapped zones. The
required properties are:

- signatures MUST be verifiable **fully offline**;
- trust roots (public keys) are distributed **out of band by configuration**
  of the consuming tools — never inside recipes, and never fetched from the
  registry being verified;
- signatures MUST travel with the content across zones.

### 12.2 Signature transport

Signatures are stored **in the same repository** as the recipe, in either
of the two layouts cosign publishes. Producers pick one; consumers MUST
accept both, because the choice belongs to whoever signs:

- **attached signature** (the classic layout): the signature artifact is
  tagged `sha256-<manifest-digest>.sig`, and its layer is a SimpleSigning
  payload pinning the subject digest;
- **Sigstore bundle** (the default of cosign 3.x): the signature artifact
  *refers* to the subject — `artifactType`
  `application/vnd.dev.sigstore.bundle.v0.x+json`, a `subject` descriptor,
  and one bundle layer whose DSSE envelope carries an in-toto statement
  naming the subject digest. It is discovered through the OCI 1.1
  Referrers API, or through the fallback tag `sha256-<manifest-digest>`
  that clients create when the registry has no such API.

A consumer that accepted only one layout would reject perfectly valid
recipes for a reason no signer can guess, so accepting both is normative,
not a convenience. Consequently:

- transfer tools MUST copy signature artifacts alongside every recipe (and
  alongside ingredients that carry signatures) so that verification remains
  possible in the destination zone — for the bundle layout this includes
  the referring artifact and, where it exists, the fallback tag;
- signing a cooked recipe’s manifest transitively covers the YAML document
  (via the layer digest) and therefore every ingredient’s pinned digest: one
  recipe signature attests the **exact bytes** of the entire delivery.

Ingredients MAY additionally carry their own cosign signatures (from
upstream publishers or from the organization’s qualification pipeline);
verifying them is a consumer policy decision.

### 12.3 Verification

Consumers (transfer tools, zone-side tooling) MUST:

1. verify the recipe signature against the configured trust roots **before**
   acting on a recipe pulled from any cookbook;
2. verify each fetched ingredient’s content against its recorded `digest`;
3. re-verify recipe signatures **before pushing** to a destination registry
   (mirror/removable-media flows: media contents are untrusted until
   verified on the destination side);
4. fail closed: on any signature or digest mismatch, the affected item MUST
   NOT be pushed or exposed to the destination zone, and the failure MUST be
   reported.

Verification is on by default. Enforcement MAY be relaxed only for explicitly
declared trust-root scopes (e.g. a named low-assurance source restricted to
specific repositories); implementations MUST NOT offer a global, undeclared
bypass of signature verification, and any relaxed scope MUST be visible in the
consumer's reported configuration.

Key rotation is handled by configuration: consumers accept a **set** of
trusted keys, enabling overlap periods where recipes signed by either the
outgoing or the incoming key verify successfully.

Consumers SHOULD accept trust roots as inline key material, local files, or
HTTPS URLs fetched and cached **at configuration time** — never at verification
time, never from the registry being verified, and never from a transport
medium (§14.4). Air-gapped consumers MUST use the inline or file forms.

## 13. Registry access and credentials

### 13.1 Hostname-only references

Recipes and retrievers reference registries **by hostname (and optional
port) only** — in ingredient `ref` and in `cookbook` fields. They MUST NOT
contain credentials, tokens, URL userinfo, or scheme selection. A recipe is
a shareable, publishable document; nothing in it is secret.

### 13.2 Out-of-band credentials: `kubernetes.io/dockerconfigjson`

Consuming tools obtain registry credentials out of band, in the standard
Kubernetes Secret format
[`kubernetes.io/dockerconfigjson`](https://kubernetes.io/docs/concepts/configuration/secret/#docker-config-secrets)
— i.e. the Docker `config.json` `auths` structure — **reused as is**:

```json
{
  "auths": {
    "registry.example.com": {
      "auth": "base64(username:password)"
    },
    "registry.example.com:5000": {
      "auth": "base64(username:password)"
    }
  }
}
```

Lookup rule: when accessing an ingredient at `ref`, the consumer selects the
`auths` entry whose key equals the `host[:port]` **actually contacted** — for
a consumer that substitutes the source endpoint (cascaded zones, local
mirrors), that is the substituted host, not the nominal host recorded in the
recipe; absent an entry, access is anonymous. Consumers supply credential files through their
own configuration (file path, environment, or mounted Kubernetes Secret).

Rationale for reusing this format unchanged:

- **No secrets in recipes** — separation is structural, not conventional:
  there is no field in which a credential could legally appear.
- **Native Kubernetes interoperability** — the same Secret already used as
  an `imagePullSecret` by the cluster that will run the content can be
  mounted, unmodified, into the transfer tool; no format conversion, no
  duplicate secret management.
- **Ubiquitous tooling** — every OCI ecosystem tool (kubelet, docker,
  helm, oras, crane, skopeo) already reads this structure; credentials
  provisioning integrates with existing secret managers and rotation
  processes.

### 13.3 Registry allow-lists

Which source and destination registries a tool may talk to is a consumer
policy (e.g. Tobby’s allow-list configuration), deliberately outside the
format: a recipe cannot grant itself access to a registry that zone policy
does not allow.

## 14. Security considerations

### 14.1 Content integrity

Cooked recipes pin all content by digest; consumers MUST verify digests on
every fetch and before every push ([§12.3](#123-verification)). Tags are
resolution conveniences only; after cooking, tags never participate in trust
decisions.

### 14.2 Time-of-check / time-of-use

Between constraint resolution (draft) and cooking, upstream tags can be
re-pointed. Cooking pipelines SHOULD resolve tags to digests **once**,
perform all checks (scans, reviews) **on the digest-identified content**, and
record those same digests — never re-resolving a tag between check and
publication.

### 14.3 Mutable tags and confusion attacks

`latest`-style mutable tags in drafts, tag re-pointing in cookbooks, and
`artifactType` mismatches are all resolution-confusion vectors addressed
respectively by [§9.1](#91-exact-tags), [§11.3](#113-naming-and-tags), and
[§7.3](#73-ociartifact). Strict field validation ([§4.3](#43-strict-validation))
prevents pinning fields from being silently ignored.

### 14.4 Trust root distribution

Offline zones cannot consult transparency logs or OCSP-style services; the
security of the scheme reduces to the integrity of the configured public
keys. Trust roots MUST be distributed through a channel independent of the
content path (e.g. baked into tool configuration under change control), and
rotations SHOULD use overlapping key sets ([§12.3](#123-verification)).

### 14.5 FileSet extraction safety

Extraction processes untrusted archive content and MUST defend accordingly:

- reject absolute paths and any path containing `..` components;
- never let symlinks or hardlinks escape the destination root (resolve and
  check every link target; reject or rewrite links pointing outside);
- ignore file types other than regular files, directories, and symlinks
  (no device nodes, FIFOs, sockets);
- do not apply setuid/setgid bits by default;
- enforce configurable limits on extracted size, file count, and path depth
  (decompression-bomb defense);
- apply whiteouts strictly per OCI layer semantics so files deleted in later
  layers cannot resurface.

### 14.6 Vendoring trade-off

`vendorDependencies: true` deliberately trades upstream signature continuity
for self-containment ([§7.2](#72-helmchart)). Organizations SHOULD prefer
explicit per-dependency ingredients where practical, and require the
upstream-digest annotation when vendoring is used.

### 14.7 No secrets, ever

No field of this format carries secret material, and [§13](#13-registry-access-and-credentials)
keeps credentials out of band by construction. Reviews SHOULD nevertheless
watch `metadata.annotations` (free-form strings) for accidental secret
leakage, as they would any committed text file.

## 15. Examples

Complete, schema-valid examples live in [`examples/`](examples/):

| File | Illustrates |
| --- | --- |
| [`examples/wordpress.yaml`](examples/wordpress.yaml) | Cooked recipe: one `HelmChart` + two multi-platform `ContainerImage`s, all digest-pinned. |
| [`examples/ai-model.yaml`](examples/ai-model.yaml) | Draft recipe: an `OCIArtifact` AI model plus its inference-server image, mixing pinned and constraint-based ingredients. |
| [`examples/fileset.yaml`](examples/fileset.yaml) | Cooked recipe with an extractable `FileSet` (configuration bundle with `extract.paths`). |
| [`examples/retriever-zone.yaml`](examples/retriever-zone.yaml) | A zone `Retriever` mixing exact versions and constraints, with a per-entry cookbook override. |

## 16. JSON Schemas

Machine-readable schemas (JSON Schema draft 2020-12, strict — unknown fields
rejected per [§4.3](#43-strict-validation)) are published in
[`schemas/`](schemas/):

- [`schemas/recipe.schema.json`](schemas/recipe.schema.json)
- [`schemas/retriever.schema.json`](schemas/retriever.schema.json)

The schemas validate structure and syntax. The following rules of this
specification are **not** expressible in JSON Schema and MUST be enforced by
tooling (the Go SDK in [`recipe/v1alpha1`](recipe/v1alpha1/) implements all
of them):

- ingredient `name` uniqueness within a recipe ([§6.2](#62-specingredients));
- the cooked profile: digest present on every ingredient and exact-tag
  versions only ([§8](#8-draft-and-cooked-recipes));
- constraint-expression grammar beyond surface syntax ([§9](#9-version-constraint-syntax));
- name/tag consistency with the publication location ([§11.3](#113-naming-and-tags)).
