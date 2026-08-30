# CLI and output

## Commands

```text
depprism diff [options] BEFORE AFTER
depprism git [options]
depprism inspect [options] FILE
depprism version
```

`diff` compares two files from the local filesystem. Companion manifests are resolved beside each input.

`git` discovers changed supported files between two commits and reads the primary and companion files directly from Git objects. It does not use package-manager output or the working-tree contents as policy evidence.

`inspect` emits a normalized snapshot as JSON or a one-line summary. It is useful when building integrations or reporting a parser issue.

## Formats

- `text` is compact terminal output.
- `markdown` is a GitHub-flavored review table capped at 200 displayed rows. Counts and policy decisions always cover the complete report.
- `json` emits every report using schema version `1`.
- `github` prints text and workflow annotations, appends Markdown to `GITHUB_STEP_SUMMARY`, and writes scalar outputs to `GITHUB_OUTPUT` when those paths are provided.

All untrusted text from lockfiles is sanitized before terminal, Markdown, or workflow-command rendering. JSON uses the standard encoder with HTML escaping.

## JSON shape

The top-level object is a repository `Review`:

```json
{
  "schema_version": 1,
  "base": "optional commit ID",
  "head": "optional commit ID",
  "reports": [],
  "summary": {
    "added": 0,
    "removed": 0,
    "updated": 0,
    "metadata": 0,
    "direct": 0,
    "high_risk": 0,
    "policy_failures": 0
  },
  "passed": true
}
```

Each report includes formats, normalized changes, causal path, evidence findings, warnings, policy findings, a summary, and its pass state. Additive fields may be introduced within schema version 1; incompatible structural changes require a new schema version.

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Analysis completed and policy passed. |
| `1` | Analysis completed and policy failed. |
| `2` | Input, policy, Git state, or output could not be evaluated safely. |

Automation should distinguish `1` from `2`: the former is a dependency decision, while the latter means no reliable decision was produced.
