# Ecosystem evidence

DepPrism reports only evidence present in committed files. It does not query a registry or execute a package manager to fill gaps. This makes the result reproducible and private, but each lock format has different limits.

## npm

Supported formats: `package-lock.json` and `npm-shrinkwrap.json`, lockfile versions 1–3.

- Package name, version, resolved source, integrity, directness, development/optional scope, and graph edges are normalized.
- `hasInstallScript` is treated as recorded build/install behavior.
- Version 2 and 3 roots come from the root package record, including development and optional dependencies.
- Version 1 nested records and `requires` edges are normalized deterministically.

## pnpm

Supported lockfile versions: major versions 5–10.

- Importers identify direct production, development, and optional dependencies.
- `packages` provides identity, source, integrity, and `requiresBuild` evidence.
- Newer `snapshots` records provide graph edges when present; older files use package dependency records.
- Peer-suffix package keys are normalized while the original lock key remains metadata.

## Yarn Classic and Berry

- Classic v1 selectors, versions, resolved sources, integrity fields, dependency edges, and optional dependency edges are parsed.
- Berry metadata, selector groups, resolutions, checksums, dependency edges, and link types are parsed.
- `package.json` supplies directness and scope when available.
- Yarn lockfiles do not reliably record lifecycle scripts, so build behavior is not inferred.

## uv

Supported primary file: `uv.lock` version 1 and later.

- Names follow Python's normalized-name rules.
- Versions, registry/Git/URL/path/workspace sources, artifact hashes, and dependency edges are normalized.
- `pyproject.toml` supplies direct production, optional, development, and dependency-group evidence.
- Source-distribution and wheel counts are retained as metadata. The presence of an sdist does not by itself prove the target installation will build it, so DepPrism does not label that as build behavior.

## Poetry

- Package name, version, groups/category, source, file hashes, and dependency names are normalized.
- `pyproject.toml` supplies direct production, optional, and development evidence for modern and legacy Poetry tables.
- When a dependency name has multiple locked candidates and the lock record does not identify the exact target, DepPrism uses the first stable candidate and emits `ambiguous-dependency-edge`.
- Poetry lockfiles do not prove target-platform build execution.

## Cargo

- Cargo.lock formats, crate names, versions, registry/Git sources, checksums, and dependency edges are normalized.
- Top-level, target-specific, build, development, and workspace dependency declarations in `Cargo.toml` provide directness.
- Renamed dependencies use the manifest's `package` field.
- `Cargo.lock` does not record which crates contain `build.rs`, so build behavior is not inferred.

## Go modules

- `require` blocks and single-line directives provide the selected module set and direct/indirect classification.
- `replace` directives preserve the original coordinate and report the replacement source/version.
- `go.sum` supplies module archive hashes when available; `/go.mod` hashes are intentionally excluded.
- `go.mod` does not contain the complete package or module edge graph, so transitive causal paths are not invented.

## Warnings are evidence

Warnings are part of the JSON schema and Markdown report. Callers should retain them. A warning means the input format could not support a conclusion; it does not mean the parser silently accepted a safe result.
