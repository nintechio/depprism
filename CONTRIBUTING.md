# Contributing

DepPrism should remain deterministic, offline, evidence-led, and safe to run on an untrusted pull request. Contributions are welcome when they preserve those properties and solve a demonstrated dependency-review problem.

## Before opening a pull request

For a parser bug, include the smallest sanitized lockfile and companion manifest that reproduce it, the actual normalized result, and the expected evidence. Never attach credentials, private registry tokens, or a proprietary lockfile you cannot share.

For a new ecosystem or finding, describe:

1. the exact committed evidence the format provides;
2. how package identity and graph edges remain deterministic;
3. what the format cannot prove;
4. false-positive and ambiguity behavior;
5. stable output and policy implications.

Registry calls, source uploads, telemetry, accounts, and model inference are outside the core project's direction.

## Development

DepPrism requires Go 1.22 or newer.

```bash
go test ./...
go test -race ./...
go vet ./...
go build -trimpath -buildvcs=false ./cmd/depprism
```

Run the demonstration:

```bash
go run ./cmd/depprism diff --format text \
  testdata/demo/before/package-lock.json \
  testdata/demo/after/package-lock.json
```

Build release archives on Linux:

```bash
./scripts/build-release.sh 0.0.0-test /tmp/depprism-release
```

## Pull requests

- Keep each pull request focused on one behavior or documentation goal.
- Add a sanitized fixture and focused test for parser behavior.
- Preserve complete failure output when reporting a failing command.
- Update the policy schema and reference together when configuration changes.
- Document evidence gaps instead of filling them with guesses.
- Avoid new dependencies unless the format cannot be handled safely by the existing parsing surface.
- Add an Unreleased changelog entry for user-visible behavior.

By contributing, you agree that your contribution is licensed under the repository's MIT License and that you have the right to submit it.
