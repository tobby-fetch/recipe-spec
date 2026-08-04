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
| **Go SDK** (parsing, validation, serialization) | lands with the first tagged release |

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

## License

The specification, JSON Schemas, examples, and Go SDK are licensed under the
[Apache License 2.0](LICENSE). Copyright © 2026 infraBuilder SASU and
contributors. The Tobby application is published separately under the
GPL-3.0 license.
