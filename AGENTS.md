# AGENTS.md

**Iris** (`github.com/philprime/iris`) — a Kubernetes controller providing a single public SMTP entrypoint for a cluster that routes/transforms inbound mail to in-cluster services. A replicated Postfix ingress terminates public SMTP; the controller compiles `Relay` CRs (`iris.philprime.dev/v1alpha1`) into Postfix routing maps and reconciles one transformer pod per relay that filters, transforms (canonical JSON + optional Jsonnet), and fans out to HTTP or downstream SMTP destinations. Docs: [`docs/`](docs/README.md) (architecture, kubernetes, relay, conventions, roadmap). Replaces a legacy Postfix relay previously in the `infra` repo.

## Ground rules

- The `Makefile` is the front door for dev tasks (build, test, generate, format, lint, deploy, …). Run `make help` to discover targets and prefer them over the underlying commands — e.g. `make format`, not `dprint fmt`/`go fmt` directly. Reach for the raw command only when no target fits.
- Never chain separable shell commands with `&&` or `;`. Run each as its own command. Genuine pipelines (`a | b`) are fine.
- Run `git` plainly from the working directory (the project repo). Only use `git -C <path>` when targeting a different repo (e.g. the `infra` repo).
- Draft commit messages and PR descriptions as markdown files under `./tmp/` (e.g. `./tmp/commit-message.md`, `./tmp/pr-body.md`), then pass them in — `git commit -F ./tmp/commit-message.md`, `gh pr create --body-file ./tmp/pr-body.md`. If a pre-commit hook reformats files and aborts the commit, you can re-run from the same file without redrafting.
- Parse, query, or transform JSON of any kind with `jq`, and YAML of any kind with `yq`. Never use `python3` (or any other language/script) to read or manipulate these files, and do not hand-roll parsing.
- Do not use the built-in file memory system. Persist any durable instructions or learnings by keeping this `AGENTS.md` up to date instead.
