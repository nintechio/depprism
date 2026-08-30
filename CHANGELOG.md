# Changelog

All notable changes to DepPrism are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and releases use semantic versioning.

## [Unreleased]

## [1.0.1] - 2026-08-30

### Fixed

- Allow the non-root Docker analyzer to write GitHub's runner-owned step summary and output channels through a narrowly scoped, privilege-dropping entrypoint.

## [1.0.0] - 2026-08-30

### Added

- Offline semantic review for npm, pnpm, Yarn Classic and Berry, uv, Poetry, Cargo, and Go modules.
- Package-manager-neutral dependency graphs with direct/transitive evidence and shortest known introduction paths.
- Findings for same-version source and integrity drift, new Git sources, and newly recorded build behavior.
- Strict JSON policy, text/Markdown/JSON/GitHub output, stable exit codes, and repository-level Git review.
- Base-commit policy trust for pull-request review.
- Non-root Docker Action and reproducible release archives with checksums and artifact attestations.

[Unreleased]: https://github.com/nintechio/depprism/compare/v1.0.1...HEAD
[1.0.1]: https://github.com/nintechio/depprism/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/nintechio/depprism/releases/tag/v1.0.0
