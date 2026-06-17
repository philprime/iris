# Testing

Three layers, fastest first: unit tests, **envtest** for reconcilers against a real API server,
and **kind** for end-to-end coverage of the whole chain.

## Unit tests

- Table-driven with `t.Run`; `t.Parallel()` where state is isolated.
- Same-package (`package x`) for internal unit tests; `package x_test` for black-box/integration.
- Pure logic lives in small, well-bounded packages so it tests without a cluster:
  - `internal/postfix` — map rendering (`transport`, `relay_domains`, `relay_recipient_maps`),
    conflict resolution ordering.
  - `internal/relay` — filter scoring, MIME → canonical envelope, Jsonnet transform, idempotency
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

- Binaries are fetched by `make setup-envtest`; `make test` sets `KUBEBUILDER_ASSETS`.
- Use these for: a `Relay` create → Deployment+Service appear; a conflicting `Relay` →
  `Conflict=True` and not compiled; delete → finalizer releases the route from the aggregate.

## End-to-end (kind)

`test/e2e/` runs the real images in a kind cluster: apply a `Relay`, send SMTP to the Postfix
ingress, assert the message reaches a stub HTTP/SMTP destination, and assert retry behavior on a
`required` destination failure.

```sh
make test-e2e   # creates kind cluster, builds+loads images, deploys chart, runs the suite
```

The e2e suite covers what envtest cannot: the Postfix reload path, the relay's SMTP server, and
the at-least-once delivery contract end to end.

## Conventions

- Integration/e2e tests behind `//go:build integration` where they need external deps
  (`go test -tags=integration ./...`).
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
