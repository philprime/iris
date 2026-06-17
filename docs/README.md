# Iris

Iris is a Kubernetes controller that provides a **single public point of entry for inbound
SMTP** into a cluster and routes/transforms each message to in-cluster services. It replaces a
previous hand-rolled Postfix relay with a declarative, operator-managed system.

A replicated **Postfix ingress** terminates public SMTP. The **Iris controller** watches
`Relay` custom resources, compiles them into Postfix routing maps, and reconciles one
**relay pod** per `Relay`. Each relay filters inbound mail, transforms it (canonical JSON +
optional Jsonnet), and **fans it out** to one or more destinations — an HTTP REST endpoint or
a downstream SMTP server.

```
                          MX → public IP
Public Internet ──:25/587/465──▶  [ Postfix Ingress ]   (N replicas, LoadBalancer Service)
                                       │  in-container reloader mounts ConfigMap,
                                       │  routes per recipient via transport maps
                                       ▼  smtp: to Service DNS
                                  [ Relay Pod ]  ──┬─ HTTP POST ─▶ REST API
                                                   └─ smtp: ─────▶ SMTP endpoint
                                       ▲
                          (1 Deployment+Service per Relay CR)
                                       ▲   reconciled by
                              [ Iris Controller ]  ◀── watches Relay CRs (leader-elected, HA)
                                       └── renders Postfix maps → ConfigMap
```

- **Project:** `github.com/kula-app/iris`
- **API group:** `iris.kula.app/v1alpha1`

## Goals

- Single, stable public SMTP entrypoint per cluster (fixed port 25, plus 587/465).
- Declarative `Relay` CRD: recipient routing + inbound filtering + transform + fan-out delivery.
- Stateless data plane; lean on Postfix for the hard MTA concerns (TLS, queue, retry, bounce).
- Match public controller standards (kubebuilder scaffolding, Kubernetes API conventions).
- Controller + relay implemented as Go in this repository.

## Non-goals (v1)

- Relay→relay **chaining orchestration**. Chaining is a manual pattern: point a relay's `smtp`
  destination at another relay's Service. The controller/chart do not automate it.
- **Content-based routing** (selecting a subset of destinations per message). v1 fan-out is
  broadcast to all destinations.
- Relay-owned durable queue. Retries are delegated to the Postfix queue.
- Outbound/relay-for-sending mail. Iris is inbound-only.

## Documentation map

**Design**

| Doc | Covers |
| --- | --- |
| [architecture.md](architecture.md) | Components, data flow, public exposure, key properties |
| [kubernetes.md](kubernetes.md) | `Relay` CRD reference, controller/reconcilers, conflict resolution, status, RBAC, webhook, Helm chart |
| [relay.md](relay.md) | Data plane: session pipeline, filters/scoring, transform, delivery contract, config format |
| [roadmap.md](roadmap.md) | Open questions & future work |

**Contributing**

| Doc | Covers |
| --- | --- |
| [development.md](development.md) | Prerequisites, local setup, running with air/kind, debugging |
| [tooling.md](tooling.md) | Makefile targets, codegen, linters, formatting, pre-commit, renovate |
| [testing.md](testing.md) | Unit, envtest, kind e2e, conventions |
| [ci.md](ci.md) | GitHub Actions workflows |
| [distribution.md](distribution.md) | Images, Helm chart, versioning, release process |
| [conventions.md](conventions.md) | Repo layout, Go/kula coding conventions, commits |
