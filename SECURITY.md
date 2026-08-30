# Security policy

## Supported versions

Security fixes are provided for the latest stable major release. Update to the newest patch release before reporting an issue that may already be fixed.

| Version | Supported |
|---|---|
| Latest `1.x` | Yes |
| Pre-release and development snapshots | Best effort |

## Reporting a vulnerability

Use this repository's **Security** tab to open a private GitHub Security Advisory. Do not include exploit details, credentials, private registry URLs, private lockfiles, or personal data in a public issue.

Please include:

- affected DepPrism version or commit;
- operating system and installation method;
- the smallest safe fixture or repository shape;
- expected trust boundary and observed bypass;
- reproduction steps, exit code, and impact;
- any proposed mitigation.

We aim to acknowledge a report within 72 hours. Validation and remediation timelines depend on severity and reproducibility. We will coordinate disclosure and credit with the reporter unless anonymity is requested.

## In scope

- loading pull-request policy from an untrusted revision;
- command, path, Markdown, JSON, or workflow-annotation injection;
- accepting malformed policy permissively;
- a parser panic or unbounded behavior on crafted committed input;
- materially incorrect dependency evidence with a security impact;
- unsafe container permissions, archive construction, checksums, or release provenance.
- widening the privileged entrypoint beyond command-directory traversal and runner-provided summary/output write access.

Registry compromise, package malware detection, and undisclosed upstream advisories are outside DepPrism's offline evidence boundary. See the complete [security model](docs/security-model.md).
