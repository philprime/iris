# Reference architectures

Operators and proxies studied while designing Iris, and what each contributed. Public clones
were copied under `tmp/refs/` (gitignored) for analysis; kula repos were read in place.

| Repo | Type | What Iris took from it |
| --- | --- | --- |
| `kubernetes/ingress-nginx` | Ingress controller | watch→render→validate→**graceful reload** loop; pod-churn must not reload; raw-TCP exposed via a front Service; Helm chart layout; admission webhook |
| `traefik/traefik` | Proxy | TCP **entrypoints are static config** (the port-25 friction) → prefer a LoadBalancer Service over ingress for raw SMTP |
| `kula-app/gha-runner-autoscaler-controller` | Go controller | The house template: `cmd`/`internal` layout, `go.mod` `tool` directive, `make analyze/format/dev`, dprint, the five workflows, distroless image, GHCR multi-arch publish |
| `kula-app/ship` | CLI | Release/distribution: reusable `workflow_call`, multi-arch GHCR, version via `-ldflags`, `KULA_RELEASE_BOT`, checksums |
| `cert-manager/cert-manager` | Operator | Thin `main`→cobra→options→`Run`; kubebuilder markers; integration-test framework (apiserver + webhook); Helm webhook/RBAC templating |
| `cloudnative-pg/cloudnative-pg` | Workload operator | **Idempotent child-resource reconcile**, owner refs + ownership check, status patch with optimistic lock + retry, `meta.SetStatusCondition`, finalizers, CEL/RBAC markers, manager leader-election settings |
| `fluxcd/source-controller` | Modern kubebuilder | **Status/conditions + patch-on-defer** reconcile structure, sub-reconciler chaining, terminal-vs-retryable error classification, pinned-tool Makefile; manager metrics server (`:8080`) + `HealthProbeBindAddress`, custom collectors via `controller-runtime/pkg/metrics.Registry.MustRegister` |
| `kula-app/go-health` | Health library | Data-plane `/livez` `/readyz` `/healthz` (RFC health+json); `RegisterReadyCheck` (traffic-gating) vs `RegisterCheck` (informational) split that keeps a flaky downstream from draining the relay |
| `kula-app/bifrost`, `kula-app/app-store-metadata-api` (`asm-relay`) | kula HTTP services | The house **Sentry** pattern: `sentry-go` + `sentry-go/slog`, `MultiHandler` fan-out (terminal + Sentry), `defer sentry.Flush`, `BeforeSend`/`TracesSampler` to drop probe noise, `iris_*`-style metric naming, `<svc>@<version>:<sha>` release via ldflags |

## Decisions locked from this survey

- **Single Go module** for v1 — not flux's split `api/` submodule. (Revisit only if external
  tools need to import the types without controller-runtime; the relay imports them in-repo.)
- **Webhook served by the controller manager** (single binary). No separate webhook binary or
  cainjector (cert-manager's split is for independent scaling — overkill here). Webhook serving
  certs are provisioned by **cert-manager** (a `Certificate` + CA injection) or Helm-generated.
- **Pinned tooling via the `go.mod` `tool` directive** (controller-gen, setup-envtest, …) — the
  kula house style; equivalent to flux's `go-install-tool` macro but matches the other repos.
- **Dependency-light reconcile** — adopt source-controller's patch-on-defer *pattern* using
  apimachinery `meta` + controller-runtime optimistic-lock patches; do **not** depend on
  `fluxcd/pkg/runtime`.
- **Stay minimal** — skip the cert-manager-scale extras until justified: no versioned
  config-file API (flags/env only), no makefile-modules, no multi-module test layout, no
  acmesolver/startupapicheck-style helper binaries.

See [kubernetes.md](kubernetes.md#reconcile--status-patterns) for how these land in the
controller design.
