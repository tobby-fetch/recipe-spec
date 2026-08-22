# Changelog

All notable changes to this project are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
The Recipe/Retriever format itself is versioned Kubernetes-style through its
`apiVersion` (`recipe.tobby.dev/v1alpha1` → `v1beta1` → `v1`); the Go SDK
module follows Go's module versioning conventions.

## [Unreleased]

### Changed

- **`cookbook`: manifest serialization now matches the OCI reference
  structs and the `§11.2` example.** The internal manifest struct
  serialized `artifactType` after `layers`, and its descriptors put `size`
  before `digest` — two field-order inversions against
  `github.com/opencontainers/image-spec` (the structs `oras-go` and
  `go-containerregistry` serialize with) and against the normative example
  of `§11.2`. The same content therefore produced a different manifest
  digest than the common Go OCI libraries. `Build`'s output is now
  byte-identical to the reference serialization, pinned by a test against
  `image-spec` (a test-only dependency).
- **`cookbook.DecideRepublication` compares recipe document layer digests,
  not manifest digests.** Byte-identical manifests across tools were never
  achievable — generic publishers add manifest annotations such as
  `org.opencontainers.image.created` — so comparing manifest digests turned
  honest republications through another tool into spurious immutability
  conflicts. `§11.2`/`§11.3` now state that the manifest digest identifies
  one published artifact (it is what gets signed) but is NOT a stable
  identity for the recipe across tools; the recipe's semantic identity is
  the digest of its single document layer, and republication decisions are
  made on it. Callers now pass `Layout.Document.Digest` (from
  `VerifyManifest` on the published manifest) and
  `Artifact.Document.Digest`.
- **SDK: parsing input is bounded by `v1alpha1.MaxParseBytes` (4 MiB).**
  Oversized input — previously accepted at a memory cost of hundreds of
  megabytes — is rejected immediately with the new `document-too-large`
  rule, before any decoding. `§5` now states the document size bound,
  aligned with the `§11.2` layer bound (`cookbook.MaxDocumentBytes`).
- **Schemas: free-form metadata strings are bounded.** `metadata.version`
  is capped at 128 characters (it becomes the OCI publication tag, `§6.1`,
  `§11.3` — the SDK also enforces OCI-tag validity semantically),
  `metadata.description` at 2048, and annotation values at 4096, in both
  the Recipe and Retriever schemas.
- `§9.2` now specifies the partial-version semantics the SDK always
  implemented: partial versions in comparison operators designate series
  (`>1.2` ≡ `>=1.3.0`, `<=3` ≡ `<4.0.0`, `=1.2` ≡ the `1.2.x` series), the
  exact caret and tilde bounds on 1- and 2-component literals, and the
  version-literal grammar (1–3 numeric components, pre-release only on full
  triples, rejected forms). Common constraint libraries read `>1.2` as
  `>1.2.0`; that reading is now explicitly non-conforming.
- Go toolchain raised to 1.25.13 (`mise.toml`, `go.mod`, CI via
  `go-version-file`); `actions/checkout` and `actions/setup-go` aligned
  across workflows to v7.0.1 / v7.0.0 and the GitHub Pages workflow pinned
  by full commit SHA (`withastro/action` v3.0.2, `actions/deploy-pages`
  v4.0.5, previously mutable `@v3`/`@v4` tags).

### Fixed

- **`cookbook.VerifyManifest` now validates what consumers act on.** It
  requires well-formed `sha256`/`sha512` descriptor digests (same pattern
  as the recipe schema — a digest is the consumer's next blob request and
  must never carry garbage or path traversal into a registry client), a
  strictly positive document layer size within the 4 MiB bound, and the
  one canonical `§11.2` empty config (size 2, the known digest of `{}`).
- `recipe lint` on arguments that name no YAML document (e.g. an empty
  directory) exited 0 with "all valid", letting a mistyped path pass a CI
  gate. It now exits 2 with "no recipe documents found".
- The schema violation for an `extract.paths` entry containing `..`
  surfaced as an inscrutable `'not' failed`; it now reads "path must not
  contain '..' components (§7.4)".
- `RECIPE-SPEC.md` no longer promises "the Go SDK that lands with the
  first tagged release": the SDK is in the repository, and the
  introduction and `§16` point at it.

### Added

- The `testdata/invalid` corpus (45 crafted documents) is now also
  validated in the rejection direction against the **raw JSON Schemas**,
  with an explicit, commented exemption list for the rules that are
  delegated to tooling (`§16`) and out of a JSON Schema's reach.
- CI: a `govulncheck` job (pinned via `go run golang.org/x/vuln@v1.7.0`)
  and a `gitleaks` job (8.30.1, checksum-verified release binary scanning
  the full git history — the CI counterpart of the pre-commit hook).
- `renovate.json`: weekly grouped updates for Go modules, GitHub Actions
  (digest-pinned), the mise toolchain, and the `*_VERSION` pins in
  workflows.
- `docs/spikes/helm-library-vs-binary.md`: the measured decision record
  behind rendering Helm charts through an external `helm` binary in the
  planned `recipe cook` tool.

- Repository governance: license, `CONTRIBUTING.md`, `SECURITY.md`, DCO
  enforcement in CI, issue and pull request templates.
- `recipe.tobby.dev/v1alpha1` draft: specification document
  (`RECIPE-SPEC.md`), JSON Schemas (draft 2020-12, strict) for the `Recipe`
  and `Retriever` kinds, and example recipes.
- Go SDK (`recipe/v1alpha1`): strict parsing, draft and cooked profile
  validation, and the `§9` version-constraint grammar (`ParseConstraint`,
  `Match`, `Resolve`).
- `§11.2`: the layer's `org.opencontainers.image.title` annotation is
  stated to be illustrative rather than normative. It was only ever shown
  in the example manifest, no implementation checked it, and requiring it
  would contradict the compatibility with generic OCI tooling promised in
  the same paragraph — that tooling writes whatever file name it was
  handed. The publishing guide claimed the opposite and is corrected.
- Go SDK (`cookbook`): the registry-independent half of `§11` — `Build`
  validates a document and assembles the `§11.2` artifact for a given
  publication location (refusing drafts per `§8` and metadata that
  disagrees with the location per `§11.3`), `VerifyManifest` enforces the
  same layout on the consuming side, and `DecideRepublication` states what
  writing an existing tag means under `§8` immutability. No network, no
  registry client, no new dependency: implementations share what a recipe
  artifact *is* and keep their own transport.
- CLI (`cmd/recipe`): `recipe lint` validates files and directories
  through the SDK, with draft and cooked profiles, multi-document YAML
  streams (each document validated independently, `§5`), text or JSON
  reporting, and CI-friendly exit codes (0 valid, 1 findings, 2 usage or
  I/O error). Install with
  `go install github.com/tobby-fetch/recipe-spec/cmd/recipe@latest`.
- Website guides, linked from the README and the landing page:
  *Publishing recipes with standard OCI tooling* (`oras` push per the
  `§11.2` artifact layout, key-based `cosign` signing and offline
  verification, signature-preserving copies across zones) and *Packaging
  a FileSet* (reproducible single-layer OCI image with a deterministic
  digest, producer-side `§14.5` safety rules, mount and extraction
  consumption).
- CI: lint (`gofmt`, `golangci-lint`), build, `go vet`, example validation
  against the SDK and schemas, and race-detected tests with coverage.

[Unreleased]: https://github.com/tobby-fetch/recipe-spec/compare/main...HEAD
