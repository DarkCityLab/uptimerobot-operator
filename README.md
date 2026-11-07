# UptimeRobot Operator

A Kubernetes operator that creates and deletes [UptimeRobot](https://uptimerobot.com)
monitors for services exposed in your cluster, using the UptimeRobot **v3** API.

- Annotate an **HTTPRoute** (Gateway API) or **Ingress** and the operator creates a
  monitor for each hostname automatically.
- Or declare monitors explicitly with the **`Monitor`** custom resource, which
  exposes the full v3 monitor surface (all monitor types and settings).
- Deleting the source object **or** the `Monitor` CR deletes the UptimeRobot
  monitor too (finalizer + owner-reference garbage collection).

See [docs/architecture.md](docs/architecture.md) for diagrams and design details.

## How it works

The `Monitor` CR is the single source of truth. Source watchers translate
annotated HTTPRoute/Ingress objects into owned `Monitor` CRs (one per hostname);
the Monitor controller is the only component that calls UptimeRobot.

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: httpbin
  annotations:
    monitoring.homelab.io/enabled: "true"   # ← opt in
spec:
  hostnames: [ httpbin.example.com ]
```

→ creates a monitor for `https://httpbin.example.com`. Delete the HTTPRoute and
the monitor is deleted.

## Prerequisites

- A Kubernetes cluster (1.30+). Gateway API CRDs are optional — the HTTPRoute
  watcher is enabled only when they are installed.
- An UptimeRobot API token (account or main API key) from the Integrations page.

## Install (Helm)

```sh
# 1. Create the API-token Secret (referenced by name; never committed).
kubectl create namespace uptimerobot-system
kubectl create secret generic uptimerobot-api-key \
  --from-literal=apiKey=YOUR_UPTIMEROBOT_TOKEN \
  -n uptimerobot-system

# 2. Install the chart.
helm install uptimerobot-operator ./dist/chart \
  -n uptimerobot-system
```

The chart defaults to the `ghcr.io/darkcitylab/uptimerobot-operator` image at the
chart's `appVersion`. Override with `--set manager.image.tag=<version>` (or
`--set manager.image.repository=...`) to pin a different image.

Key values (see [`dist/chart/values.yaml`](dist/chart/values.yaml)):

| Value | Default | Purpose |
|-------|---------|---------|
| `uptimerobot.apiKeySecretName` | `uptimerobot-api-key` | Secret holding the token |
| `uptimerobot.apiKeyKey` | `apiKey` | key within the Secret |
| `uptimerobot.mountPath` | `/etc/uptimerobot` | token mount path |
| `uptimerobot.rateLimitPerMinute` | `10` | client-side rate limit (Free plan) |
| `uptimerobot.defaultAlertContacts` | `""` | default alert-contact IDs |
| `manager.image.repository` | `ghcr.io/darkcitylab/uptimerobot-operator` | operator image |
| `manager.image.tag` | `""` (→ chart appVersion) | image tag |

To tune the operator, add flags to `manager.args`, e.g.
`--requests-per-minute=10`, `--default-alert-contacts=8307662`,
`--status-refresh-interval=5m`.

## Usage

### Annotation-driven (HTTPRoute / Ingress)

| Annotation | Purpose |
|------------|---------|
| `monitoring.homelab.io/enabled` | `"true"` to opt in (required) |
| `monitoring.homelab.io/scheme` | override URL scheme (default `https`, or `http` for non-TLS Ingress) |
| `monitoring.homelab.io/path` | append a path, e.g. `/healthz` |
| `monitoring.homelab.io/type` | monitor type (default `HTTP`) |
| `monitoring.homelab.io/interval` | check interval (seconds) |
| `monitoring.homelab.io/timeout` | check timeout (seconds) |
| `monitoring.homelab.io/alert-contacts` | comma-separated alert-contact IDs |

### Explicit `Monitor` CR

```yaml
apiVersion: monitoring.homelab.io/v1alpha1
kind: Monitor
metadata:
  name: example
spec:
  type: HTTP
  url: https://example.com/health
  interval: 300
  timeout: 30
  http:
    method: GET
    successHttpResponseCodes: ["2xx", "3xx"]
  collectStats: true          # poll uptime/response-time into .status
  # paused: true              # pause/resume via the API
  # alertContacts:            # per-CR override of the global default
  #   - alertContactId: 8307662
```

More examples in [`config/samples/`](config/samples/). Supported `type` values:
`HTTP`, `KEYWORD`, `PING`, `PORT`, `HEARTBEAT`, `DNS`, `API`, `UDP`,
`VISUAL_COMPARISON`, `THIRD_PARTY_SERVICE` (per-type required fields are enforced
by CRD validation).

Reset statistics on demand by changing the annotation
`monitoring.homelab.io/reset-stats` to a new value on a `Monitor`.

Inspect status:

```sh
kubectl get monitors
# NAME      TYPE   URL                          MONITOR-ID   MONITOR-STATUS   STATE
# example   HTTP   https://example.com/health   802793596    UP               Active
```

## Verifying signatures (cosign)

Release artifacts are signed keylessly with [cosign](https://github.com/sigstore/cosign)
via GitHub OIDC (no long-lived keys). Verify the image and chart before use:

```sh
VERSION=0.1.3 # x-release-please-version
IDENTITY="https://github.com/DarkCityLab/uptimerobot-operator/.github/workflows/release.yml@refs/tags/v${VERSION}"
ISSUER='https://token.actions.githubusercontent.com'

# image
cosign verify "ghcr.io/darkcitylab/uptimerobot-operator:${VERSION}" \
  --certificate-identity "$IDENTITY" --certificate-oidc-issuer "$ISSUER"

# chart
cosign verify "ghcr.io/darkcitylab/charts/uptimerobot-operator:${VERSION}" \
  --certificate-identity "$IDENTITY" --certificate-oidc-issuer "$ISSUER"
```

## Development

```sh
make test                 # unit + envtest
make run                  # run locally (set --api-key-file to a local token file)
make docker-build IMG=... # build the operator image
make manifests generate   # regenerate CRDs + deepcopy
```

Run locally against a cluster with the bundled token file:

```sh
make install              # install CRDs
go run ./cmd --api-key-file=./api-key --requests-per-minute=10
```

## Releasing

Releases are automated with [release-please](https://github.com/googleapis/release-please-action):

1. Land changes on `main` using [Conventional Commits](https://www.conventionalcommits.org)
   (`feat:`, `fix:`, `build(deps):`, …). `feat:` → minor bump, `fix:`/others → patch,
   `feat!:`/`BREAKING CHANGE:` → major.
2. release-please keeps an open **"chore(main): release X.Y.Z"** PR that bumps the version
   in `dist/chart/Chart.yaml` (`version` + `appVersion`) and `README.md`, and updates
   `CHANGELOG.md`.
3. **Merge that PR** → release-please creates the `vX.Y.Z` tag + GitHub Release.
4. The tag triggers [`release.yml`](.github/workflows/release.yml), which builds, **signs**
   (cosign), and publishes the multi-arch image and the Helm chart. The chart's image tag
   follows `appVersion`, so image and chart versions always match.

No manual version editing or tagging required. (Requires "Allow GitHub Actions to create
and approve pull requests" enabled in repo/org Actions settings.)

## License

Apache License 2.0 — see [LICENSE](LICENSE).
