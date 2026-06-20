# Development setup

How to get a local Iris development environment running. Iris follows the same local-dev shape
as other philprime Go controllers. A `Makefile` is the single entry point, `air` provides hot-reload,
and `kind` provides a throwaway cluster.

> All commands go through the `Makefile`. Never run raw `go`. Run `make help` to list targets.
> Tooling reference: [tooling.md](tooling.md).

## Prerequisites

| Tool    | Purpose                                | Install                 |
| ------- | -------------------------------------- | ----------------------- |
| Go 1.26 | Toolchain (version pinned in `go.mod`) | `brew install go`       |
| Docker  | Build images, run `kind`               | Docker Desktop / colima |
| kind    | Local Kubernetes for e2e               | `make init` (Brewfile)  |
| kubectl | Talk to the cluster                    | `make init`             |
| Helm    | Render/install the chart               | `make init`             |
| dprint  | Non-Go formatting                      | `make init`             |

Go-based tools (`controller-gen`, `setup-envtest`, `air`, `delve`, `staticcheck`, `govulncheck`)
are declared in the `go.mod` `tool` directive and run via `go tool`, so there is no manual install.

## First-time setup

```sh
make init      # platform-detects (init-darwin / init-linux); runs `brew bundle`, installs deps
make install   # go mod tidy
make generate  # controller-gen: deepcopy + CRD manifests
make build     # build controller, relay, reloader binaries into dist/
```

## Running locally

The controller runs out-of-cluster against whatever cluster your `kubeconfig` points at. The
data plane (Postfix + relay) runs in-cluster.

```sh
make dev-controller   # run the controller with air hot-reload (.air.controller.toml)
make run              # run the controller once, no hot-reload (go run ./cmd/controller)
```

Point at a local cluster first:

```sh
make kind-up     # create a kind cluster and load CRDs
make deploy      # install the chart (controller + Postfix tier) into the cluster
# ... iterate with `make dev-controller` against that cluster ...
make kind-down   # tear it down
```

The **relay** also runs cleanly on the host, since it needs no Kubernetes API access (only a
config file and mounted secrets). `make dev-relay` hot-reloads it against the checked-in sample
config, listening on an unprivileged SMTP port. The settings live in `.air.relay.toml` and
`hack/dev/relay.config.yaml`.

```sh
make dev-relay   # run the relay locally with air hot-reload
```

The **reloader** has no local-dev target on purpose. Its job is to shell out to `postmap` and
`postfix reload`, which only exist inside the Postfix image, so it is exercised through its tests
rather than run on the host.

To iterate on the relay in-cluster instead, it runs as a normal pod. Build and load its image into
kind:

```sh
make build-docker        # buildx the controller/relay/postfix images
make kind-load           # load images into the kind cluster
```

## Debugging

`delve` is available via `go tool dlv`. The `air` configs build with symbols for dev. Attach
delve to the running `./tmp/controller` process, or run `go tool dlv debug ./cmd/controller`.

## Testing locally

See [testing.md](testing.md). In short: `make test` (unit + envtest), `make test-e2e`
(kind-based end-to-end).

## Sending a test email

Once the chart is deployed and a `Relay` exists, send a message to the Postfix LoadBalancer:

```sh
kubectl port-forward svc/iris-postfix 2525:25
swaks --server localhost:2525 --to invites@invite.example.com --from test@email.apple.com
```
