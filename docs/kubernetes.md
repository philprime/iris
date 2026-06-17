# Kubernetes API & Controller

Iris follows standard controller conventions: kubebuilder scaffolding and Kubernetes API
conventions (Kstatus-style conditions, `observedGeneration`, printer columns, CEL validation,
finalizers, owner references, leader election, controller-runtime metrics).

## The `Relay` CRD

Group/Version/Kind: **`iris.kula.app/v1alpha1`, `Relay`** (namespaced). One `Relay` = one
transformer Deployment+Service + a set of routes compiled into Postfix.

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
        transform:                          # OPTIONAL Jsonnet remap
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

### Field semantics

- **`routes`** — each entry is `address:` (exact) **or** `domain:` (wildcard local-part). Both
  supported; an exact address wins over a `@domain` route (Postfix transport semantics).
- **`filters`** (optional) — declarative inbound validation. The relay rejects with SMTP 5xx
  before forwarding when a hard rule fails (size, sender domain, DKIM) or when the heuristic
  **score** is below `minScore`. See [relay.md](relay.md#filters--scoring) for signal semantics.
- **`destinations`** — an **array**; every accepted message is delivered to **all** of them
  (fan-out). Each is a discriminated union of exactly one of `http` / `smtp`, enforced by CEL
  `x-kubernetes-validations` + the validating webhook. Each destination has its **own**
  transform/payload format.
- **`required`** (per destination, default `true`) — gates retry. See
  [relay.md](relay.md#delivery-contract).
- **`idempotency`** — selects the stable key sent to every destination.
- **Secrets** are always `secretRef` (same namespace as the `Relay`), never inline.

## Conflict resolution

Route keys (exact addresses and `@domain`s) must be unique cluster-wide. On collision,
**first-writer-wins**, ordered deterministically by `(creationTimestamp, namespace, name)`. The
earliest claimant compiles into Postfix; later claimants get `Conflict=True`, are excluded from
routing, and do not break the ingress. The validating webhook rejects obvious collisions at
apply-time; the controller is the backstop for races.

## Controller internals

One manager, two reconcilers, leader-elected (HA; only the leader reconciles):

1. **RelayReconciler** — per `Relay`: reconciles a **Deployment + Service** (owner-referenced for
   garbage collection) and renders the relay's config (routes/filters/destinations) into a
   per-relay **ConfigMap** that the relay pod mounts. Wires `secretRef`s as volumes/env into the
   pod (same namespace → allowed).
2. **ConfigReconciler** — watches **all** `Relay`s, compiles the **global** Postfix maps
   (`transport`, `relay_domains`, `relay_recipient_maps`), and writes the Postfix **ConfigMap**.
   Owns conflict resolution. Render → diff → write only on change.

### Standard-controller hygiene

- `status.conditions` (`Ready`, `Programmed`, `Conflict`) per Kstatus; `observedGeneration`; CRD
  printer columns.
- A **finalizer** on `Relay` releases its routes from the aggregate before deletion.
- CEL `x-kubernetes-validations` on the CRD + a **validating webhook** (destination union,
  conflict pre-check, Jsonnet parse check).
- controller-runtime **metrics**; leader election as a chart value (default on).
- Watches: `Relay` (primary), owned `Deployment`/`Service`, and the Postfix ConfigMap.

## Reconcile & status patterns

Grounded in cloudnative-pg, source-controller, and cert-manager (see [references.md](references.md)):

- **Idempotent child resources** — for each `Deployment`/`Service`/`ConfigMap` a `Relay` owns:
  `Get` → `Create` (after `ctrl.SetControllerReference`) if absent, else compute the desired object
  and `Patch` with `client.MergeFrom(original)`; skip the write when `reflect.DeepEqual` shows no
  change. Verify ownership (`metav1.GetControllerOf`) before patching.
- **Status via patch-on-defer + optimistic lock** — capture the object at the top of `Reconcile`;
  set conditions with apimachinery `meta.SetStatusCondition`; in a deferred block patch status once
  via `client.Status().Patch(ctx, obj, client.MergeFromWithOptions(orig, client.MergeFromWithOptimisticLock{}))`
  wrapped in `retry.RetryOnConflict`; always set `observedGeneration`. This is the dependency-light
  equivalent of flux's SerialPatcher/summarize — **no `fluxcd/pkg/runtime` dependency**.
- **Conditions** — `Ready` summarizes the owned conditions (`Programmed`, `Conflict`, and a per-relay
  `DeploymentAvailable`). Negative-polarity conditions (e.g. `Conflict`) are present only when abnormal.
- **Finalizers** — `controllerutil.AddFinalizer` on first reconcile; on `DeletionTimestamp`, release
  the relay's routes from the aggregate, then `controllerutil.RemoveFinalizer` (both via `MergeFrom`).
- **Error classification** — terminal/config errors mark `Ready=False` with a stable reason and do
  not requeue; transient errors return an error to requeue with backoff.

## Webhook & manager wiring

- The validating webhook is **served by the controller manager** (single binary) — not a separate
  webhook binary + cainjector (cert-manager's split is for independent scaling; overkill here).
  Webhook serving certificates are provisioned by **cert-manager** (a `Certificate` + CA injection)
  or Helm-generated certs.
- Manager: leader election on (`LeaseDuration` 15s / `RenewDeadline` 10s,
  `LeaderElectionReleaseOnCancel: true`), metrics endpoint, health/readiness probes.

## RBAC

Generated from `//+kubebuilder:rbac` markers on the reconcilers (`make manifests`):

- Controller: `Relay` (get/list/watch + `status`/`finalizers`), `Deployment`/`Service`/`ConfigMap`
  (full CRUD in managed namespaces), `Secret` (get/list/watch for referenced secrets), `Lease`
  (leader election), `Event` (emit).
- Relay pods: **no Kubernetes API access**.

## Helm chart

`chart/iris/` installs controller + CRDs + RBAC + Postfix tier + LoadBalancer Service + webhook +
ServiceMonitor + PDB, mirroring the ingress-nginx chart layout. Key values: controller replica
count + leader election, Postfix replica count and resources, exposure `mode`, TLS/cert-manager
settings.
