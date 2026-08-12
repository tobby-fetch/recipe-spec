<h1 align="center">Recipe Spec</h1>

<p align="center">
  <em>One YAML document that says exactly what your software delivery is made of.</em>
</p>

---

A **Recipe** is a declarative, signable, fully pinned description of the OCI
artifacts — container images, Helm charts, AI models, files — that together
make up a software delivery. Every ingredient is pinned to an exact content
digest, so a single signature attests the whole delivery. Recipes are built to
travel across isolated network zones, all the way to air-gapped environments.

This repository publishes the format under **Apache-2.0** so that any tool can
read, validate, and emit Recipes:

| Content | Where |
|---|---|
| **Specification** of the `Recipe` and `Retriever` kinds (`recipe.tobby.dev/v1alpha1`, draft) | [RECIPE-SPEC.md](RECIPE-SPEC.md) |
| **JSON Schemas** (draft 2020-12, strict — unknown fields rejected) | [`schemas/`](schemas/) |
| **Examples** (cooked and draft recipes, a zone Retriever) | [`examples/`](examples/) |
| **Go SDK** (parsing, validation, serialization) | [`recipe/v1alpha1`](recipe/v1alpha1/) — see [Go SDK](#go-sdk) |
| **CLI** (`recipe lint`: validation from the shell and CI) | [`cmd/recipe`](cmd/recipe/) — see [CLI](#cli) |
| **Guides** (publishing with OCI tooling, packaging a FileSet) | [website guides](https://tobby-fetch.github.io/recipe-spec/) — see [Guides](#guides) |

Highlights of the format:

- **Draft vs cooked** — a published (“cooked”) recipe is fully digest-pinned
  and immutable: one cosign signature transitively attests the exact bytes of
  every ingredient.
- **Four ingredient kinds** — `ContainerImage`, `HelmChart`, `OCIArtifact`,
  `FileSet` (arbitrary files packaged as an OCI image, mountable or
  extractable).
- **Recipes are OCI artifacts** — published to a *cookbook* (a plain OCI
  repository), so the same registries, transport, and signing machinery apply
  to recipes and to their contents.
- **No secrets, ever** — registries are referenced by hostname only;
  credentials stay out of band (`kubernetes.io/dockerconfigjson`, reused
  as-is).
- **Relocation convention** (§11.5) — a deterministic destination path for
  every ingredient, invariant across any number of zone hops.

Recipes are the format that drives
[**Tobby**](https://github.com/tobby-fetch/tobby-fetch), the transfer tool for
OCI assets across network zones.

> 📜 **Status: `v1alpha1` (draft).** Breaking changes are possible until
> `v1beta1`; the format is versioned by the in-document `apiVersion` and
> follows the Kubernetes deprecation philosophy. Review and issues welcome.

Landing page: **https://tobby-fetch.github.io/recipe-spec/**

## Go SDK

The reference implementation of the format lives in this module:

```sh
go get github.com/tobby-fetch/recipe-spec
```

```go
import v1alpha1 "github.com/tobby-fetch/recipe-spec/recipe/v1alpha1"

r, err := v1alpha1.ParseRecipe(data) // strict: unknown fields are rejected
if err != nil {
    var errs v1alpha1.ErrorList      // one entry per violation, with field path
    // ...
}
if err := r.Validate(v1alpha1.ProfileCooked); err != nil {
    // not publishable: missing digests or unresolved version constraints
}
```

Parsing validates against the embedded JSON Schemas plus the rules the
specification delegates to tooling (ingredient name uniqueness, the §9
constraint grammar). A parsed recipe is always a valid **draft**; the
**cooked** profile additionally requires a digest and an exact-tag version
on every ingredient (§8). `ParseConstraint` implements the §9 version
grammar (`Match` a tag, `Resolve` a tag list; `||` is rejected). See the
[package documentation](https://pkg.go.dev/github.com/tobby-fetch/recipe-spec/recipe/v1alpha1)
for details.

## CLI

The `recipe` command wraps the SDK for shell and CI use:

```sh
go install github.com/tobby-fetch/recipe-spec/cmd/recipe@latest
```

```sh
recipe lint examples/                     # draft profile (the default)
recipe lint --profile cooked recipe.yaml  # publishable? (§8: digests + exact tags)
recipe lint --output json manifests/      # machine-readable findings
```

`recipe lint` parses and validates every given document through the SDK —
never through logic of its own. A directory argument stands for every
`*.yaml`/`*.yml` file under it, recursively, and multi-document streams
(`---` separators) are accepted, each document validated independently
(§5). Findings are reported one per line —
`<file>[#doc N]: <path>: <message> (<rule>)` — or, with `--output json`,
as a stable array of `{file, document, path, rule, message}` objects.
Exit codes: 0 all documents valid, 1 findings, 2 usage or I/O error.

## Guides

Two hands-on guides complement the specification:

- [**Publishing recipes with standard OCI tooling**](https://tobby-fetch.github.io/recipe-spec/guides/publishing-recipes/)
  — push a cooked recipe to a cookbook with `oras`, sign it with `cosign`
  (key-based, offline-verifiable), and copy it across zones with its
  signature (§11–§12).
- [**Packaging a FileSet**](https://tobby-fetch.github.io/recipe-spec/guides/packaging-a-fileset/)
  — build a reproducible, single-layer OCI image out of arbitrary files
  with a deterministic digest, honoring the extraction safety rules
  (§7.4, §14.5).

## License

The specification, JSON Schemas, examples, and Go SDK are licensed under the
[Apache License 2.0](LICENSE). Copyright © 2026 infraBuilder SASU and
contributors. The Tobby application is published separately under the
GPL-3.0 license.
