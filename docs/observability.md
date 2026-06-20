# Observability

Iris exposes the three standard pillars on every binary: **health/readiness probes**,
**Prometheus metrics**, and **error/trace reporting via Sentry**. The shape differs by component
because the control plane is a controller-runtime manager while the data plane is plain
`net/http` + `go-smtp`:

| Component      | Health                                                                                        | Metrics                                                | Errors        |
| -------------- | --------------------------------------------------------------------------------------------- | ------------------------------------------------------ | ------------- |
| **controller** | controller-runtime manager probes (`AddHealthzCheck`/`AddReadyzCheck`)                        | controller-runtime metrics server (`metrics.Registry`) | Sentry + slog |
| **relay**      | [`kula-app/go-health`](https://github.com/kula-app/go-health) (`/livez` `/readyz` `/healthz`) | `promhttp` on the admin server                         | Sentry + slog |
| **reloader**   | `go-health` (`/livez` `/readyz`)                                                              | `promhttp` on the admin server                         | Sentry + slog |

This split is deliberate: the controller already gets a metrics server and probe endpoints from
the manager, so layering `go-health` on top would be redundant. The data-plane binaries have no
manager, so they use `go-health`, the same library every other philprime HTTP service uses.

## Listen addresses

Each binary serves its observability surface on fixed, separately-bound ports (kubebuilder
defaults for the controller, a single admin server for the data plane). Each bind address is
overridable via the env var below, defined in [`internal/config`](../internal/config/config.go):

| Binary     | Env var                        | Default | Serves                                      |
| ---------- | ------------------------------ | ------- | ------------------------------------------- |
| controller | `IRIS_CONTROLLER_METRICS_ADDR` | `:8080` | `/metrics`                                  |
| controller | `IRIS_CONTROLLER_HEALTH_ADDR`  | `:8081` | `/healthz`, `/readyz`                       |
| controller | `IRIS_CONTROLLER_WEBHOOK_ADDR` | `:9443` | validating webhook                          |
| relay      | `IRIS_RELAY_ADMIN_ADDR`        | `:8080` | `/metrics`, `/livez`, `/readyz`, `/healthz` |
| reloader   | `IRIS_RELOADER_ADMIN_ADDR`     | `:8080` | `/metrics`, `/livez`, `/readyz`             |

The relay's SMTP listener (25, from Postfix) is separate from its admin HTTP server. The admin
server hosts only probes + metrics and needs **no Kubernetes API access** (consistent with
[relay.md](relay.md)). Probe ports are cluster-internal, so do not expose `/metrics` or `/healthz`
publicly (see [Security](#security--cardinality)).

## Health & readiness

Probe semantics follow the Kubernetes convention `go-health` already encodes (see its README):
**liveness is process-local** (a failure restarts the pod, so it must never depend on a
downstream), **readiness gates Service endpoints** (a failure drains traffic, so only put
genuinely traffic-blocking checks here), and **healthz is the comprehensive dashboard**.

### Controller (controller-runtime manager)

The manager serves `/healthz` and `/readyz` on its `HealthProbeBindAddress`. Liveness is a
process-local ping that never touches the API server or a downstream. Readiness gates on the
webhook server being able to serve (certificate loaded, listener up).

Leader election does **not** gate readiness. A non-leader replica is healthy and ready, it is
simply idle. (controller-runtime exports `leader_election_master_status` for that signal.)

### Relay (go-health)

Readiness gates Service endpoints on a single check: the SMTP listener is bound. Destination
reachability is a `/healthz`-only check, never a readiness gate. Postfix queues and retries on a
required-destination failure (see [relay.md](relay.md#delivery-contract)), so a flaky downstream
must not drain the relay from its Service and stall all mail. The wiring lives in
[`internal/relay/health.go`](../internal/relay/health.go).

Putting destination reachability on `/healthz` rather than `/readyz` is the whole reason
`go-health` separates health and readiness checks.

### Reloader

- `/livez`: process alive (`system:time` heartbeat only).
- `/readyz`: Postfix master reachable (`postfix status`) **and** the last `postfix reload`
  succeeded. A failed reload means the ingress is serving stale routes, which should drain
  the replica.

## Metrics

Naming follows the philprime convention: an `iris_` prefix, declared as package-level
collectors in an `internal/metrics` package and registered once in `init()`. Counters end in
`_total`, durations in `_seconds` (histograms with `prometheus.DefBuckets` unless a domain range
fits better). Keep label cardinality bounded. Never label by message-id, recipient, or sender.

### Controller

v1 serves `/metrics` as **plain HTTP on `:8080`**, kept cluster-internal by a NetworkPolicy (not
controller-runtime's secure `:8443` TLS+authn serving, which is avoided for v1 to skip the cert
plumbing, and can be revisited if metrics are ever scraped across a trust boundary).

The controller registers its collectors with **controller-runtime's** registry, not the global
default one, so they serve on the manager's metrics endpoint.

controller-runtime already provides the reconcile/workqueue/client/leader-election families for
free, so **do not reimplement them**:

| Metric (provided)                                          | Use                               |
| ---------------------------------------------------------- | --------------------------------- |
| `controller_runtime_reconcile_total{controller,result}`    | reconcile throughput + error rate |
| `controller_runtime_reconcile_time_seconds{controller}`    | reconcile latency                 |
| `controller_runtime_reconcile_errors_total{controller}`    | terminal error rate               |
| `workqueue_depth` / `_adds_total` / `_retries_total{name}` | backlog & churn                   |
| `leader_election_master_status`                            | which replica is leading          |
| `rest_client_requests_total{code,method}`                  | API-server pressure               |

Iris adds only domain metrics the runtime can't know about (low cardinality, aggregated rather than
per-relay, because per-relay state belongs in CR conditions + kube-state-metrics):

| Metric (iris)                       | Type    | Labels   | Meaning                                                          |
| ----------------------------------- | ------- | -------- | ---------------------------------------------------------------- |
| `iris_relays`                       | Gauge   | `phase`  | Relays by status (`ready`/`conflict`/`programming`)              |
| `iris_route_conflicts`              | Gauge   | —        | Routes currently losing first-writer-wins                        |
| `iris_postfix_config_renders_total` | Counter | `result` | Postfix-map render→write attempts (`written`/`nochange`/`error`) |
| `iris_postfix_config_generation`    | Gauge   | —        | Generation of the last written Postfix ConfigMap                 |

### Relay

Default Prometheus registry, served via `promhttp.Handler()` on the admin server:

| Metric                                 | Type      | Labels                    | Meaning                                                                                              |
| -------------------------------------- | --------- | ------------------------- | ---------------------------------------------------------------------------------------------------- |
| `iris_relay_sessions_total`            | Counter   | `result`                  | SMTP sessions (`accepted`/`rejected`)                                                                |
| `iris_relay_messages_total`            | Counter   | `result`                  | Messages by outcome (`delivered`/`rejected_size`/`rejected_sender`/`rejected_dkim`/`rejected_score`) |
| `iris_relay_filter_score`              | Histogram | —                         | Heuristic score distribution (vs `minScore`)                                                         |
| `iris_relay_message_bytes`             | Histogram | —                         | Accepted message size                                                                                |
| `iris_relay_deliveries_total`          | Counter   | `destination,type,result` | Per-destination fan-out (`type`=`http`/`smtp`, `result`=`success`/`failure`)                         |
| `iris_relay_delivery_duration_seconds` | Histogram | `destination,type`        | Per-destination delivery latency                                                                     |
| `iris_relay_deliveries_in_flight`      | Gauge     | —                         | Concurrent in-flight deliveries                                                                      |

`destination` is the operator-chosen `destinations[].name` (bounded by the CR), so its cardinality
is safe. The `result`/`type` split lets you alert on `required`-destination failure rate, which is
what drives the SMTP 4xx → Postfix-retry path.

### Reloader

| Metric                                              | Type      | Labels   | Meaning                                         |
| --------------------------------------------------- | --------- | -------- | ----------------------------------------------- |
| `iris_postfix_reloads_total`                        | Counter   | `result` | `postfix reload` attempts (`success`/`failure`) |
| `iris_postfix_reload_duration_seconds`              | Histogram | —        | Reload latency                                  |
| `iris_postfix_config_last_reload_timestamp_seconds` | Gauge     | —        | Unix time of the last successful reload         |

> **Postfix MTA metrics** (queue depth, bounce/defer rates) are **not** in v1, because `boky/postfix`
> exposes no Prometheus endpoint. A `postfix_exporter` sidecar is a possible future addition.

### Scraping

The Helm chart ships a **ServiceMonitor** per component (see [kubernetes.md](kubernetes.md) and
[distribution.md](distribution.md)). The controller and Postfix tier are singletons with stable
Services. For relays, the `RelayReconciler` adds a named `metrics` port to each relay Service and
a single ServiceMonitor selects them all by label, so new relays are scraped without chart
changes.

## Error reporting (Sentry)

Sentry is the in-app error/trace channel, wired identically across all three binaries using the
established philprime pattern (`bifrost`): `github.com/getsentry/sentry-go` +
`github.com/getsentry/sentry-go/slog`. It is **opt-in** (`IRIS_SENTRY_ENABLED=false` by default) so
local/dev and air-gapped installs run clean. v1 posture is **errors + logs only, tracing off**
(`IRIS_SENTRY_ENABLE_TRACING=false`, `IRIS_SENTRY_TRACES_SAMPLE_RATE=0.0`). Operators opt into
performance tracing per environment. The `TracesSampler` that drops probe spans is wired regardless,
so enabling tracing later never floods the quota with health traffic.

### Wiring

Sentry is initialized as early as possible in `run()`, only when enabled, then bridged into `slog`
and flushed on shutdown. The Sentry slog handler maps `ERROR` records to Sentry issues and
`WARN`/`INFO` records to Sentry logs. The `TracesSampler` (which drops probe and metrics spans) and
`BeforeSend` (which drops expected or noisy events) are wired at init time.

The `MultiHandler` (copied from the philprime house `internal/logging`) sends every record to both the
terminal handler and the Sentry slog handler, so developers keep local visibility while production
captures issues. For the **controller**, this slog logger is bridged into controller-runtime's
`logr` via `logr.FromSlogHandler(...)` and set with `ctrl.SetLogger`, so manager and reconciler
logs flow through the same pipeline.

### What gets captured (and what must not)

Capture **unexpected** failures, never routine protocol outcomes:

- **Controller**: `sentry.CaptureException(err)` on a _terminal_ reconcile error (the same place
  `Ready=False` is set in the patch-on-defer block, see
  [kubernetes.md](kubernetes.md#reconcile--status-patterns)). Transient requeues are normal flow, so
  **do not** capture them. Tag the scope with the `Relay` namespace/name.
- **Relay**: capture transform/Jsonnet evaluation errors and recovered panics in an SMTP session.
  **Do not** capture filter rejections or `required`-destination 4xx: those are expected,
  documented outcomes already covered by metrics ([relay.md](relay.md#delivery-contract)).
- **Reloader**: capture `postfix reload` failures (a stale ingress is a real incident).

`BeforeSend` is the central kill-switch for anything noisy that slips through. `TracesSampler`
returns `0.0` for probe/metrics spans so health traffic doesn't drown the trace quota.

### Release identifier

`Release` uses the philprime unified format **`iris@<version>:<git-sha>`**, injected at build time via
`-ldflags "-X main.sentryRelease=iris@${VERSION}:${GIT_SHA}"` and defaulted through
`IRIS_SENTRY_RELEASE` (see [distribution.md](distribution.md)). This ties Sentry issues back to the
exact image, matching the `version`/`commit`/`date` ldflags already used for the binaries.

### Configuration

Sentry is configured per binary from the `IRIS_SENTRY_*` env vars below. It is disabled by default
with tracing off, so local and air-gapped installs run clean. The settings surface as Helm values
(chart-wide, with optional per-component override) and are injected as env on each Deployment.

| Variable                         | Default | Purpose                                                                            |
| -------------------------------- | ------- | ---------------------------------------------------------------------------------- |
| `IRIS_SENTRY_ENABLED`            | `false` | Master switch. When false the SDK is not initialized and nothing is sent.          |
| `IRIS_SENTRY_DSN`                | —       | Sentry project DSN. Required when enabled.                                         |
| `IRIS_SENTRY_ENVIRONMENT`        | `local` | Environment tag on every event (for example `production`).                         |
| `IRIS_SENTRY_RELEASE`            | —       | Release identifier. Defaults to the build's `iris@<version>:<git-sha>` when unset. |
| `IRIS_SENTRY_DEBUG`              | `false` | Enables the Sentry SDK's own debug logging.                                        |
| `IRIS_SENTRY_ATTACH_STACKTRACE`  | `true`  | Attaches stack traces to captured messages.                                        |
| `IRIS_SENTRY_SAMPLE_RATE`        | `1.0`   | Error event sampling rate, from 0 to 1.                                            |
| `IRIS_SENTRY_ENABLE_TRACING`     | `false` | Turns on performance tracing.                                                      |
| `IRIS_SENTRY_TRACES_SAMPLE_RATE` | `0.0`   | Trace sampling rate, from 0 to 1. Only relevant when tracing is enabled.           |

The `Sentry` struct in [`internal/config`](../internal/config/config.go) is the source of truth for
the exact defaults and validation.

## Logging

`slog` everywhere (with `…Context` variants when a `context.Context` is in scope), structured
fields only, never `fmt.Sprintf` in messages (see [conventions.md](conventions.md)). The
`MultiHandler` is the single fan-out point. Without Sentry it is just the terminal handler. The
`go-health` engine logs through the same injected logger via `core.WithLogger`, and is silent on
`pass` so the kubelet's 10s probe cadence does not flood logs.

## Security & cardinality

- Keep `/metrics` and the probe endpoints **cluster-internal**. They leak topology and `/metrics`
  is unauthenticated, so the chart ships a **NetworkPolicy** restricting scrape access to the
  monitoring namespace. (Secure `:8443` TLS serving is the fallback only if metrics ever cross a
  trust boundary.)
- `go-health` zeroes a check's `Output` on `pass` and can be wrapped to redact downstream error
  strings, relevant if `/healthz` is ever exposed beyond the cluster (see its README §Security).
- Never put unbounded values (message-id, addresses, remote IPs) in metric labels or in low-level
  Sentry tags. They belong in logs/breadcrumbs, not in time series or issue grouping keys.

## Dependencies introduced

| Module                                       | Purpose                                          |
| -------------------------------------------- | ------------------------------------------------ |
| `github.com/kula-app/go-health`              | Data-plane `/livez` `/readyz` `/healthz`         |
| `github.com/prometheus/client_golang`        | Metrics + `promhttp` (data plane)                |
| `github.com/getsentry/sentry-go` + `/slog`   | Error/trace reporting + slog bridge              |
| `sigs.k8s.io/controller-runtime/pkg/metrics` | Controller metrics registry (already transitive) |

See [references.md](references.md) for the projects these patterns were drawn from.
