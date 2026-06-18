# Continuous integration

GitHub Actions workflows mirror the philprime controller convention (one concern per workflow). All
run on **push to `main`**, **pull requests**, and **`workflow_dispatch`**, each with a
concurrency group that cancels in-progress runs for the same ref.

| Workflow       | Runs                                                    | Fails when                             |
| -------------- | ------------------------------------------------------- | -------------------------------------- |
| `analyze.yml`  | `make analyze` (`go vet`, `staticcheck`, `govulncheck`) | Static-analysis or vuln findings       |
| `build.yml`    | `make build` (all binaries)                             | Compilation breaks                     |
| `format.yml`   | `make format`, then checks `git status --porcelain`     | Formatting not committed               |
| `generate.yml` | `make generate`, then checks `git status --porcelain`   | Generated code/CRDs not committed      |
| `test.yml`     | `make test` (unit + envtest) and `make test-e2e` (kind) | Any test fails                         |
| `publish.yml`  | Build + push the controller / relay / postfix images    | Build/push error (push skipped on PRs) |

Each Go workflow uses `actions/setup-go` with the version from `go.mod` and module caching.

## `generate.yml` (keeping codegen honest)

Because Iris defines CRDs, `generate.yml` regenerates `zz_generated.deepcopy.go` and the
CRD/RBAC/webhook manifests and fails if the working tree changes. The committed `config/` and
`chart/iris/crds/` must match `api/v1alpha1`. Run `make generate` before pushing.

## `publish.yml` (image build & push)

Modeled on the philprime multi-arch publish pattern:

- `docker/setup-qemu-action` + `docker/setup-buildx-action` for **`linux/amd64` + `linux/arm64`**.
- `docker/login-action` to **GHCR** (`ghcr.io`) using `GITHUB_TOKEN` (login + push skipped on PRs).
- `docker/metadata-action` derives tags: `latest` on `main`, `pr-<n>` on PRs (build-only), and
  semver `1.2.3` / `1.2` / `1` on `v*` tags.
- `docker/build-push-action` with `cache: type=gha,mode=max`, building all three images
  (`controller`, `relay`, `postfix`).

Release tagging, chart publishing, and the release bot are covered in
[distribution.md](distribution.md).
