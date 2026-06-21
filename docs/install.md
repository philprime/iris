# Installation

Iris is distributed as container images and an OCI Helm chart on GitHub Container Registry. The
chart in [`chart/iris/`](../chart/iris) is the install surface. It deploys the controller, the
Postfix ingress tier, and the validating webhook. This page covers a default install. For images,
versioning, and the release process, see [distribution.md](distribution.md).

## Prerequisites

- A Kubernetes cluster on 1.25 or newer. The `Relay` CRD uses CEL validation rules, which the API
  server enforces from 1.25.
- [cert-manager](https://cert-manager.io/docs/installation/) installed in the cluster. The
  validating webhook's serving certificate is provisioned through a cert-manager `Certificate` by
  default, so the chart needs cert-manager present. To install without it, either disable the
  webhook's cert-manager integration (`webhook.certManager.enabled=false`) and supply the
  certificate yourself, or disable the webhook (`webhook.enabled=false`).
- For the default `LoadBalancer` exposure, a cluster that can provision `LoadBalancer` Services (a
  cloud provider, or MetalLB on bare metal). This is what gives Iris a stable public IP for your MX
  records. On clusters without that, set `exposure.service.type=NodePort` and pin
  `exposure.service.nodePorts`. If you front Postfix yourself, set `exposure.service.enabled=false`.
- The Prometheus operator CRDs, only if you enable the `ServiceMonitor` (`metrics.serviceMonitor`),
  which is off by default.

## Install the chart

Replace `X.Y.Z` with a released version (see the [packages page](https://github.com/philprime/iris/pkgs/container/charts%2Firis)):

```sh
helm install iris oci://ghcr.io/philprime/charts/iris \
  --version X.Y.Z \
  -n iris-system --create-namespace
```

## Point your MX records at the ingress

The Postfix ingress is published through a `LoadBalancer` Service named `<release>-postfix` (for
the command above, `iris-postfix`). Read its external address once the cloud provider assigns one:

```sh
kubectl get svc iris-postfix -n iris-system -o wide
```

Create an MX record for your mail domain pointing at that address. Mail then arrives on ports 25,
587, and 465.

## Create a Relay

Routing is declarative. Apply a `Relay` describing the recipients you claim and where accepted mail
should go. See the example in the [README](../README.md#example), the field reference in
[crd-reference.md](crd-reference.md), and the field semantics in [kubernetes.md](kubernetes.md).

## Configure

The configurable surface lives in [`chart/iris/values.yaml`](../chart/iris/values.yaml), which
documents every value and its default. The settings you are most likely to touch:

- `controller.replicas` / `postfix.replicas`: availability and throughput of each tier.
- `exposure.service`: how the ingress is published (the Service `type`, and a pinned
  `loadBalancerIP` or `nodePorts` when applicable).
- `postfix.tls`: opportunistic STARTTLS on the public listeners. Off by default, so Postfix serves
  plaintext until you enable it.
- `webhook` and its `certManager` settings: how the validating webhook is served.
- `metrics.serviceMonitor`: scraping by the Prometheus operator.
- `sentry`: opt-in error reporting on every component.

Apply changes with `helm upgrade`.

## Verify

```sh
kubectl get pods -n iris-system
kubectl get relay -A
```

The controller and Postfix pods should be `Running`. Each `Relay` reports `Ready` and `Programmed`
conditions once the controller has compiled its routes.

## Uninstall

```sh
helm uninstall iris -n iris-system
```

Helm leaves the CRDs in place so your `Relay` resources survive a reinstall. Removing the CRDs
deletes every `Relay` in the cluster:

```sh
kubectl delete crd relays.iris.philprime.dev
```
