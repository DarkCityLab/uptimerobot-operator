# Security Policy

Thanks for helping keep UptimeRobot Operator and its users safe.

> This is an independent, spare-time project and is **not** affiliated with
> UptimeRobot. Vulnerabilities in the hosted UptimeRobot service or its API
> should be reported to UptimeRobot directly — this policy covers only the
> operator (its code, container image, and Helm chart).

## Supported versions

This project is pre-1.0 and ships frequently. Security fixes are made against the
**latest released version** only; please upgrade before reporting.

| Version              | Supported          |
| -------------------- | ------------------ |
| Latest `0.1.x` release | :white_check_mark: |
| Older releases       | :x:                |

## Reporting a vulnerability

**Please do not open a public issue, pull request, or discussion for security
problems**, and do not disclose the issue publicly until a fix is available.

Report privately through GitHub's private vulnerability reporting:

1. Go to the repository's **Security** tab → **Report a vulnerability**, or open
   <https://github.com/DarkCityLab/uptimerobot-operator/security/advisories/new>.
2. Describe the issue with enough detail to reproduce it: affected version,
   configuration (CR / annotations, redact secrets), impact, and any
   proof-of-concept.

This creates a private advisory visible only to the maintainers. If you are
unable to use that flow, contact the maintainer
[@ogizhelev](https://github.com/ogizhelev) and ask for a private channel — again,
without disclosing details publicly.

### What to expect

As a spare-time project, responses are best-effort rather than bound to a strict
SLA, but the aim is:

- An acknowledgement within about **5 business days**.
- An initial assessment (accepted / needs-info / not-a-vuln) once reproduced.
- Coordinated disclosure: a fix released and a
  [GitHub Security Advisory](https://github.com/DarkCityLab/uptimerobot-operator/security/advisories)
  published, crediting you unless you prefer to remain anonymous.

## Scope

In scope:

- The operator source code and reconciliation logic.
- The published container image and Helm chart.
- Handling of the UptimeRobot API token and endpoint credentials (e.g. the
  Secret-backed `passwordSecretRef` / `auth-secret` flow).

Out of scope:

- The UptimeRobot service and its public API.
- Vulnerabilities requiring cluster-admin or pre-existing compromise of the
  cluster the operator runs in.
- Insecure user configuration (e.g. granting the operator excess RBAC, or
  storing tokens outside a Secret).

## Handling of secrets

The UptimeRobot API token and any endpoint credentials are referenced by Secret
name and read at runtime — they are never meant to appear in a `Monitor` spec,
an annotation, or logs in plaintext. If you find a path where a credential is
logged or otherwise exposed, please report it as described above.

## Verifying release artifacts

Release images and Helm charts are signed keylessly with
[cosign](https://github.com/sigstore/cosign) via GitHub OIDC (no long-lived
keys). Verify them before use — see
[**Verifying signatures (cosign)**](README.md#verifying-signatures-cosign) in the
README for the exact commands.
