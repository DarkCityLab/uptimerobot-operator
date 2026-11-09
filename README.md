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

# 2. (Recommended) Verify the chart's cosign signature before installing.
VERSION=0.1.5 # x-release-please-version
cosign verify "ghcr.io/darkcitylab/charts/uptimerobot-operator:${VERSION}" \
  --certificate-identity "https://github.com/DarkCityLab/uptimerobot-operator/.github/workflows/release.yml@refs/tags/v${VERSION}" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com

# 3. Install the chart from GHCR (OCI).
helm install uptimerobot-operator \
  oci://ghcr.io/darkcitylab/charts/uptimerobot-operator --version "${VERSION}" \
  -n uptimerobot-system
```

The chart defaults to the `ghcr.io/darkcitylab/uptimerobot-operator` image at the
chart's `version` (so chart and image versions always match). Override with
`--set manager.image.tag=<version>` (or `--set manager.image.repository=...`) to
pin a different image. (To install from a local checkout instead of GHCR, use
`helm install uptimerobot-operator ./dist/chart -n uptimerobot-system`.)

Key values (see [`dist/chart/values.yaml`](dist/chart/values.yaml)):

| Value | Default | Purpose |
|-------|---------|---------|
| `uptimerobot.apiKeySecretName` | `uptimerobot-api-key` | Secret holding the token |
| `uptimerobot.apiKeyKey` | `apiKey` | key within the Secret |
| `uptimerobot.mountPath` | `/etc/uptimerobot` | token mount path |
| `uptimerobot.rateLimitPerMinute` | `10` | client-side rate limit (Free plan) |
| `uptimerobot.defaultAlertContacts` | `""` | default alert-contact IDs |
| `uptimerobot.secretWatch.enabled` | `false` | watch credential Secrets for prompt rotation (grants `secrets: list;watch`) |
| `uptimerobot.secretResyncInterval` | `60s` | poll-based rotation interval when `secretWatch` is off |
| `manager.image.repository` | `ghcr.io/darkcitylab/uptimerobot-operator` | operator image |
| `manager.image.tag` | `""` (→ chart version) | image tag |

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
| `monitoring.homelab.io/paused` | `"true"` to pause the generated monitor (resume by removing it or setting `"false"`) |
| `monitoring.homelab.io/reset-stats` | change the value (a nonce) to trigger a one-shot stats reset of the generated monitor |
| `monitoring.homelab.io/http-method` | request method: `HEAD`/`GET`/`POST`/`PUT`/`PATCH`/`DELETE`/`OPTIONS`/`QUERY` |
| `monitoring.homelab.io/auth-type` | `NONE`/`HTTP_BASIC`/`DIGEST`/`BEARER` |
| `monitoring.homelab.io/auth-username` | username for `HTTP_BASIC`/`DIGEST` |
| `monitoring.homelab.io/auth-secret` | name of a Secret (same namespace) holding the password/token |
| `monitoring.homelab.io/auth-secret-key` | key within that Secret (default `password`) |
| `monitoring.homelab.io/http-headers` | custom request headers as a JSON object, e.g. `{"X-Env":"prod"}` |
| `monitoring.homelab.io/http-body` | request body (non-HEAD methods) |
| `monitoring.homelab.io/http-body-type` | `KEY_VALUE` or `RAW_JSON` |
| `monitoring.homelab.io/success-codes` | comma-separated success codes/ranges, e.g. `2xx,3xx` |
| `monitoring.homelab.io/follow-redirects` | `"true"`/`"false"` |
| `monitoring.homelab.io/check-ssl-errors` | `"true"`/`"false"` |
| `monitoring.homelab.io/ssl-expiry-reminder` | `"true"`/`"false"` |
| `monitoring.homelab.io/domain-expiry-reminder` | `"true"`/`"false"` |

#### Authenticated endpoints (annotation or CR)

When the monitored endpoint needs auth, set `auth-type` and point `auth-secret` at a k8s Secret
holding the credential — the value is **never** put in the annotation/spec in plaintext. The same
Secret-key model is available on a standalone `Monitor` via `spec.http.authType` +
`spec.http.passwordSecretRef`. The credential is the **password** for `HTTP_BASIC`/`DIGEST` and the
**token** for `BEARER` (sent as `httpPassword`). The operator reads the Secret and hands the
credential to UptimeRobot, which replays it on each check — the operator itself never pings the
endpoint. A missing/deleted Secret surfaces on the Monitor's status (`State: Error`,
`Ready=False`, reason `DependencyError`) and leaves any existing UptimeRobot monitor untouched.

**Rotation:** by default the operator holds only `get` on Secrets and re-reads + re-pushes a
rotated credential on `--secret-resync-interval` (default 60s). For prompt (seconds) rotation,
enable the Secret watch — `--set uptimerobot.secretWatch.enabled=true` — which grants the operator
`list;watch` on Secrets cluster-wide and watches those labelled
`monitoring.homelab.io/credential=true`.

#### Full HTTPRoute example (all annotations)

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: httpbin
  namespace: default
  annotations:
    monitoring.homelab.io/enabled: "true"          # required opt-in
    monitoring.homelab.io/scheme: "https"           # URL scheme (default https)
    monitoring.homelab.io/path: "/health"           # path appended to each hostname
    monitoring.homelab.io/type: "HTTP"              # monitor type (default HTTP)
    monitoring.homelab.io/interval: "300"           # check interval (seconds)
    monitoring.homelab.io/timeout: "30"             # check timeout (seconds)
    monitoring.homelab.io/alert-contacts: "8307662,8307663"
    monitoring.homelab.io/paused: "false"
    monitoring.homelab.io/reset-stats: "2026-11-07" # change the value to reset stats once
    # --- HTTP request + authentication ---
    monitoring.homelab.io/http-method: "POST"       # HEAD|GET|POST|PUT|PATCH|DELETE|OPTIONS|QUERY
    monitoring.homelab.io/auth-type: "BEARER"       # NONE|HTTP_BASIC|DIGEST|BEARER
    monitoring.homelab.io/auth-username: "svc"      # HTTP_BASIC/DIGEST only
    monitoring.homelab.io/auth-secret: "endpoint-auth"   # Secret name (same namespace)
    monitoring.homelab.io/auth-secret-key: "token"       # key in the Secret (default "password")
    monitoring.homelab.io/http-headers: '{"X-Env":"prod"}'
    monitoring.homelab.io/http-body: '{"ping":true}'
    monitoring.homelab.io/http-body-type: "RAW_JSON"     # KEY_VALUE|RAW_JSON
    monitoring.homelab.io/success-codes: "2xx,3xx"
    monitoring.homelab.io/follow-redirects: "true"
    monitoring.homelab.io/check-ssl-errors: "true"
    monitoring.homelab.io/ssl-expiry-reminder: "true"
    monitoring.homelab.io/domain-expiry-reminder: "true"
spec:
  hostnames: ["httpbin.example.com"]   # one monitor is created per hostname
  parentRefs: [{ name: my-gateway }]
---
apiVersion: v1
kind: Secret
metadata:
  name: endpoint-auth
  namespace: default
  labels:
    # Required ONLY when uptimerobot.secretWatch.enabled=true (prompt rotation).
    monitoring.homelab.io/credential: "true"
stringData:
  token: "s3cr3t-bearer-token"
```

