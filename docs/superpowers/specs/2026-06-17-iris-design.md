# Iris — Design (2026-06-17)

This design has been split into living documentation under [`docs/`](../../README.md):

- [architecture.md](../../architecture.md) — components, data flow, public exposure, key properties
- [kubernetes.md](../../kubernetes.md) — `Relay` CRD reference, controller/reconcilers, conflict resolution, status, RBAC, webhook, Helm chart
- [relay.md](../../relay.md) — data plane: session pipeline, filters/scoring, transform, delivery contract, config format
- [conventions.md](../../conventions.md) — repo layout, Go/kula conventions, testing, commits
- [roadmap.md](../../roadmap.md) — open questions & future work

The living docs are the source of truth; this file marks the original design date. The full
original spec text is preserved in git history (commit `5e38544`).
