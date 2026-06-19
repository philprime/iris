# Testing

Three layers, fastest first: unit tests, **envtest** for reconcilers against a real API server,
and **kind** for end-to-end coverage of the whole chain.

## Unit tests

- Table-driven with `t.Run`, and `t.Parallel()` where state is isolated.
- Same-package (`package x`) for internal unit tests. Use `package x_test` for black-box/integration.
- Pure logic lives in small, well-bounded packages so it tests without a cluster:
  - `internal/postfix` handles map rendering (`transport`, `relay_domains`,
    `relay_recipient_maps`) and conflict resolution ordering.
  - `internal/relay` covers filter scoring, MIME → canonical envelope, Jsonnet transform, idempotency
    key derivation, fan-out result aggregation (which combination of `required` failures yields
    SMTP 4xx vs 250).

```sh
make test           # unit + envtest
make test-coverage  # + coverage report at tmp/coverage.out
```

## envtest (reconciler tests)

Controller reconcilers are tested against a real Kubernetes API server (apiserver + etcd, no
kubelet/scheduler) provided by `setup-envtest`. This validates CRD schemas, status/conditions,
owner references, and finalizer behavior without a full cluster.

- Binaries are fetched by `make setup-envtest`, and `make test` sets `KUBEBUILDER_ASSETS`. The
  suite (`internal/controller`) skips itself when the assets are absent, so a bare `go test` still
  runs the fake-client unit tests.
- It covers the following. A `Relay` create makes a Deployment+Service+ConfigMap appear. A
  conflicting `Relay` is marked `Conflict=True` and excluded from the aggregate. A delete makes the
  finalizer release the route from the aggregate.

## End-to-end (kind)

`test/e2e/` (build tag `e2e`) runs the real images in a kind cluster. It deploys a stub HTTP
destination and two `Relay`s, sends mail with `swaks` to the Postfix ingress, asserts the message
reaches the stub through the relay, and asserts that a `required` destination failure makes the
relay answer SMTP `4xx` (the retry signal Postfix acts on).

```sh
make test-e2e       # creates kind cluster, builds+loads images, deploys chart, runs the suite
make test-e2e-run   # only the suite, against an already-deployed cluster (fast iteration)
```

`make test-e2e` deploys with the webhook disabled (cert-manager is not installed in the kind
cluster) and the locally built `:dev` images. It needs `kind`, `helm`, `kubectl`, and `swaks` on
`PATH`. The suite covers what envtest cannot: the Postfix routing and reload path, the relay's SMTP
server, and the delivery contract end to end.

## Conventions

- Tests that need external deps sit behind a build tag: the kind suite uses `//go:build e2e`
  (`make test-e2e`), so a plain `make test` never needs a cluster.
- Every integration test carries a Gherkin-style header comment:

  ```go
  // Feature: <feature>
  // Scenario: <scenario>
  //   Given <state>
  //   When  <action>
  //   Then  <outcome>
  func TestX(t *testing.T) { ... }
  ```

- Mock external dependencies via consumer-defined interfaces (declared in the package that uses
  them) to keep fakes minimal.
