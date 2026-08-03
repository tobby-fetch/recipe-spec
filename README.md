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

- the **specification** of the `Recipe` and `Retriever` kinds,
- the **JSON Schemas**,
- the **Go SDK** (parsing, validation, serialization).

Recipes are the format that drives
[**Tobby**](https://github.com/tobby-fetch/tobby-fetch), the transfer tool for
OCI assets across network zones.

> 📜 **Specification in progress.** The spec, schemas, and SDK are on their
> way. Watch this repository for the first releases.

Landing page: **https://tobby-fetch.github.io/recipe-spec/**

## License

The specification, JSON Schemas, and Go SDK are licensed under the
[Apache License 2.0](LICENSE). The Tobby application is published separately
under the GPL-3.0 license.
