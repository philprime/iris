# Roadmap & open questions

Items intentionally out of v1 scope, to revisit.

- **Content-based routing** — a `match:` predicate per destination, selecting a subset of
  destinations per message (beyond v1 broadcast fan-out).
- **Additional exposure modes** — `traefik` (`IngressRouteTCP` + static entrypoint) and `none`.
  v1 ships `loadBalancer` only.
- **Per-relay autoscaling** — HPA and the signals to drive it.
- **Bounce/DSN handling policy** — surfaced through the CRD; currently delegated to Postfix
  defaults.
- **Multi-tenancy / domain-claim RBAC** — who may claim which domains, beyond first-writer-wins.

See [kubernetes.md](kubernetes.md) and [relay.md](relay.md) for the v1 behavior these would
extend.
