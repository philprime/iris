# Relay (data plane)

The relay is a stateless `emersion/go-smtp` server. One Deployment+Service runs per `Relay` CR.
It is configured entirely from a mounted file + mounted Secrets and needs **no Kubernetes API
access**.

## Session pipeline

For each inbound SMTP session from the Postfix ingress:

1. **`MAIL FROM`** → sender filter (allowed sender domains).
2. **`RCPT TO`** → route match (the relay only accepts recipients it claims).
3. **`DATA`** →
   - enforce `maxMessageBytes` (reject 552 if exceeded);
   - parse MIME;
   - run filter **scoring** (reject 5xx if below `minScore`);
   - compute the **idempotency key** once;
   - **fan out** to all destinations (per-destination transform + deliver);
   - return `250` if all `required` destinations succeed, `4xx` if any `required` destination
     fails (Postfix retries). Best-effort (`required: false`) failures are logged + metered.

## Filters & scoring

Declarative inbound validation from `spec.filters`:

- **Hard rules** (reject before forwarding): `maxMessageBytes`, `allowedSenderDomains`,
  `requireDKIM` (DKIM `d=` must match).
- **Heuristic score** — sum of matched `scoreSignals`; accept when `score >= minScore`. Signals
  are named, reusable checks:
  - `fromDomain` — `From:` header domain is allowed.
  - `messageIdDomain` — `Message-ID` domain is allowed.
  - `dkimDomain` — DKIM `d=` domain is allowed.
  - `authResults` — `Authentication-Results` shows DKIM pass for an allowed domain.
  - `bodyLinkDomain` — body contains a link to an allowed domain.

## Transform

Each destination transforms independently:

1. **MIME → canonical envelope** (the fixed schema below).
2. **Optional Jsonnet** (`google/go-jsonnet`) per destination, remapping the canonical envelope
   into the consumer's own schema (Ory-Kratos-style mapping). Referenced via
   `jsonnetConfigMapRef`.
3. **Payload format** — `json` (canonical envelope, default) or `raw` (`message/rfc822`).

### Canonical JSON envelope

What `payloadFormat: json` emits and what a Jsonnet `transform` receives as input. Fixed,
documented, versioned schema:

```json
{
  "version": "v1",
  "idempotencyKey": "<message-id-or-sha256>",
  "envelope": { "mailFrom": "...", "rcptTo": ["..."] },
  "headers": { "Subject": "...", "From": "...", "...": "..." },
  "from": "...",
  "to": ["..."],
  "subject": "...",
  "text": "...",
  "html": "...",
  "attachments": [
    { "filename": "...", "contentType": "...", "bytesBase64": "..." }
  ],
  "raw": "<optional full RFC822, base64>"
}
```

## Delivery

- **HTTP** — POST (or configured method) with a timeout, the `Idempotency-Key` header, and
  secret-based `Authorization`.
- **SMTP** — forward to a downstream host/port with optional STARTTLS/auth. (Pointing this at
  another relay's Service is how manual relay→relay chaining is done.)

## Delivery contract

The pipeline is **at-least-once** (Postfix queue + retry). Fan-out is **not atomic**: if one of
several destinations fails and triggers a retry, destinations that already succeeded receive the
message again. Mitigations (standard for any queue-backed email system):

1. Every delivery carries an **idempotency key** (`Idempotency-Key` HTTP header and/or in the JSON
   envelope) so downstreams dedup.
2. **`required: false`** marks best-effort destinations whose failure does not trigger a retry.

This contract — _at-least-once, idempotency-key-deduped, `required` gates retry_ — is part of the
public API.

## Config file format

The relay's mounted config is rendered as **YAML via `sigs.k8s.io/yaml`**, reusing the
`api/v1alpha1` structs as the single source of truth (YAML→JSON→struct through json tags). The
file carries a `version` field for forward compatibility, and stays debuggable with
`kubectl get cm -o yaml`. (Postfix map files use Postfix's own native format, not YAML.)

## Observability

`slog` (with `…Context` variants) + per-destination success/failure/score metrics. The relay
serves `/livez` `/readyz` `/healthz` via [`kula-app/go-health`](https://github.com/kula-app/go-health)
and `/metrics` via `promhttp` on a small admin HTTP server (no Kubernetes API access). Destination
reachability is a `/healthz`-only (informational) check, **not** a readiness gate — Postfix queues
and retries on failure, so a flaky downstream must not drain the relay. Full surface, the `iris_relay_*`
metric catalogue, and Sentry capture rules are in [observability.md](observability.md).
