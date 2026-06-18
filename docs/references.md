# Reference architectures

Operators and proxies studied while designing Iris, and what each contributed. Public clones
were copied under `tmp/refs/` (gitignored) for analysis. philprime repos were read in place.

| Repo                                        | Type               | What Iris took from it                                                                                                                                                                                                                                                                                                |
| ------------------------------------------- | ------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `kubernetes/ingress-nginx`                  | Ingress controller | The watch→render→validate→**graceful reload** loop, where pod-churn must not trigger a reload. Raw-TCP is exposed via a front Service. Also the Helm chart layout and admission webhook.                                                                                                                              |
| `traefik/traefik`                           | Proxy              | TCP **entrypoints are static config** (the port-25 friction), which is why Iris prefers a LoadBalancer Service over ingress for raw SMTP.                                                                                                                                                                             |
| `kula-app/gha-runner-autoscaler-controller` | Go controller      | The house template: `cmd`/`internal` layout, `go.mod` `tool` directive, `make analyze/format/dev`, dprint, the five workflows, distroless image, GHCR multi-arch publish                                                                                                                                              |
| `kula-app/ship`                             | CLI                | Release and distribution: reusable `workflow_call`, multi-arch GHCR, version via `-ldflags`, `KULA_RELEASE_BOT`, checksums                                                                                                                                                                                            |
| `cert-manager/cert-manager`                 | Operator           | The thin `main`→cobra→options→`Run` flow, kubebuilder markers, an integration-test framework (apiserver + webhook), and Helm webhook/RBAC templating.                                                                                                                                                                 |
| `cloudnative-pg/cloudnative-pg`             | Workload operator  | **Idempotent child-resource reconcile**, owner refs + ownership check, status patch with optimistic lock + retry, `meta.SetStatusCondition`, finalizers, CEL/RBAC markers, manager leader-election settings                                                                                                           |
| `fluxcd/source-controller`                  | Modern kubebuilder | The **status/conditions + patch-on-defer** reconcile structure, sub-reconciler chaining, terminal-vs-retryable error classification, and pinned-tool Makefile. Also the manager metrics server (`:8080`) + `HealthProbeBindAddress` and custom collectors via `controller-runtime/pkg/metrics.Registry.MustRegister`. |
| `kula-app/go-health`                        | Health library     | Data-plane `/livez` `/readyz` `/healthz` (RFC health+json), and the `RegisterReadyCheck` (traffic-gating) vs `RegisterCheck` (informational) split that keeps a flaky downstream from draining the relay.                                                                                                             |

## Decisions locked from this survey

- **Single Go module** for v1, not flux's split `api/` submodule. (Revisit only if external
  tools need to import the types without controller-runtime. The relay imports them in-repo.)
- **Webhook served by the controller manager** (single binary). No separate webhook binary or
  cainjector, because cert-manager's split is for independent scaling and that is overkill here.
  Webhook serving certs are provisioned by **cert-manager** (a `Certificate` + CA injection) or
  Helm-generated.
- **Pinned tooling via the `go.mod` `tool` directive** (controller-gen, setup-envtest, …) is the
  philprime house style. It is equivalent to flux's `go-install-tool` macro but matches the other
  repos.
- **Dependency-light reconcile**, adopting source-controller's patch-on-defer _pattern_ using
  apimachinery `meta` + controller-runtime optimistic-lock patches. Do **not** depend on
  `fluxcd/pkg/runtime`.
- **Stay minimal** by skipping the cert-manager-scale extras until justified: no versioned
  config-file API (flags/env only), no makefile-modules, no multi-module test layout, no
  acmesolver/startupapicheck-style helper binaries.

See [kubernetes.md](kubernetes.md#reconcile--status-patterns) for how these land in the
controller design.
