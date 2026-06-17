# AGENTS.md

**Iris** (`github.com/kula-app/iris`) — a Kubernetes controller providing a single public SMTP entrypoint for a cluster that routes/transforms inbound mail to in-cluster services. A replicated Postfix ingress terminates public SMTP; the controller compiles `Relay` CRs (`iris.kula.app/v1alpha1`) into Postfix routing maps and reconciles one transformer pod per relay that filters, transforms (canonical JSON + optional Jsonnet), and fans out to HTTP or downstream SMTP destinations. Docs: [`docs/`](docs/README.md) (architecture, kubernetes, relay, conventions, roadmap). Replaces a legacy Postfix relay previously in the `infra` repo.

## Ground rules

- Never chain separable shell commands with `&&` or `;`. Run each as its own command. Genuine pipelines (`a | b`) are fine.
- Run `git` plainly from the working directory (the project repo). Only use `git -C <path>` when targeting a different repo (e.g. the `infra` repo).
- Parse JSON with `jq` and YAML with `yq`. Do not hand-roll parsing or use Python one-liners.
- Do not use the built-in file memory system. Persist any durable instructions or learnings by keeping this `AGENTS.md` up to date instead.
