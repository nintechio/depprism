# Integration recipes

These recipes keep dependency evidence separate from vulnerability, license, and artifact-trust decisions. DepPrism explains committed change; another control can decide whether the resulting package is acceptable.

## Pull-request review

```yaml
name: Dependency evidence
on: [pull_request]

permissions:
  contents: read

jobs:
  review:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
        with:
          fetch-depth: 0
      - id: depprism
        uses: nintechio/depprism@v1
      - if: always()
        run: |
          echo "Added: ${{ steps.depprism.outputs.added }}"
          echo "High-risk evidence: ${{ steps.depprism.outputs['high-risk'] }}"
```

Use the Action's own conclusion as the policy gate. `if: always()` is useful only for downstream reporting because a policy failure intentionally stops normal later steps.

## Local review before opening a pull request

```bash
depprism git --base origin/main
```

This compares committed `HEAD` with the specified base. Commit the work first, or point `--head` at another commit. DepPrism does not mix uncommitted working-tree content into the review.

## Save complete JSON evidence

```bash
depprism git --base origin/main --format json > dependency-review.json
```

The JSON contains every change even when the Markdown renderer caps display rows. Treat the file as potentially sensitive: source URLs and package names can reveal private repository structure.

## Monorepositories

No path list is required. Git mode discovers every changed supported primary file at any depth and resolves its companion manifest from the same directory and commit.

Use one root `.depprism.json` policy. A source allowlist must cover every enabled ecosystem in the repository. If teams need materially different trust policies, run separate jobs against separate base branches or repositories rather than assuming a pull-request-controlled policy path is trusted.

## Evidence-only rollout

Start with the built-in source/integrity protections and keep new Git/build findings visible without blocking:

```json
{
  "$schema": "https://raw.githubusercontent.com/nintechio/depprism/main/policy.schema.json",
  "fail_on": ["integrity-drift", "source-drift"],
  "max_added": 0,
  "deny_new_git_sources": false,
  "deny_new_build_behavior": false,
  "allowed_source_prefixes": [],
  "allowed_packages": []
}
```

Observe normal automated update pull requests, then enable one rule at a time. Build-behavior enforcement should be enabled only after reading the ecosystem evidence table: not every lock format records it.

## Pair with a vulnerability scanner

Run DepPrism first to explain the transition and a vulnerability scanner second to evaluate current advisories. Keep their results distinct:

- DepPrism can say “the source changed without a version change.”
- A vulnerability scanner can say “this coordinate matches a current advisory.”
- Neither statement alone proves that an artifact is benign.

This separation makes failures easier to investigate and avoids presenting registry availability as a requirement for deterministic change review.
