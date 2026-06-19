# Iris remaining work

Handoff for the remaining gaps after the control plane, data plane, shared
infrastructure, Helm chart, images, and CI all landed. Work the items top to
bottom (lower numbers first). They are roughly ordered so the e2e suite, which
proves everything, comes last.

## Ground rules (read `AGENTS.md` first)

- **Makefile only.** Never run `go`, `helm`, `docker`, `kind`, `dprint`, etc.
  directly. Use `make build`, `make test`, `make analyze`, `make format`,
  `make chart-lint`, `make build-docker`, `make test-e2e`. Add a target if none
  fits.
- **TDD.** Write a failing test first, watch it fail, then implement. Focused
  test for concurrency? `make test PKG=./pkg/... RUN=TestName RACE=1`.
- **Commit after each task** with a Conventional Commit, drafting the message
  under `./tmp/`. No AI attribution.
- **One source of truth.** Do not restate code/config in docs; point to it.
- Reference repos for house style live at
  `/home/philprime/dev/kula-app/shipable/shipable-deployer` (Sentry, go-health).
- Verify before claiming done: `make build`, `make test`, `make analyze` (and
  `make chart-lint` for chart changes) must pass.

## Tasks

### 7. Cryptographic DKIM verification

`internal/relay/filter.go` currently checks DKIM **structurally** (a
`DKIM-Signature` header with a matching `d=`, an `Authentication-Results` with
`dkim=pass`). Replace with real verification.

- Use `github.com/emersion/go-msgauth/dkim` (`dkim.Verify`).
- `requireDKIM` and the `dkimDomain` / `authResults` signals should reflect a
  cryptographically valid signature whose `d=` matches an allowed domain.
- Make it testable without DNS: inject the public-key lookup (the verifier
  takes a resolver) so a test can sign a message locally and supply the key,
  rather than hitting real DNS. Generate an RSA key in the test, build a signed
  message with `dkim.Sign`, and verify with the injected key.
- Files: `internal/relay/filter.go` (+ test). Wire the verifier into `Evaluate`.

### 8. Wire Postfix `main.cf` to the mounted routing maps (functional blocker)

The chart mounts the maps ConfigMap at `/etc/postfix/maps` and the controller
renders into it, but Postfix is not configured to consume them, so recipients
do not route to the relay Services. This is the main blocker for end-to-end
delivery.

- Configure `transport_maps = hash:/etc/postfix/maps/transport`,
  `relay_recipient_maps = hash:/etc/postfix/maps/relay_recipient_maps`, and
  `relay_domains` from `/etc/postfix/maps/relay_domains` via the `boky/postfix`
  configuration mechanism (env vars `POSTFIX_*` / extra config, see boky docs)
  and the chart.
- The reloader already runs `postmap` on `transport` and
  `relay_recipient_maps`; confirm those are the files Postfix reads.
- `relay_domains` is a plain list, not a hashed map, so it is referenced
  differently (a file value, not `hash:`).
- Files: `chart/iris/templates/postfix.yaml` (env/config), possibly
  `build/postfix.Dockerfile` / a baked `main.cf` snippet. Verify with
  `make chart-lint` and ideally the e2e suite (task 11).

### 9. Inbound SMTP STARTTLS via cert-manager

Provision a serving certificate for the public Postfix listener and enable
opportunistic STARTTLS on 25/587/465.

- Add a cert-manager `Certificate` for the Postfix tier (gated on chart values),
  mount it into the Postfix pod, and point `boky/postfix` TLS env at it.
- Reuse the webhook cert-manager wiring as a model
  (`chart/iris/templates/webhook.yaml`).
- Files: `chart/iris/templates/postfix.yaml`, `values.yaml`. Verify with
  `make chart-lint`.

### 10. Envtest reconciler suite

The reconcilers are covered by controller-runtime fake-client unit tests.
`docs/testing.md` calls for **envtest** against a real apiserver. Add a suite
under `internal/controller` that uses the `KUBEBUILDER_ASSETS` `make test`
already provides (`make setup-envtest`).

- Cover: a `Relay` create makes a Deployment+Service+ConfigMap appear; a
  conflicting `Relay` is marked `Conflict=True` and excluded from the Postfix
  ConfigMap; deletion releases the route via the finalizer.
- Use `sigs.k8s.io/controller-runtime/pkg/envtest` with the CRDs from
  `config/crd/bases`. Start the manager (or call reconcilers directly against
  the envtest client) in a `TestMain`/suite.
- Keep them behind the normal `make test` (envtest assets are already wired).

### 11. End-to-end kind suite (proves everything)

Add `test/e2e/` so `make test-e2e` creates a kind cluster, builds and loads the
images, deploys the chart, applies a `Relay`, sends SMTP to the Postfix ingress,
and asserts the message reaches a stub HTTP/SMTP destination plus retry
behavior on a required-destination failure.

- The target already exists (`make test-e2e`, builds tagged `e2e`); the suite
  (`go:build e2e`) does not. `kind` is installed.
- Depends on task 8 (Postfix routing) to actually deliver.
- Stub destination: a small in-cluster HTTP server (or the relay's SMTP) that
  records receipt. Use `swaks` or a Go SMTP client to inject mail.
- Once green, wire `make test-e2e` into `.github/workflows/test.yml` (and update
  `docs/ci.md`, which currently lists only `make test`).

## Done already (for context)

Control plane (reconcilers + webhook + manager), relay data plane (pipeline,
delivery, concurrent fan-out, server, binaries), reloader, shared infra
(`internal/config`, `logging`, `observability`, `metrics`), Helm chart, three
Dockerfiles, CI workflows, data-plane metrics, go-health probes, webhook Jsonnet
parse-check, chart RBAC sync, and Sentry capture with terminal error
classification. `make build` (all three binaries), `make test`, `make analyze`,
and `make chart-lint` are green.
