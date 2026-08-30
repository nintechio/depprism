# Architecture

DepPrism separates package-format parsing from review policy:

```text
committed lockfile + companion manifest
                  │
                  ▼
        ecosystem-specific parser
                  │
                  ▼
       normalized dependency graph
                  │
          semantic comparison
                  │
                  ▼
    evidence findings + causal paths
                  │
       trusted policy evaluation
                  │
                  ▼
       text / Markdown / JSON / CI
```

## Normalized package identity

A package record includes name, version, source, integrity, scope, directness, recorded build behavior, graph edges, and format-specific metadata. Its internal ID is a deterministic digest of ecosystem, name, version, and normalized source. The human-readable prefix is not used as a trust decision.

Separate records are retained when the same package name/version resolves from different sources. Graph references must point to an existing record, and snapshots validate these invariants before comparison.

## Comparison

Packages are grouped by name. Exact version/source records are matched first; metadata differences become metadata changes. Remaining stable-sorted records are paired as updates, with unpaired records becoming additions or removals.

The shortest known path is a breadth-first search from direct roots. When an ecosystem cannot provide graph edges, the report shows the package without inventing a chain.

## Git mode

Git refs are resolved to commit object IDs before review. Changed supported paths are read using `git cat-file`; companion reads use the same commit. The configured policy path is read only from the resolved base commit. Git is executed with argument arrays, never through a shell.

This design keeps pull-request-controlled filenames, refs, and lockfile contents out of shell parsing and prevents head policy from authorizing its own change.

## Dependency surface

The core uses the Go standard library plus two parsing libraries: `go-toml/v2` for TOML and `yaml/v3` for YAML. JSON, Yarn Classic, and Go module parsing are implemented with standard-library primitives. No package manager SDK, registry client, telemetry library, or model client is included.
