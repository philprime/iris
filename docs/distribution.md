# Distribution & release

Iris ships as **container images + a Helm chart** — not as end-user CLI binaries. The release
machinery follows the kula conventions used across the Go repos (multi-arch GHCR publish,
`docker/metadata-action` tagging, version metadata via `-ldflags`, and a `KULA_RELEASE_BOT`
GitHub App for automated cross-repo PRs).

## Artifacts

| Artifact           | Registry / location                 | Built from                                                           |
| ------------------ | ----------------------------------- | -------------------------------------------------------------------- |
| `controller` image | `ghcr.io/kula-app/iris/controller`  | `images/controller/Dockerfile` (distroless)                          |
| `relay` image      | `ghcr.io/kula-app/iris/relay`       | `images/relay/Dockerfile` (distroless)                               |
| `postfix` image    | `ghcr.io/kula-app/iris/postfix`     | `images/postfix/Dockerfile` (`FROM boky/postfix` + baked `reloader`) |
| Helm chart         | OCI: `ghcr.io/kula-app/charts/iris` | `chart/iris/`                                                        |

## Image build

- **controller / relay** — multi-stage: `golang:<ver>-alpine` builder → `gcr.io/distroless/static:nonroot`.
  `CGO_ENABLED=0`, `-trimpath`,
  `-ldflags="-s -w -X main.version=… -X main.commit=… -X main.date=… -X main.sentryRelease=iris@<version>:<git-sha>"`.
  The `sentryRelease` is the unified Sentry release identifier (see
  [observability.md](observability.md#release-identifier)). Runs as `nonroot:nonroot`.
- **postfix** — `FROM boky/postfix:<pinned>` with the `reloader` binary copied in; the reloader is
  the entrypoint that starts Postfix and watches the mounted routing ConfigMap (see
  [architecture.md](architecture.md)).
- All images are built `linux/amd64` + `linux/arm64` via QEMU + buildx and pushed as a
  multi-arch manifest list.

## Versioning & tags

- **Stable release:** a git tag `vX.Y.Z` → images tagged `X.Y.Z`, `X.Y`, `X`, and `latest`;
  the chart is packaged with `version`/`appVersion` = `X.Y.Z`.
- **Main (dev):** every push to `main` publishes `latest` (and the `main` ref tag).
- **PRs:** images build but are **not** pushed; tagged `pr-<n>` for inspection only.

Tags are derived by `docker/metadata-action` (see [ci.md](ci.md)).

## Helm chart

`chart/iris/` is the primary install surface (mirrors the ingress-nginx chart layout):

- `crds/` — generated CRDs (kept in sync by `make manifests`; CI enforces).
- `templates/` — controller Deployment + RBAC + leader-election, Postfix Deployment +
  LoadBalancer Service, validating webhook, ServiceMonitor, PDB.
- `values.yaml` — controller replicas + leader election, Postfix replicas/resources, exposure
  `mode` (`loadBalancer` default), image repos/tags, TLS/cert-manager settings.

Release flow:

```sh
make chart-lint       # helm lint
make chart-package    # helm package chart/iris
# CI on a vX.Y.Z tag: helm push iris-X.Y.Z.tgz oci://ghcr.io/kula-app/charts
```

Install:

```sh
helm install iris oci://ghcr.io/kula-app/charts/iris --version X.Y.Z -n iris-system --create-namespace
```

## Release process

1. Merge changes to `main` (CI green: analyze/build/format/generate/test).
2. Tag `vX.Y.Z` and push the tag.
3. `publish.yml` builds + pushes the three multi-arch images with semver tags; the chart job
   packages and `helm push`es the OCI chart with matching `version`/`appVersion`.
4. A GitHub Release is created with notes; the `KULA_RELEASE_BOT` App token is used for any
   automated PRs (e.g. bumping a deployment repo to the new chart version).

`main` must always be deployable; chart and image versions move together.

## Consuming Iris elsewhere

kula clusters install the chart via Pulumi (wrapping the OCI chart) the same way other shared
operators are deployed. Pin to a specific `vX.Y.Z`; `latest` is for dev clusters only.
