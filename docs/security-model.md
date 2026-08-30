# Security model

DepPrism reduces dependency-review noise; it does not establish that a dependency is safe.

## Trust boundaries

In pull-request review:

- the base commit and GitHub event payload are trusted workflow inputs;
- the head commit, changed paths, lockfile content, package names, sources, and checksums are untrusted;
- `.depprism.json` is loaded from the base commit;
- the container and pinned workflow dependencies are part of the execution boundary;
- output files named by `GITHUB_STEP_SUMMARY` and `GITHUB_OUTPUT` are runner-provided channels.

DepPrism resolves refs to commits, reads file bytes from Git objects, invokes Git without a shell, rejects unsafe repository paths, strictly decodes policy, validates normalized graph references, sanitizes human output, and escapes GitHub workflow commands.

GitHub creates its Docker Action command-channel files as the host runner user. The container entrypoint starts as root, grants the dedicated analyzer group traverse access to the GitHub-managed command directory and write access to only the two exact runner-provided files beneath it, leaves the host owner unchanged, and immediately executes the analyzer through `su-exec` as the dedicated UID 10001 user. Paths are resolved before the `/github/file_commands` boundary is checked. No repository-controlled content is parsed before privileges are dropped.

## What offline evidence can prove

DepPrism can prove that committed dependency evidence changed in a particular way. For example, a checksum or source changed without a version change, a new Git resolution appeared, or an npm/pnpm record newly declares install/build behavior.

It cannot prove:

- that a package or maintainer is trustworthy;
- that a checksum corresponds to a benign artifact;
- that a version has no current vulnerability advisory;
- that an install will select the same platform artifact in every environment;
- that an omitted or unsupported field means a behavior is absent;
- that the reviewed commit will be the commit eventually deployed.

Use registry provenance, vulnerability scanning, artifact verification, sandboxed builds, and protected-branch controls as separate layers.

## Resource considerations

Lockfiles are parsed in memory and Git output is captured in memory. Run the Action only on repositories whose committed dependency files fit normal CI memory limits. The parsers cap line-scanner tokens at 4 MiB where line-oriented formats are used; structured decoders retain their library limits.

DepPrism never executes content from a dependency file. It does not fetch missing Git objects. The checkout must contain the base and head commits.

## Reporting security issues

Potential policy bypasses, output injection, unsafe Git argument behavior, parser panics on crafted committed input, and release provenance failures should be reported privately according to [SECURITY.md](../SECURITY.md).
