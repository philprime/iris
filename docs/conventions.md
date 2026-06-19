# Conventions

Iris follows philprime's Go standards and kubebuilder conventions. Module
`github.com/philprime/iris`, Go 1.26, Makefile-driven.

## Repository layout

kubebuilder conventions + philprime's `cmd`/`internal` layout:

```
iris/
├── PROJECT  Makefile  go.mod  AGENTS.md  CLAUDE.md
├── Brewfile  dprint.json  .air.toml  .pre-commit-config.yaml  renovate.json
├── .github/workflows/      # analyze, build, format, generate, test, publish
├── api/v1alpha1/            # Relay types + kubebuilder markers + zz_generated.deepcopy.go
├── cmd/
│   ├── controller/{main,run}.go   # manager: reconcilers + webhook + leader election
│   ├── relay/{main,run}.go        # go-smtp data-plane server
│   └── reloader/{main,run}.go     # file-watch → copy + postmap + reload (baked into postfix image)
├── internal/
│   ├── controller/         # relay_controller.go, config_controller.go
│   ├── postfix/            # render transport / relay_domains / relay_recipient_maps (+ tests)
│   ├── relay/             # server, session, filter, transform, deliver_http, deliver_smtp, envelope
│   ├── config/            # env config w/ validator tags (all binaries)
│   ├── metrics/           # iris_* prometheus collectors (one per binary; see observability.md)
│   ├── logging/           # MultiHandler (terminal + Sentry slog fan-out)
│   ├── observability/     # Sentry init + slog/logr bridge (shared by all binaries)
│   └── webhook/           # validating webhook for Relay
├── config/                # kubebuilder kustomize bases: crd, rbac, manager, webhook
├── chart/iris/            # Helm chart (distribution; mirrors ingress-nginx)
│   ├── Chart.yaml  values.yaml  crds/  templates/
├── build/{controller,relay,postfix}.Dockerfile  # + postfix-entrypoint.sh
├── docs/
└── test/e2e/              # envtest + kind
```

**Three images:** `controller`, `relay`, `postfix` (boky/postfix + baked `reloader`).

- Flat package structure, group by feature not type.
- Dependencies flow inward: `cmd` → `internal`.
- `cmd/<binary>/` is a thin `main.go` + `run.go` (DI wiring via constructor functions).

## Go conventions

- **Makefile targets only**, never raw `go`. Run `make help` to discover targets.
- **`make format` before committing** (pre-commit runs gofmt/goimports + vet/staticcheck).
- **Config:** environment variables only, validated at startup with `validator` struct tags.
  Prefix `IRIS_<COMPONENT>_<SETTING>` (flag > env precedence where flags exist).
- **Logging:** `slog` with `…Context` variants when a `context.Context` is available. Use
  structured fields, never `fmt.Sprintf` in messages, and never `fmt.Print` in services.
- **Errors:** wrap with `fmt.Errorf("context: %w", err)` and use lowercase error strings
  (ST1005). Put exported sentinel errors in the package that owns the failure. Log OR return,
  never both.
- **DI:** constructor functions (`NewX`), accept interfaces / return concrete types, no global
  state.

## Testing

Table-driven tests, same-package unit tests, envtest for reconcilers, kind for e2e. Full
guidance in [testing.md](testing.md).

## Commits & PRs

- [Conventional Commits](https://www.conventionalcommits.org/): `feat`, `fix`, `build`, `chore`,
  `ci`, `docs`, `style`, `refactor`, `perf`, `test`.
- **No AI/co-author attribution** in commits, PRs, or comments.
- Branch from `main` with prefixes `feature/`, `fix/`, `refactor/`, `docs/`, `chore/`, `test/`.
- `main` must always be deployable.
