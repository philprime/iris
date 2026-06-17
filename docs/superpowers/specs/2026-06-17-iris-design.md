# Iris — SMTP Ingress Controller — Design

- **Status:** Approved (design phase)
- **Date:** 2026-06-17
- **Project:** Iris (`github.com/kula-app/iris`)
- **API group:** `iris.kula.app/v1alpha1`

## 1. Summary

Iris is a Kubernetes controller that provides a **single public point of entry for
inbound SMTP** into a cluster and routes/transforms each message to in-cluster
services. It replaces the previous hand-rolled Postfix relay with a declarative,
operator-managed system.

A replicated **Postfix ingress** terminates public SMTP. The **Iris controller**
watches `Relay` custom resources, compiles them into Postfix routing maps, and
reconciles one **relay pod** per `Relay`. Each relay filters inbound mail, transforms
it (canonical JSON + optional Jsonnet), and **fans it out** to one or more
destinations — an HTTP REST endpoint or a downstream SMTP server.

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

## 2. Goals / Non-goals

**Goals**
- Single, stable public SMTP entrypoint per cluster (fixed port 25, plus 587/465).
- Declarative `Relay` CRD: recipient routing + inbound filtering + transform + fan-out delivery.
- Stateless data plane; lean on Postfix for the hard MTA concerns (TLS, queue, retry, bounce).
- Match public controller standards (kubebuilder scaffolding, Kubernetes API conventions).
- Controller + relay implemented as Go in this repository.

**Non-goals (v1)**
- Relay→relay **chaining orchestration**. Chaining is a manual pattern: point a relay's
  `smtp` destination at another relay's Service. The controller/chart do not automate it.
- **Content-based routing** (selecting a subset of destinations per message). v1 fan-out is
  broadcast to all destinations. Routing is a future `match:` field per destination.
- Relay-owned durable queue. Retries are delegated to the Postfix queue.
- Outbound/relay-for-sending mail. Iris is inbound-only.

## 3. Architecture & Components

1. **Iris controller (control plane)** — Go + controller-runtime. Leader-elected,
   replicated for availability (default 2 replicas; only the leader reconciles). Two
   reconcilers under one manager (see §6). Owns CRD status and a validating webhook.