### Explicit `Monitor` CR

A fully-populated HTTP monitor showing every common field:

```yaml
apiVersion: monitoring.homelab.io/v1alpha1
kind: Monitor
metadata:
  name: example-http
  namespace: default
spec:
  # --- common to all types ---
  type: HTTP                       # HTTP|KEYWORD|PING|PORT|HEARTBEAT|DNS|API|UDP|VISUAL_COMPARISON|THIRD_PARTY_SERVICE
  friendlyName: "example /health"  # default: "<namespace>/<name>"
  url: https://example.com/health  # required except HEARTBEAT and THIRD_PARTY_SERVICE
  interval: 300                    # seconds (>= 15)
  timeout: 30                      # seconds (0–60); check types only
  paused: false
  responseTimeThreshold: 2000      # ms
  groupId: 0
  tagNames: ["prod", "web"]
  maintenanceWindowIds: [12345]
  customFields:                    # paid plans only
    team: platform
  region:
    regions: ["na", "eu"]          # na|eu|as|oc
    manualSelected: true
    threshold: { na: 3000, eu: 4000 }
  alertContacts:                   # per-CR; overrides the operator-wide default
    - alertContactId: 8307662
      threshold: 0                 # minutes (paid plans)
      recurrence: 0                # minutes (paid plans)
  # --- HTTP/KEYWORD/API request + auth ---
  http:
    method: POST                   # HEAD|GET|POST|PUT|PATCH|DELETE|OPTIONS|QUERY
    authType: BEARER               # NONE|HTTP_BASIC|DIGEST|BEARER
    username: svc                  # HTTP_BASIC/DIGEST
    passwordSecretRef:             # password for BASIC/DIGEST, token for BEARER
      name: endpoint-auth
      key: token
    customHeaders:
      X-Env: prod
    successHttpResponseCodes: ["2xx", "3xx"]
    postValueData: '{"ping":true}'
    postValueType: RAW_JSON        # KEY_VALUE|RAW_JSON
    followRedirections: true
    checkSSLErrors: true
    sslExpirationReminder: true
    domainExpirationReminder: true
  # --- nested per-type config (HTTP/KEYWORD/API/PING/PORT) ---
  config:
    ipVersion: ipv4Only            # ipv4Only|ipv6Only
    sslExpirationPeriodDays: [7, 30]
    applicationErrorRetries: 3     # 0–3
  # --- metadata/stats collection (opt-in) ---
  collectStats: true               # poll uptime/response-time into .status
  statsRefreshInterval: 15m
```

