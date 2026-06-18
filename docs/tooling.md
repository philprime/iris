# Dev Tooling

Iris uses the philprime Go toolchain conventions: a `Makefile` front door, tools pinned via the
`go.mod` `tool` directive, `dprint` for non-Go formatting, `staticcheck`/`govulncheck`/`go vet`
for analysis, and `pre-commit` to enforce it all locally.

## Toolchain (go.mod `tool` directive)

Tools are versioned as module dependencies and invoked with `go tool <name>`, so there are no
global installs and Renovate keeps them current. The pinned versions live in `go.mod`. The table
below is only what each is for:

| Tool                                                 | Purpose                                                                                |
| ---------------------------------------------------- | -------------------------------------------------------------------------------------- |
| `sigs.k8s.io/controller-tools/cmd/controller-gen`    | Generate deepcopy code, CRD/RBAC/webhook manifests                                     |
| `sigs.k8s.io/controller-runtime/tools/setup-envtest` | Download the envtest API-server/etcd binaries                                          |
| `github.com/air-verse/air`                           | Hot-reload for `make dev`                                                              |
| `github.com/go-delve/delve/cmd/dlv`                  | Debugger                                                                               |
| `honnef.co/go/tools/cmd/staticcheck`                 | Static analysis                                                                        |
| `golang.org/x/vuln/cmd/govulncheck`                  | Vulnerability scanning                                                                 |
| `github.com/elastic/crd-ref-docs`                    | Generate CRD reference docs from `api/v1alpha1` (feeds [kubernetes.md](kubernetes.md)) |

System tools (Go, Docker, kubectl, Helm, kind, …) come from the `Brewfile` via `make init`.

## Makefile Targets

The `Makefile` is the front door for every dev task. Run `make help` for the
authoritative list. It is self-documenting, generated from the `##` comment
above each target. Targets are grouped into setup, code generation, building,
development, testing, quality, local cluster (kind), deployment, and maintenance.

Each non-trivial target wraps its output in a collapsible log group and emits a
success notice via `scripts/log.sh`, which speaks GitHub Actions workflow
commands (`::group::`, `::notice::`, …) in CI and prints plain text locally.

## Code Generation

Iris defines CRDs, so codegen is part of the loop. After editing `api/v1alpha1/*_types.go`:

```sh
make generate   # regenerates zz_generated.deepcopy.go + CRD/RBAC/webhook manifests
```

`make manifests` writes CRDs to `config/crd/` (kustomize base) and copies them into
`chart/iris/crds/` so the chart stays in sync. CI fails if generated files are not committed
(see [ci.md](ci.md)).

## Formatting

- **Go:** `gofmt`/`go fmt` (enforced, and also a pre-commit hook).
- **Everything else** (JSON, Markdown, TOML, YAML, Dockerfile): `dprint fmt`, configured in
  `dprint.json` (which also owns the list of excluded paths).

## Static Analysis

`make analyze` runs `go vet`, `staticcheck`, and `govulncheck`. Note the `staticcheck` ST1005
rule: error strings must start lowercase (see [conventions.md](conventions.md)).

## pre-commit

`.pre-commit-config.yaml` runs three layers of hooks on commit: housekeeping (whitespace,
merge-conflict, private-key, GitHub Actions/workflow schema, `shellcheck`), `make analyze`, and the
`gofmt`/`dprint` formatters. See the file for the exact set.

`make init` installs the git hook (via `make setup-hooks`, which runs `pre-commit install`). Run
`make setup-hooks` directly if you skipped `init`. Run `make format` before committing so the format
hooks pass on the first try.

## Renovate

`renovate.json` keeps Go modules (including the `tool` directive), GitHub Actions, Docker base
images, and the `boky/postfix` tag current via automated PRs.
