# Conventions

Iris follows kula's Go standards and kubebuilder conventions. Module
`github.com/kula-app/iris`, Go 1.26, Makefile-driven.

## Repository layout

kubebuilder conventions + kula's `cmd`/`internal` layout:

```
iris/
├── PROJECT  Makefile  go.mod  AGENTS.md  CLAUDE.md
├── api/v1alpha1/            # Relay types + kubebuilder markers + zz_generated.deepcopy.go
├── cmd/
│   ├── controller/{main,run}.go   # manager: reconcilers + webhook + leader election
│   ├── relay/{main,run}.go        # go-smtp data-plane server
│   └── reloader/{main,run}.go     # file-watch → postmap + postfix reload (baked into postfix image)
├── internal/
│   ├── controller/         # relay_controller.go, config_controller.go
│   ├── postfix/            # render transport / relay_domains / relay_recipient_maps (+ tests)
│   ├── relay/             # server, session, filter, transform, deliver_http, deliver_smtp, envelope
│   ├── config/            # env config w/ validator tags (both binaries)
│   └── webhook/           # validating webhook for Relay
├── config/                # kubebuilder kustomize bases: crd, rbac, manager, webhook
├── chart/iris/            # Helm chart (distribution; mirrors ingress-nginx)
│   ├── Chart.yaml  values.yaml  crds/  templates/
├── images/{controller,relay,postfix}/Dockerfile
├── docs/
└── test/e2e/              # envtest + kind
```

**Three images:** `controller`, `relay`, `postfix` (boky/postfix + baked `reloader`).

- Flat package structure, group by feature not type.
- Dependencies flow inward: `cmd` → `internal`.
- `cmd/<binary>/` is a thin `main.go` + `run.go` (DI wiring via constructor functions).

## Go conventions

- **Makefile targets only** — never raw `go`; run `make help` to discover targets.
- **`make format` before committing** (pre-commit runs gofmt/goimports + vet/staticcheck).
- **Config:** environment variables only, validated at startup with `validator` struct tags.
  Prefix `IRIS_<COMPONENT>_<SETTING>` (flag > env precedence where flags exist).
- **Logging:** `slog` with `…Context` variants when a `context.Context` is available; structured
  fields, never `fmt.Sprintf` in messages; never `fmt.Print` in services.
- **Errors:** wrap with `fmt.Errorf("context: %w", err)`; lowercase error strings (ST1005);
  exported sentinel errors in the package that owns the failure; log OR return, never both.
- **DI:** constructor functions (`NewX`), accept interfaces / return concrete types, no global
  state.

## Testing

- Table-driven tests with `t.Run`; `t.Parallel()` for isolated state.
- Same-package unit tests; integration tests in `package x_test`.
- Integration tests behind `//go:build integration` (`go test -tags=integration ./...`).
- Controller tests use **envtest**; end-to-end uses **kind** under `test/e2e/`.
- Integration tests carry a Gherkin-style header comment (Feature/Scenario/Given/When/Then).

## Commits & PRs

- [Conventional Commits](https://www.conventionalcommits.org/): `feat`, `fix`, `build`, `chore`,
  `ci`, `docs`, `style`, `refactor`, `perf`, `test`.
- **No AI/co-author attribution** in commits, PRs, or comments.
- Branch from `main` with prefixes `feature/`, `fix/`, `refactor/`, `docs/`, `chore/`, `test/`.
- `main` must always be deployable.