#### Per-type fields

Swap `type` and add the block(s) for that type (everything under *common* above still applies):

```yaml
# KEYWORD — also set:
  keyword: { value: "OK", alertWhen: ALERT_EXISTS, caseSensitive: false }  # alertWhen: ALERT_EXISTS|ALERT_NOT_EXISTS

# PING  — type: PING;  url: host-or-ip;  config: { ipVersion: ipv4Only }
# PORT  — type: PORT;  url: host;  port: 443;  portAlertCondition: OPEN   # OPEN|CLOSED
# UDP   — type: UDP;   url: host;  port: 1234
  config: { udp: { payload: "ping", packetLossThreshold: 20 } }
# HEARTBEAT — type: HEARTBEAT (no url);  gracePeriod: 300   # ping URL appears in .status.heartbeatUrl
# DNS   — type: DNS;   url: dns-server
  config: { dnsRecords: { A: ["1.2.3.4"], CNAME: ["example.com."] } }     # A,AAAA,CNAME,MX,NS,TXT,SRV,PTR,SOA,SPF,DNSKEY,DS,NSEC,NSEC3
# API   — type: API;   url: https://api.example.com
  config:
    apiAssertions:
      logic: AND                   # AND|OR
      checks:
        - { property: status_code, comparison: equals, target: 200 }
# VISUAL_COMPARISON — type: VISUAL_COMPARISON;  url: https://example.com
  config:
    visualComparison:
      sensitivityThreshold: 10     # 0–100
      viewport: desktop            # desktop|mobile
      areaCoordinates: { percentX: 0, percentY: 0, percentWidth: 100, percentHeight: 50 }
# THIRD_PARTY_SERVICE — type: THIRD_PARTY_SERVICE (no url)
  config:
    thirdPartyService:
      providerId: 42
      trackedComponentIds: [1, 2]
      notify: { severity: all, incrementalUpdates: false }  # severity: all|major|none
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
VERSION=0.1.5 # x-release-please-version
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

Automated with [release-please](https://github.com/googleapis/release-please-action):

1. Land [Conventional Commits](https://www.conventionalcommits.org) on `main`
   (`feat:` → minor, `fix:`/others → patch, `feat!:` → major).
2. **Merge the "chore(main): release X.Y.Z" PR** release-please keeps open (it bumps
   Chart.yaml/README and writes the changelog). Merging tags `vX.Y.Z`.
3. The tag triggers [`release.yml`](.github/workflows/release.yml), which builds,
   **cosign-signs**, and publishes the multi-arch image + Helm chart (chart and image
   versions always match).

**One-time setup:** release-please needs its own token to trigger `release.yml` (GitHub
won't fire workflows from a `GITHUB_TOKEN`-created tag). Add a fine-grained PAT — this repo
only, **Contents** + **Pull requests: Read and write** — as the Actions secret
**`RELEASE_PLEASE_TOKEN`**. Without it the release PR still works, but you push the tag yourself.

**Manual release** (bypass release-please): `hack/release.sh X.Y.Z --push` bumps every
version-bearing file, commits, tags, and pushes.



## License

Apache License 2.0 — see [LICENSE](LICENSE).