2. **Postfix ingress (data plane front door)** — `boky/postfix` Deployment (N replicas)
   behind a LoadBalancer Service on 25/587/465. Provides TLS (opportunistic STARTTLS),
   queueing, retry/backoff, and bounces. Configured via the chart (a singleton per install,
   like ingress-nginx's controller) — **not** a CRD. Its routing config is generated from
   the `Relay` CRs.

3. **Postfix image + in-container reloader** — a custom image `FROM boky/postfix` with a
   small reloader process (`cmd/reloader`) that starts Postfix, watches the mounted routing
   ConfigMap (inotify), and runs `postmap` + `postfix reload` on change. A separate sidecar
   is **not** used (it cannot cleanly signal another container's Postfix master).

4. **Relay pod (data plane transformer)** — single reusable Go image (`emersion/go-smtp`
   server). One Deployment+Service per `Relay`. Receives SMTP from Postfix, applies filters,
   transforms, and delivers. Configured entirely from a mounted file + mounted Secrets;
   **needs no Kubernetes API access**.

5. **Helm chart** — installs controller + CRDs + RBAC + Postfix tier + LoadBalancer Service +
   webhook + ServiceMonitor + PDB, mirroring the ingress-nginx chart layout.

**Key properties**
- Relay-pod churn never reloads Postfix: transport targets are **Service DNS names**, so
  kube-proxy handles pod IP changes. Postfix only reloads when a route (`Relay`) changes.
- Raw TCP ports (25/587/465) are declared once on the LoadBalancer Service; everything below
  is dynamic.
- The only stateful, hard-MTA concerns live in Postfix. Everything Iris owns is stateless.

## 4. The `Relay` CRD

Group/Version/Kind: **`iris.kula.app/v1alpha1`, `Relay`** (namespaced).
One `Relay` = one transformer Deployment+Service + a set of routes compiled into Postfix.

```yaml
apiVersion: iris.kula.app/v1alpha1
kind: Relay
metadata:
  name: appstore-invites
  namespace: example
spec:
  # 1. WHAT MAIL THIS RELAY CLAIMS → compiled into Postfix transport + relay_recipient_maps
  routes:
    - address: invites@invite.example.com   # exact address (wins over domain)
    - domain:  invite.example.com           # any local-part on the domain

  # 2. INBOUND FILTERING → relay rejects with SMTP 5xx before transforming (optional)
  filters:
    maxMessageBytes: 26214400               # 25 MiB
    allowedSenderDomains: ["email.apple.com"]
    requireDKIM: ["email.apple.com"]        # DKIM d= must match one of these
    minScore: 2                             # accept if score >= minScore
    scoreSignals: [fromDomain, messageIdDomain, dkimDomain, authResults, bodyLinkDomain]

  # 3. DELIVERY → fan-out to ALL destinations (broadcast)
  idempotency: messageId                    # messageId (default) | sha256 — key sent to every destination
  destinations:
    - name: webhook
      required: true                        # failure → SMTP 4xx → Postfix retries the message
      http:
        url: https://service.internal/inbound
        method: POST                        # default POST
        payloadFormat: json                 # json (canonical envelope, default) | raw (message/rfc822)
        authSecretRef: { name: webhook, key: token }   # → Authorization header
        transform:                          # OPTIONAL Jsonnet remap (Ory-Kratos style)
          jsonnetConfigMapRef: { name: mapping, key: map.jsonnet }
    - name: archive
      required: false                       # best-effort; failure logged + metered, no upstream retry
      smtp:
        host: archive.internal
        port: 1025
        startTLS: false
        # authSecretRef: { name: ..., key: ... }

  # 4. RELAY POD SHAPE (optional; sensible defaults)
  deployment:
    replicas: 1
    resources:
      requests: { cpu: 50m,  memory: 64Mi }
      limits:   { cpu: 250m, memory: 128Mi }

status:
  conditions:                               # Kstatus-style
    - { type: Ready,      status: "True", observedGeneration: 4, ... }
    - { type: Programmed, status: "True", ... }   # compiled into the Postfix ingress
    - { type: Conflict,   status: "False", ... }
  observedGeneration: 4
  claimedRoutes: ["invites@invite.example.com", "@invite.example.com"]
  serviceRef: { name: relay-appstore-invites }
```

### 4.1 Field semantics

- **`routes`** — each entry is `address:` (exact) **or** `domain:` (wildcard local-part).
  Both supported; an exact address wins over a `@domain` route (Postfix transport semantics).
- **`filters`** (optional) — declarative inbound validation. The relay rejects with SMTP 5xx
  before forwarding when a hard rule fails (size, sender domain, DKIM) or when the heuristic
  **score** (sum of matched `scoreSignals`) is below `minScore`. Signals are named, reusable
  checks: `fromDomain`, `messageIdDomain`, `dkimDomain`, `authResults`, `bodyLinkDomain`.
- **`destinations`** — an **array**; every accepted message is delivered to **all** of them
  (fan-out). Each is a discriminated union of exactly one of `http` / `smtp`, enforced by CEL
  `x-kubernetes-validations` + the validating webhook. Each destination has its **own**
  transform/payload format (fan-out to different systems usually wants different shapes).
- **`required`** (per destination, default `true`) — gates retry. A failure of a `required`
  destination returns SMTP 4xx so Postfix redelivers; `required: false` failures are
  best-effort (logged + metered, no retry).
- **`idempotency`** — selects the stable key (`Message-ID` header or SHA-256 of raw bytes)
  sent to every destination so downstreams can dedup.
- **Secrets** are always `secretRef` (same namespace as the `Relay`), never inline.

### 4.2 Delivery contract

The pipeline is **at-least-once** (Postfix queue + retry). Fan-out is **not atomic**: if one
of several destinations fails and triggers a retry, destinations that already succeeded
receive the message again. Mitigations (standard for any queue-backed email system):
1. Every delivery carries an **idempotency key** (`Idempotency-Key` HTTP header and/or in the
   JSON envelope) so downstreams dedup.
2. **`required: false`** marks best-effort destinations whose failure does not trigger a retry.

This contract — *at-least-once, idempotency-key-deduped, `required` gates retry* — is part of
the public API documentation.

### 4.3 Canonical JSON envelope

What `payloadFormat: json` emits and what a Jsonnet `transform` receives as input. Fixed,
documented, versioned schema:

```json
{
  "version": "v1",
  "idempotencyKey": "<message-id-or-sha256>",
  "envelope": { "mailFrom": "...", "rcptTo": ["..."] },
  "headers": { "Subject": "...", "From": "...", "...": "..." },
  "from": "...", "to": ["..."], "subject": "...",
  "text": "...", "html": "...",
  "attachments": [ { "filename": "...", "contentType": "...", "bytesBase64": "..." } ],
  "raw": "<optional full RFC822, base64>"
}
```

### 4.4 Conflict resolution

Route keys (exact addresses and `@domain`s) must be unique cluster-wide. On collision,
**first-writer-wins**, ordered deterministically by `(creationTimestamp, namespace, name)`.
The earliest claimant compiles into Postfix; later claimants get `Conflict=True`, are excluded
from routing, and do not break the ingress. The validating webhook rejects obvious collisions
at apply-time; the controller is the backstop for races.

## 5. Public exposure

Modeled as an exposure `mode` so it can grow, defaulting to the portable option:
- **`loadBalancer`** (default, v1) — controller/chart create a Service `type=LoadBalancer` on
  25/587/465 in front of Postfix. Cloud LB gives a stable public IP for MX records. Port 25 is
  raw TCP; this avoids HTTP-first ingress controllers entirely.
- **`traefik`** (documented, later phase) — emit `IngressRouteTCP`. Requires a dedicated
  Traefik **entrypoint** on port 25 in Traefik's *static* config (cannot be injected by a CRD),
  which is the known friction point.
- **`none`** — operator wires their own front Service.

TLS: opportunistic STARTTLS on inbound; certificates via cert-manager.

## 6. Controller internals

One manager, two reconcilers, leader-elected (HA; only the leader reconciles):

1. **RelayReconciler** — per `Relay`: reconciles a **Deployment + Service** (owner-referenced
   for garbage collection) and renders the relay's config (routes/filters/destinations) into a
   per-relay **ConfigMap** that the relay pod mounts. Wires `secretRef`s as volumes/env into the
   pod (same namespace → allowed).
2. **ConfigReconciler** — watches **all** `Relay`s, compiles the **global** Postfix maps
   (`transport`, `relay_domains`, `relay_recipient_maps`), and writes the Postfix **ConfigMap**.
   Owns conflict resolution (§4.4). Render → diff → write only on change.

**Standard-controller hygiene**
- `status.conditions` (`Ready`, `Programmed`, `Conflict`) per Kstatus; `observedGeneration`;
  CRD printer columns.
- A **finalizer** on `Relay` releases its routes from the aggregate before deletion.
- CEL `x-kubernetes-validations` on the CRD + a **validating webhook** (destination union,
  conflict pre-check, Jsonnet parse check).
- controller-runtime **metrics**; leader election as a chart value (default on).
- Watches: `Relay` (primary), owned `Deployment`/`Service`, and the Postfix ConfigMap.

## 7. Relay internals

`emersion/go-smtp` server; config from the mounted file + mounted Secrets:
- **Session:** `MAIL FROM` → sender filter; `RCPT TO` → route match; `DATA` → size cap →
  MIME parse → filter score (reject 5xx below `minScore`) → compute **idempotency key** once →
  **fan out** to all destinations (per-destination transform + deliver) → return `250` if all
  `required` succeed, `4xx` if any `required` fails (Postfix retries); best-effort failures are
  logged + metered.
- **Transform:** MIME → canonical envelope (§4.3); optional **Jsonnet** (`google/go-jsonnet`)
  per destination.
- **Deliver:** HTTP (timeout, `Idempotency-Key` header, secret-based auth) or downstream SMTP
  (optional STARTTLS/auth).
- **Observability:** `slog` (with `…Context` variants) + per-destination success/failure/score
  metrics. No Kubernetes API access.

## 8. Repository layout

kubebuilder conventions + kula's `cmd`/`internal` layout. Module `github.com/kula-app/iris`,
Go 1.26, Makefile-driven.

```
iris/
├── PROJECT  Makefile  go.mod  AGENTS.md  CLAUDE.md
├── api/v1alpha1/            # Relay types + kubebuilder markers + zz_generated.deepcopy.go
├── cmd/
│   ├── controller/{main,run}.go   # manager: reconcilers + webhook + leader election
│   ├── relay/{main,run}.go        # go-smtp data-plane server
│   └── reloader/{main,run}.go     # file-watch → postmap + postfix reload (baked into postfix image)
├── internal/
│   ├── controller/         # relay_controller.go, config_controller.go
│   ├── postfix/            # render transport / relay_domains / relay_recipient_maps (+ tests)
│   ├── relay/             # server, session, filter, transform, deliver_http, deliver_smtp, envelope
│   ├── config/            # env config w/ validator tags (both binaries)
│   └── webhook/           # validating webhook for Relay
├── config/                # kubebuilder kustomize bases: crd, rbac, manager, webhook
├── chart/iris/            # Helm chart (distribution; mirrors ingress-nginx)
│   ├── Chart.yaml  values.yaml  crds/  templates/
├── images/{controller,relay,postfix}/Dockerfile
├── docs/superpowers/specs/
└── test/e2e/              # envtest + kind
```

**Three images:** `controller`, `relay`, `postfix` (boky/postfix + baked `reloader`).

## 9. Conventions (from kula standards)

- Go 1.26, Makefile targets only (never raw `go`); `make format` before commit.
- Env-only config with `validator` tags, prefix `IRIS_<COMPONENT>_<SETTING>`; validate at startup.
- `slog` with `…Context` variants; structured fields; no `fmt.Print` in services.
- Wrap errors with `%w`; lowercase error strings (ST1005); exported sentinel errors.
- Table-driven tests; integration tests behind `//go:build integration`; Gherkin header comments
  on integration tests.
- Conventional Commits; no AI/co-author attribution.

## 10. Relay config file format

The relay's mounted config is rendered as **YAML via `sigs.k8s.io/yaml`**, reusing the
`api/v1alpha1` structs as the single source of truth (YAML→JSON→struct through json tags). The
file carries a `version` field for forward compatibility. ConfigMaps stay debuggable with
`kubectl get cm -o yaml`. (Postfix map files use Postfix's own native format, not YAML.)

## 11. Open questions / future work

- Content-based routing (`match:` per destination) — selecting a subset of destinations per
  message, beyond v1 broadcast fan-out.
- `traefik` and `none` exposure modes (v1 ships `loadBalancer`).
- Per-relay HPA and autoscaling signals.
- Bounce/DSN handling policy surfaced through the CRD (currently delegated to Postfix defaults).
- Multi-tenancy/RBAC story for who may claim which domains (beyond first-writer-wins).
```
