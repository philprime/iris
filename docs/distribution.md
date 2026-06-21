# Distribution & release

Iris ships as **container images + a Helm chart**, not as end-user CLI binaries. The release
machinery follows the philprime conventions used across the Go repos (multi-arch GHCR publish,
`docker/metadata-action` tagging, version metadata via `-ldflags`, and a `PHILPRIME_RELEASE_BOT`
GitHub App for automated cross-repo PRs).

## Artifacts

| Artifact           | Registry / location                  | Built from                                                          |
| ------------------ | ------------------------------------ | ------------------------------------------------------------------- |
| `controller` image | `ghcr.io/philprime/iris-controller`  | `build/controller.Dockerfile` (distroless)                          |
| `relay` image      | `ghcr.io/philprime/iris-relay`       | `build/relay.Dockerfile` (distroless)                               |
| `postfix` image    | `ghcr.io/philprime/iris-postfix`     | `build/postfix.Dockerfile` (`FROM boky/postfix` + baked `reloader`) |
| Helm chart         | OCI: `ghcr.io/philprime/charts/iris` | `chart/iris/`                                                       |

## Image build

- **controller / relay** are multi-stage: `golang:<ver>-alpine` builder → `gcr.io/distroless/static:nonroot`.
  `CGO_ENABLED=0`, `-trimpath`,
  `-ldflags="-s -w -X main.version=… -X main.commit=… -X main.date=… -X main.sentryRelease=iris@<version>:<git-sha>"`.
  The `sentryRelease` is the unified Sentry release identifier (see
  [observability.md](observability.md#release-identifier)). Runs as `nonroot:nonroot`.
- **postfix** is `FROM boky/postfix` (pinned via renovate) with the `reloader` binary baked in. The
  image entrypoint seeds a writable copy of the mounted routing maps, runs the reloader as a
  background companion, and hands off to the Postfix master, so the reloader watches the maps and
  runs postmap plus postfix reload on change (see [architecture.md](architecture.md)).
- All images are built `linux/amd64` + `linux/arm64` via QEMU + buildx and pushed as a
  multi-arch manifest list.

## Versioning & tags

- **Stable release:** a git tag `vX.Y.Z` → images tagged `X.Y.Z`, `X.Y`, `X`, and `latest`.
  The chart is packaged with `version`/`appVersion` = `X.Y.Z`.
- **Main (dev):** every push to `main` publishes `latest` (and the `main` ref tag).
- **PRs:** images build but are **not** pushed. They are tagged `pr-<n>` for inspection only.

Tags are derived by `docker/metadata-action`.

## Helm chart

`chart/iris/` is the primary install surface (mirrors the ingress-nginx chart layout):

- `crds/`: generated CRDs (kept in sync by `make manifests`, and CI enforces this).
- `templates/`: the controller, Postfix tier, validating webhook, and monitoring resources.
- `values.yaml`: the configurable surface (replicas, resources, exposure `service`, images, webhook
  settings). See the file for the full set and defaults.

Release flow:

```sh
make chart-lint       # helm lint
make chart-package    # helm package chart/iris
# CI on a vX.Y.Z tag: helm push iris-X.Y.Z.tgz oci://ghcr.io/philprime/charts
```

Install:

```sh
helm install iris oci://ghcr.io/philprime/charts/iris --version X.Y.Z -n iris-system --create-namespace
```

## Release process

1. Merge changes to `main` (CI green: analyze/build/format/generate/test).
2. Tag `vX.Y.Z` and push the tag.
3. `publish.yml` builds + pushes the three multi-arch images with semver tags. The chart job
   packages and `helm push`es the OCI chart with matching `version`/`appVersion`.
4. A GitHub Release is created with notes. The `PHILPRIME_RELEASE_BOT` App token is used for any
   automated PRs (e.g. bumping a deployment repo to the new chart version).

`main` must always be deployable, and chart and image versions move together.

## Consuming Iris elsewhere

philprime clusters install the chart via Pulumi (wrapping the OCI chart) the same way other shared
operators are deployed. Pin to a specific `vX.Y.Z`. The `latest` tag is for dev clusters only.
