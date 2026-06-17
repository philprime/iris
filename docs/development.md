# Development setup

How to get a local Iris development environment running. Iris follows the same local-dev shape
as other kula Go controllers: a `Makefile` is the single entry point, `air` provides hot-reload,
and `kind` provides a throwaway cluster.

> All commands go through the `Makefile` — never run raw `go`. Run `make help` to list targets.
> Tooling reference: [tooling.md](tooling.md).

## Prerequisites

| Tool | Purpose | Install |
| --- | --- | --- |
| Go 1.26 | Toolchain (version pinned in `go.mod`) | `brew install go` |
| Docker | Build images, run `kind` | Docker Desktop / colima |
| kind | Local Kubernetes for e2e | `make init` (Brewfile) |
| kubectl | Talk to the cluster | `make init` |
| Helm | Render/install the chart | `make init` |
| dprint | Non-Go formatting | `make init` |

Go-based tools (`controller-gen`, `setup-envtest`, `air`, `delve`, `staticcheck`, `govulncheck`)
are declared in the `go.mod` `tool` directive and run via `go tool` — no manual install.

## First-time setup

```sh
make init      # platform-detects (init-darwin / init-linux); runs `brew bundle`, installs deps
make install   # go mod tidy
make generate  # controller-gen: deepcopy + CRD manifests
make build     # build controller, relay, reloader binaries into dist/
```

## Running locally

The controller runs out-of-cluster against whatever cluster your `kubeconfig` points at; the
data plane (Postfix + relay) runs in-cluster.

```sh
make dev       # run the controller with air hot-reload (.air.toml)
make run       # run the controller once, no hot-reload (go run ./cmd/controller)
```

Point at a local cluster first:

```sh
make kind-up     # create a kind cluster and load CRDs
make deploy      # install the chart (controller + Postfix tier) into the cluster
# ... iterate with `make dev` against that cluster ...
make kind-down   # tear it down
```

For data-plane iteration, the **relay** runs as a normal pod; build and load its image into kind:

```sh
make build-docker        # buildx the controller/relay/postfix images
make kind-load           # load images into the kind cluster
```

## Debugging

`delve` is available via `go tool dlv`. The `air` configs build with symbols for dev; attach
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
