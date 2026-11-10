# Contributing to UptimeRobot Operator

Thanks for your interest in improving this project! It's an independent,
spare-time project, so please be patient with review turnaround — but
contributions of all sizes are genuinely welcome, from typo fixes to new monitor
features.

By participating, you agree to abide by our
[Code of Conduct](CODE_OF_CONDUCT.md).

> This project is not affiliated with UptimeRobot (see the disclaimer in the
> [README](README.md)). It is a client for UptimeRobot's public v3 API.

## Ways to contribute

- **Report a bug** or **request a feature** — open an issue using one of the
  [issue templates](https://github.com/DarkCityLab/uptimerobot-operator/issues/new/choose).
- **Report a security issue** — please do **not** open a public issue. Use the
  repository's **Security → Report a vulnerability** page so it stays private to
  the maintainers.
- **Send a pull request** — see the workflow below.

## Developer Certificate of Origin (DCO) — required

All commits must be signed off under the
[Developer Certificate of Origin](https://developercertificate.org). This is a
lightweight statement that you wrote the patch (or otherwise have the right to
submit it under the project's Apache-2.0 license).

Add the sign-off automatically with the `-s` flag:

```sh
git commit -s -m "fix: handle empty hostname list"
```

This appends a trailer to your commit message that **must** match your commit
author identity:

```
Signed-off-by: Your Name <your.email@example.com>
```

A CI check (`.github/workflows/dco.yml`) verifies that every commit in a PR is
signed off and will fail the PR otherwise. If you forgot, amend or rebase:

```sh
# last commit only
git commit --amend -s --no-edit

# every commit on your branch (against main)
git rebase --signoff main
```

Then force-push your branch. Commits authored by bots (e.g. Dependabot) are
exempt.

## Commit messages — Conventional Commits

This repo releases with
[release-please](https://github.com/googleapis/release-please-action), which
derives versions and the changelog from
[Conventional Commits](https://www.conventionalcommits.org). Please prefix your
commit subjects accordingly:

| Prefix | Effect | Use for |
|--------|--------|---------|
| `feat:` | minor bump | new user-facing capability |
| `fix:` | patch bump | bug fixes |
| `docs:`, `chore:`, `ci:`, `test:`, `refactor:` | patch bump | non-feature changes |
| `feat!:` / `fix!:` (or `BREAKING CHANGE:` footer) | major bump | breaking changes |

Example: `feat: add KEYWORD monitor alertWhen support`.

## Development setup

Prerequisites:

- Go (the version pinned in [`go.mod`](go.mod)) — the Makefile pins all other
  tools (controller-gen, kustomize, golangci-lint, envtest) and downloads them
  into `./bin` on demand.
- Docker (for building images and running the Kind-based e2e tests).
- A Kubernetes cluster for manual testing. For e2e, use a **dedicated**
  [Kind](https://kind.sigs.k8s.io/) cluster — never your real dev/prod cluster.

Common commands:

```sh
make test          # unit tests + envtest (real API server + etcd)
make lint          # golangci-lint
make lint-fix      # auto-fix style issues
make run           # run the operator locally against your kubeconfig context
make test-e2e      # e2e tests on an isolated Kind cluster
```

Before pushing, please run `make test` and `make lint` (or `make lint-fix`)
locally.

## Project layout & generated code

This is a [Kubebuilder](https://book.kubebuilder.io) project. A few rules keep
things consistent — see [AGENTS.md](AGENTS.md) for the full guide.

**Never hand-edit generated files.** If your change touches API types or
RBAC/kubebuilder markers, regenerate instead of editing the output:

```sh
make manifests     # regenerate CRDs + RBAC from markers
make generate      # regenerate DeepCopy methods
```

Do not edit these (they are overwritten by the commands above):

- `api/**/zz_generated.*.go`
- `config/crd/bases/*.yaml`
- `config/rbac/role.yaml`
- `config/webhook/manifests.yaml`
- `PROJECT`

Also: don't remove `// +kubebuilder:scaffold:*` markers, and don't move
scaffolded files — the CLI expects them in fixed locations.

When you change API types, keep the Helm chart CRDs in sync:

```sh
make manifests sync-chart-crd
```

Add or update an example under [`config/samples/`](config/samples/) when you add
a user-facing field.

## Pull request workflow

1. Fork the repo and create a topic branch off `main`
   (e.g. `fix/empty-hostnames`).
2. Make your change. Add/adjust tests (Ginkgo + Gomega) and docs as needed.
3. Run `make test` and `make lint` locally.
4. Commit with a Conventional Commit subject and **sign off** (`git commit -s`).
5. Open a PR and fill in the [pull request template](.github/PULL_REQUEST_TEMPLATE.md).
6. CI must be green (tests, lint, DCO, CodeQL, chart tests). A review from a
   [code owner](.github/CODEOWNERS) is required before merge.

Keep PRs focused — smaller, single-purpose PRs are reviewed faster.

## License

By contributing, you agree that your contributions will be licensed under the
project's [Apache License 2.0](LICENSE), consistent with section 5 of that
license.
