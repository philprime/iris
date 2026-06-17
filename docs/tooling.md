# Dev tooling

Iris uses the kula Go toolchain conventions: a `Makefile` front door, tools pinned via the
`go.mod` `tool` directive, `dprint` for non-Go formatting, `staticcheck`/`govulncheck`/`go vet`
for analysis, and `pre-commit` to enforce it all locally.

## Toolchain (go.mod `tool` directive)

Tools are versioned as module dependencies and invoked with `go tool <name>` — no global installs,
Renovate keeps them current:

| Tool | Purpose |
| --- | --- |
| `sigs.k8s.io/controller-tools/cmd/controller-gen` | Generate deepcopy code, CRD/RBAC/webhook manifests |
| `sigs.k8s.io/controller-runtime/tools/setup-envtest` | Download the envtest API-server/etcd binaries |
| `github.com/air-verse/air` | Hot-reload for `make dev` |
| `github.com/go-delve/delve/cmd/dlv` | Debugger |
| `honnef.co/go/tools/cmd/staticcheck` | Static analysis |
| `golang.org/x/vuln/cmd/govulncheck` | Vulnerability scanning |
| `github.com/elastic/crd-ref-docs` | Generate CRD reference docs from `api/v1alpha1` (feeds [kubernetes.md](kubernetes.md)) |

System tools (Go, Docker, kind, kubectl, Helm, dprint) come from the `Brewfile` via `make init`.

## Makefile targets

`make help` is the source of truth. Catalogue:

| Target | Purpose |
| --- | --- |
| `init` / `init-darwin` / `init-linux` | Platform-detect and install system deps (`brew bundle`, dprint) |
| `install` | `go mod tidy` |
| `generate` | `controller-gen` deepcopy + `manifests` (CRDs/RBAC/webhook) into `config/` and `chart/iris/crds/` |
| `build` | Build `controller`, `relay`, `reloader` binaries into `dist/` |
| `build-docker` | `docker buildx` the controller / relay / postfix images |
| `dev` | Run the controller with `air` hot-reload (`.air.toml`) |
| `run` | `go run ./cmd/controller` (no hot-reload) |
| `test` | Unit tests + envtest (`go test ./...` with `KUBEBUILDER_ASSETS`) |
| `test-coverage` | Tests with a coverage report (`tmp/coverage.out`) |
| `test-e2e` | kind-based end-to-end suite (`test/e2e/`) |
| `setup-envtest` | Download/locate the envtest binaries |
| `kind-up` / `kind-down` / `kind-load` | Manage the local kind cluster and load images |
| `deploy` / `undeploy` | Install/remove the Helm chart in the current cluster |
| `analyze` | `go vet` + `staticcheck` + `govulncheck` |
| `format` | `go mod tidy` + `go fmt ./...` + `dprint fmt` |
| `chart-lint` / `chart-package` | `helm lint` / `helm package` the chart |
| `upgrade-deps` | `go get -u ./...` then `make format` |
| `help` | List targets |

## Code generation

Iris defines CRDs, so codegen is part of the loop. After editing `api/v1alpha1/*_types.go`:

```sh
make generate   # regenerates zz_generated.deepcopy.go + CRD/RBAC/webhook manifests
```

`make manifests` writes CRDs to `config/crd/` (kustomize base) and copies them into
`chart/iris/crds/` so the chart stays in sync. CI fails if generated files are not committed
(see [ci.md](ci.md)).

## Formatting

- **Go:** `gofmt`/`go fmt` (enforced; also a pre-commit hook).
- **Everything else** (JSON, Markdown, TOML, YAML, Dockerfile): `dprint fmt`, configured in
  `dprint.json`. `dist/`, `build/`, and `tmp/` are excluded.

## Static analysis

`make analyze` runs `go vet`, `staticcheck`, and `govulncheck`. Note the `staticcheck` ST1005
rule: error strings must start lowercase (see [conventions.md](conventions.md)).

## pre-commit

`.pre-commit-config.yaml` runs the housekeeping + format + analyze hooks locally:

- `check-case-conflict`, `check-merge-conflict`, `check-symlinks`, `check-xml`,
  `detect-private-key`, `end-of-file-fixer`
- `check-github-actions`, `check-github-workflows`, `shellcheck`
- `make-analyze` (runs `make analyze`)
- `format-go` (`gofmt -w`), `format-json` / `format-markdown` / `format-yaml` (`dprint fmt`)

Run `make format` before committing so the format hooks pass on the first try.

## Renovate

`renovate.json` keeps Go modules (including the `tool` directive), GitHub Actions, Docker base
images, and the `boky/postfix` tag current via automated PRs.
