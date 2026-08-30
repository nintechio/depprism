# Policy reference

DepPrism's policy is intentionally small. In Git mode, the policy is read from the base commit, so a proposed change cannot weaken the rule used to review itself.

## Bootstrap behavior

When `.depprism.json` does not exist in the base commit, DepPrism uses its built-in default: fail on `integrity-drift` and `source-drift`. Merge the initial policy setup before making the check required if you want stricter rules to apply to subsequent pull requests.

## Fields

### `fail_on`

An array of finding codes that fail the review:

| Code | Meaning |
|---|---|
| `integrity-drift` | Integrity evidence changed while the normalized version did not. |
| `source-drift` | The resolved source changed while the normalized version did not. |
| `new-git-source` | A package now resolves from Git. |
| `build-enabled` | An existing package newly records install/build behavior. |
| `new-build-package` | An added package records install/build behavior. |

The default is `integrity-drift` and `source-drift`.

### `max_added`

Maximum added normalized package records across each report. `0` disables the limit. Package managers can resolve one manifest edit into many transitive additions, so roll this out using observed repository changes rather than the number of edited manifest lines.

### `deny_new_git_sources`

Fails `new-git-source` without requiring that code in `fail_on`.

### `deny_new_build_behavior`

Fails `build-enabled` and `new-build-package`. Only formats that commit this evidence can trigger the rule. DepPrism emits a warning for formats where build behavior cannot be established.

### `allowed_source_prefixes`

When non-empty, every changed package's resulting source must start with one listed prefix. This is literal prefix matching after whitespace and a trailing slash are normalized. Include every registry and approved local/Git prefix used by all enabled ecosystems.

An empty source does not match an allowlist. A package listed in `allowed_packages` bypasses source and finding enforcement.

### `allowed_packages`

Exact normalized package names exempt from policy enforcement. Changes remain visible in the report. Treat this as a narrow, reviewed exception list rather than a general ignore mechanism.

## Strict decoding

Unknown fields, invalid finding codes, duplicate array values, empty allowlist entries, negative limits, malformed JSON, and trailing JSON all return exit `2`. This is an operational error rather than a passing review.

## Example rollout

Start with the default evidence rules:

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

After observing normal automated update pull requests, set a realistic addition ceiling and decide whether Git sources and recorded build behavior require explicit review.
