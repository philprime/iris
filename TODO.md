# Iris remaining work

All handoff items are complete. `make build`, `make test` (unit + envtest),
`make analyze`, `make chart-lint`, and `make test-e2e` (kind) are green.

## Completed

1. **Cryptographic DKIM verification** (task 7). `internal/relay/filter.go` now
   verifies signatures with `go-msgauth/dkim` through an injectable resolver.
   `requireDKIM` and the `dkimDomain`/`authResults` signals reflect a valid
   signature whose `d=` matches an allowed domain.
2. **Postfix routing maps** (task 8). The image entrypoint and reloader point
   Postfix at the controller-rendered maps. boky/postfix compiles every
   referenced map with postmap, so the maps are copied from the read-only
   ConfigMap mount into a writable work directory and compiled there.
3. **Inbound STARTTLS via cert-manager** (task 9). Opportunistic STARTTLS on
   25/587/465 is gated on `postfix.tls.enabled`, with a cert-manager
   Certificate modeled on the webhook wiring.
4. **Envtest reconciler suite** (task 10). `internal/controller` runs against a
   real apiserver: a Relay create makes its children appear, a conflict is
   marked and excluded, and deletion releases the route via the finalizer.
5. **End-to-end kind suite** (task 11). `test/e2e` builds and loads the images,
   deploys the chart, sends mail through the Postfix ingress to a stub
   destination, and asserts the required-destination retry contract. Wired into
   `test.yml`.
