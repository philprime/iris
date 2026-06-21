# Kubernetes API & Controller

Iris follows standard controller conventions: a kubebuilder-style layout and markers, and Kubernetes API
conventions (Kstatus-style conditions, `observedGeneration`, printer columns, CEL validation,
finalizers, owner references, leader election, controller-runtime metrics).

## The `Relay` CRD

Group/Version/Kind: **`iris.philprime.dev/v1alpha1`, `Relay`** (namespaced). One `Relay` = one
transformer Deployment+Service + a set of routes compiled into Postfix.

The full field reference (every field, its type, defaults, and validation) is generated from the
API types into [crd-reference.md](crd-reference.md). A minimal `Relay`:

```yaml
apiVersion: iris.philprime.dev/v1alpha1
kind: Relay
metadata:
  name: appstore-invites
  namespace: example
spec:
  routes:
    - address: invites@invite.example.com
    - domain: invite.example.com
  destinations:
    - name: webhook
      http:
        url: https://service.internal/inbound
```

### Semantics worth calling out

These behaviors are not obvious from the generated field reference:

- An exact `address` route wins over a `@domain` route (Postfix transport semantics).
- `destinations` is a fan-out. Every accepted message is delivered to **all** of them. Each
  destination is exactly one of `http` or `smtp` (enforced by CEL plus the validating webhook) and
  carries its own transform and payload format.
- `required` (default true) gates retry. A required-destination failure returns SMTP 4xx so Postfix
  retries. See [relay.md](relay.md#delivery-contract).
- `filters` reject with SMTP 5xx before forwarding. Signal semantics are in
  [relay.md](relay.md#filters--scoring).
- Secrets are always referenced (`authSecretRef`, same namespace as the `Relay`), never inline.

## Conflict resolution

Route keys (exact addresses and `@domain`s) must be unique cluster-wide. On collision,
**first-writer-wins**, ordered deterministically by `(creationTimestamp, namespace, name)`. The
earliest claimant compiles into Postfix. Later claimants get `Conflict=True`, are excluded from
routing, and do not break the ingress. The validating webhook rejects obvious collisions at
apply-time, and the controller is the backstop for races.

## Controller internals

One manager, two reconcilers, leader-elected (HA, so only the leader reconciles):

1. **RelayReconciler** runs per `Relay`. It reconciles a **Deployment + Service**
   (owner-referenced for garbage collection) and renders the relay's config
   (routes/filters/destinations) into a per-relay **ConfigMap** that the relay pod mounts. It wires
   `authSecretRef`s as volumes/env into the pod (same namespace is allowed).
2. **ConfigReconciler** watches **all** `Relay`s, compiles the **global** Postfix maps
   (`transport`, `relay_domains`, `relay_recipient_maps`), and writes the Postfix **ConfigMap**.
   It owns conflict resolution. Render, then diff, then write only on change.

### Standard-controller hygiene

- `status.conditions` (`Ready`, `Programmed`, `Conflict`) per Kstatus, plus `observedGeneration`
  and CRD printer columns.
- A **finalizer** on `Relay` releases its routes from the aggregate before deletion.
- CEL `x-kubernetes-validations` on the CRD + a **validating webhook** (destination union,
  conflict pre-check, Jsonnet parse check).
- controller-runtime **metrics**, and leader election as a chart value (default on).
- Watches: `Relay` (primary), owned `Deployment`/`Service`, and the Postfix ConfigMap.

## Reconcile & status patterns

Grounded in cloudnative-pg, source-controller, and cert-manager (see [references.md](references.md)):

- **Idempotent child resources**: for each `Deployment`/`Service`/`ConfigMap` a `Relay` owns,
  `Get` then `Create` (after `ctrl.SetControllerReference`) if absent, else compute the desired
  object and `Patch` with `client.MergeFrom(original)`, skipping the write when `reflect.DeepEqual`
  shows no change. Verify ownership (`metav1.GetControllerOf`) before patching.
- **Status via patch-on-defer plus optimistic lock**: capture the object at the top of `Reconcile`,
  set conditions with apimachinery `meta.SetStatusCondition`, and in a deferred block patch status
  once via `client.Status().Patch(ctx, obj, client.MergeFromWithOptions(orig, client.MergeFromWithOptimisticLock{}))`
  wrapped in `retry.RetryOnConflict`, always setting `observedGeneration`. This is the
  dependency-light equivalent of flux's SerialPatcher/summarize, with **no `fluxcd/pkg/runtime`
  dependency**.
- **Conditions**: `Ready` summarizes the owned conditions (`Programmed`, `Conflict`, and a per-relay
  `DeploymentAvailable`). Negative-polarity conditions (e.g. `Conflict`) are present only when abnormal.
- **Finalizers**: `controllerutil.AddFinalizer` on first reconcile. On `DeletionTimestamp`, release
  the relay's routes from the aggregate, then `controllerutil.RemoveFinalizer` (both via `MergeFrom`).
- **Error classification**: terminal/config errors mark `Ready=False` with a stable reason and do
  not requeue. Transient errors return an error to requeue with backoff.

## Webhook & manager wiring

- The validating webhook is **served by the controller manager** (single binary), not a separate
  webhook binary plus cainjector. Cert-manager's split is for independent scaling, which is overkill
  here. Webhook serving certificates are provisioned by **cert-manager** (a `Certificate` plus CA
  injection), or supplied yourself when cert-manager is disabled.
- Manager: leader election on (`LeaseDuration` 15s / `RenewDeadline` 10s,
  `LeaderElectionReleaseOnCancel: true`), plus the metrics endpoint and health/readiness probes.
  Probe and metric wiring (ports, custom `iris_*` collectors) and Sentry are documented in
  [observability.md](observability.md).

## RBAC

The manager ClusterRole is generated from the `//+kubebuilder:rbac` markers on the reconcilers
(`make manifests`) into [`config/rbac/role.yaml`](../config/rbac/role.yaml), which is the source of
truth for the exact grants. The controller needs the `Relay` CR plus its status and finalizers, the
child `Deployment`/`Service`/`ConfigMap` it manages, referenced `Secret`s, leases for leader
election, and events. Relay pods need **no** Kubernetes API access.

## Helm chart

`chart/iris/` installs the controller, its RBAC, the CRDs, and the Postfix ingress tier, mirroring
the ingress-nginx chart layout. The validating webhook, ServiceMonitor, and PodDisruptionBudget are
gated by chart values (with different defaults). The configurable surface (replicas, resources,
exposure `service`, images, webhook and cert-manager settings) lives in
[`chart/iris/values.yaml`](../chart/iris/values.yaml).

Opportunistic STARTTLS on the public SMTP ports (25/587/465) is gated on `postfix.tls.enabled`.
When on, cert-manager provisions the serving certificate the same way as the webhook (a self-signed
Issuer by default, or `postfix.tls.certManager.issuerRef` for a real one), the secret is mounted
into the Postfix pod, and the image entrypoint enables TLS at level `may`. Senders that do not offer
TLS still deliver in plaintext.
