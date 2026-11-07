# UptimeRobot Operator — Architecture

The operator creates and deletes [UptimeRobot](https://uptimerobot.com/api/v3/)
monitors for services exposed in a Kubernetes cluster. It is built with
[kubebuilder](https://book.kubebuilder.io/) / controller-runtime and talks to the
UptimeRobot **v3** REST API.

## Components

```mermaid
flowchart TB
  subgraph cluster["Kubernetes cluster"]
    hr["HTTPRoute\n(annotated)"]
    ing["Ingress\n(annotated)"]
    mon["Monitor CR\nmonitoring.homelab.io/v1alpha1"]
    sec["Secret\n(UptimeRobot API token)"]

    subgraph op["uptimerobot-operator (manager)"]
      direction TB
      httpC["HTTPRouteReconciler"]
      ingC["IngressReconciler"]
      res["resolver\n(strategy per source kind)"]
      monC["MonitorReconciler\n(+ finalizer)"]
      cl["uptimerobot.Client\n(interface)"]
      key["apikey.FileSource\n(mounted, mtime-cached)"]
    end
  end
  ur["UptimeRobot API v3\napi.uptimerobot.com/v3"]

  hr -- "watch + annotation" --> httpC
  ing -- "watch + annotation" --> ingC
  httpC --> res
  ingC --> res
  httpC -- "CreateOrUpdate\n(+ controller ownerRef)" --> mon
  ingC -- "CreateOrUpdate\n(+ controller ownerRef)" --> mon
  mon -- "reconcile" --> monC
  sec -. "mounted file" .-> key
  key --> cl
  monC --> cl
  cl -- "POST/GET/PATCH/DELETE\npause/start/reset/stats" --> ur

  hr -. "delete ⇒ GC deletes owned" .-> mon
  mon -. "finalizer" .-> monC
  monC -. "DELETE /monitors/{id}" .-> ur
```

### Two layers, one source of truth

The **`Monitor` CR is the single source of truth**. There are two layers:

1. **Source watchers** (`HTTPRouteReconciler`, `IngressReconciler`) translate an
   annotated source object into one `Monitor` CR per hostname, each with a
   *controller* `OwnerReference` back to the source. They never call UptimeRobot.
   They also **prune** generated monitors when the annotation is removed or a
   hostname disappears.
2. **`MonitorReconciler`** is the only component that talks to UptimeRobot. It
   owns the create / adopt / update / pause / reset / delete lifecycle and uses a
   finalizer so the external monitor is removed before the CR leaves etcd.

This keeps the integration simple (one API caller), lets users create `Monitor`
CRs directly for full control, and makes deletion cascade reliably.

## Create flow (annotation-driven)

```mermaid
sequenceDiagram
  autonumber
  participant U as User
  participant K as kube-apiserver
  participant S as Source watcher
  participant M as MonitorReconciler
  participant R as UptimeRobot API

  U->>K: apply HTTPRoute (monitoring.homelab.io/enabled=true)
  K-->>S: reconcile HTTPRoute
  S->>S: resolve hostnames, build URL(s)
  S->>K: CreateOrUpdate Monitor CR (ownerRef → HTTPRoute)
  K-->>M: reconcile Monitor
  M->>K: add finalizer
  M->>R: POST /monitors (type-specific body)
  alt duplicate (009-011)
    M->>R: GET /monitors (adopt by url + name)
  end
  M->>K: status.monitorID, conditions=Ready
  opt collectStats
    M->>R: GET /monitors/{id}/stats/uptime + response-time
    M->>K: status.uptime / responseTime
  end
```

## Delete flow (cascade)

```mermaid
sequenceDiagram
  autonumber
  participant U as User
  participant K as kube-apiserver
  participant G as GC
  participant M as MonitorReconciler
  participant R as UptimeRobot API

  U->>K: delete HTTPRoute
  K->>G: owner removed
  G->>K: delete owned Monitor CR (sets deletionTimestamp)
  K-->>M: reconcile Monitor (being deleted)
  M->>R: DELETE /monitors/{id}
  alt not found (000-004)
    Note over M: treat as already deleted
  end
  M->>K: remove finalizer ⇒ Monitor gone
```

Deleting a `Monitor` CR directly follows the same finalizer path.

## API client resilience

The `uptimerobot` package is transport-only (no controller-runtime imports) and
handles the v3 contract:

- **Auth**: `Authorization: Bearer <token>`, token read from a mounted Secret file.
- **Rate limiting**: a client-side token-bucket limiter (default 10 req/min to
  respect the Free plan) serializes calls so the operator never self-inflicts a 429.
- **Error handling**: responses are parsed into a typed `APIError` carrying the
  stable `code`; callers branch on `code` (falling back to HTTP status for unknown
  codes) via `IsNotFound` / `IsDuplicate` / `IsRateLimited` / `IsTerminal`.
- **429**: surfaced as a rate-limit error carrying `Retry-After`; the reconciler
  requeues after that delay instead of hot-looping.
- **Idempotency**: a create returning `009-011` (duplicate) adopts the existing
  monitor by URL + friendly name; a delete returning `000-004` (not found) is
  treated as success so an out-of-band deletion cannot wedge the finalizer.
- **Type-discriminated body**: `CreateMonitor`/`UpdateMonitor` assemble the exact
  `oneOf` body for the monitor's type (HTTP/KEYWORD/PING/PORT/HEARTBEAT/DNS/API/
  UDP/VISUAL_COMPARISON/THIRD_PARTY_SERVICE); `type` is never sent on update
  (it is immutable server-side).

## Graceful degradation

The HTTPRoute watcher is only started when the Gateway API CRD is present (probed
via the RESTMapper at startup), so the operator runs on clusters without Gateway
API installed. Installing Gateway API later requires an operator restart.
