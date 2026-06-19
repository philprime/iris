# API Reference

## Packages

- [iris.philprime.dev/v1alpha1](#irisphilprimedevv1alpha1)

## iris.philprime.dev/v1alpha1

Package v1alpha1 contains the API schema definitions for the iris
v1alpha1 API group (iris.philprime.dev).

### Resource Types

- [Relay](#relay)
- [RelayList](#relaylist)

#### ConfigMapKeyRef

ConfigMapKeyRef references a single key in a ConfigMap in the Relay's
namespace.

_Appears in:_

- [Transform](#transform)

| Field           | Description                          | Default | Validation          |
| --------------- | ------------------------------------ | ------- | ------------------- |
| `name` _string_ | name is the ConfigMap name.          |         | MinLength: 1 <br /> |
| `key` _string_  | key is the key within the ConfigMap. |         | MinLength: 1 <br /> |

#### DeploymentSpec

DeploymentSpec tunes the per-relay transformer Deployment.

_Appears in:_

- [RelaySpec](#relayspec)

| Field                                                                                                                                   | Description                                                | Default | Validation                             |
| --------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------- | ------- | -------------------------------------- |
| `replicas` _integer_                                                                                                                    | replicas is the number of transformer pods.                |         | Minimum: 0 <br />Optional: \{\} <br /> |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#resourcerequirements-v1-core)_ | resources sets the container resource requests and limits. |         | Optional: \{\} <br />                  |

#### Destination

Destination is a single fan-out target. Exactly one of http or smtp must be
set.

_Appears in:_

- [RelaySpec](#relayspec)

| Field                                        | Description                                                                                                                                                                                                   | Default | Validation            |
| -------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------- | --------------------- |
| `name` _string_                              | name identifies the destination. It is used in metrics labels and must<br />be unique within the relay.                                                                                                       |         | MinLength: 1 <br />   |
| `required` _boolean_                         | required gates retry. A failure to a required destination returns SMTP<br />4xx so Postfix retries. A best-effort destination failure is logged and<br />metered without an upstream retry. Defaults to true. | true    | Optional: \{\} <br /> |
| `http` _[HTTPDestination](#httpdestination)_ | http delivers via an HTTP request.                                                                                                                                                                            |         | Optional: \{\} <br /> |
| `smtp` _[SMTPDestination](#smtpdestination)_ | smtp delivers to a downstream SMTP server.                                                                                                                                                                    |         | Optional: \{\} <br /> |

#### Filters

Filters declares inbound validation applied before a message is forwarded.

_Appears in:_

- [RelaySpec](#relayspec)

| Field                                              | Description                                                                                      | Default | Validation                                                                                           |
| -------------------------------------------------- | ------------------------------------------------------------------------------------------------ | ------- | ---------------------------------------------------------------------------------------------------- |
| `maxMessageBytes` _integer_                        | maxMessageBytes rejects messages larger than this size with SMTP 552.                            |         | Optional: \{\} <br />                                                                                |
| `allowedSenderDomains` _string array_              | allowedSenderDomains restricts the accepted envelope sender domains.                             |         | Optional: \{\} <br />                                                                                |
| `requireDKIM` _string array_                       | requireDKIM requires a passing DKIM signature whose d= matches one of<br />these domains.        |         | Optional: \{\} <br />                                                                                |
| `minScore` _integer_                               | minScore accepts a message when its heuristic score is greater than or<br />equal to this value. |         | Optional: \{\} <br />                                                                                |
| `scoreSignals` _[ScoreSignal](#scoresignal) array_ | scoreSignals lists the named checks summed into the heuristic score.                             |         | Enum: [fromDomain messageIdDomain dkimDomain authResults bodyLinkDomain] <br />Optional: \{\} <br /> |

#### HTTPDestination

HTTPDestination delivers a message to an HTTP endpoint.

_Appears in:_

- [Destination](#destination)

| Field                                             | Description                                                                                                  | Default | Validation                                   |
| ------------------------------------------------- | ------------------------------------------------------------------------------------------------------------ | ------- | -------------------------------------------- |
| `url` _string_                                    | url is the endpoint that receives the request.                                                               |         | MinLength: 1 <br />                          |
| `method` _string_                                 | method is the HTTP method used for delivery.                                                                 | POST    | Enum: [POST PUT] <br />Optional: \{\} <br /> |
| `payloadFormat` _[PayloadFormat](#payloadformat)_ | payloadFormat selects the request body. json emits the canonical<br />envelope and raw emits message/rfc822. | json    | Enum: [json raw] <br />Optional: \{\} <br /> |
| `authSecretRef` _[SecretKeyRef](#secretkeyref)_   | authSecretRef supplies the bearer token sent in the Authorization header.                                    |         | Optional: \{\} <br />                        |
| `transform` _[Transform](#transform)_             | transform optionally remaps the canonical envelope with Jsonnet.                                             |         | Optional: \{\} <br />                        |

#### IdempotencyMode

_Underlying type:_ _string_

IdempotencyMode selects how the idempotency key is derived.

_Validation:_

- Enum: [messageId sha256]

_Appears in:_

- [RelaySpec](#relayspec)

| Field       | Description                                                    |
| ----------- | -------------------------------------------------------------- |
| `messageId` | IdempotencyMessageID uses the message Message-ID header.<br /> |
| `sha256`    | IdempotencySHA256 uses a SHA-256 digest of the message.<br />  |

#### PayloadFormat

_Underlying type:_ _string_

PayloadFormat selects the HTTP request body format.

_Validation:_

- Enum: [json raw]

_Appears in:_

- [HTTPDestination](#httpdestination)

| Field  | Description                                                      |
| ------ | ---------------------------------------------------------------- |
| `json` | PayloadFormatJSON emits the canonical JSON envelope.<br />       |
| `raw`  | PayloadFormatRaw emits the full message as message/rfc822.<br /> |

#### Relay

Relay is the Schema for the relays API.

_Appears in:_

- [RelayList](#relaylist)

| Field                                                                                                              | Description                                                     | Default | Validation |
| ------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------- | ------- | ---------- |
| `apiVersion` _string_                                                                                              | `iris.philprime.dev/v1alpha1`                                   |         |            |
| `kind` _string_                                                                                                    | `Relay`                                                         |         |            |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |         |            |
| `spec` _[RelaySpec](#relayspec)_                                                                                   |                                                                 |         |            |

#### RelayList

RelayList contains a list of Relay.

| Field                                                                                                          | Description                                                     | Default | Validation |
| -------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------- | ------- | ---------- |
| `apiVersion` _string_                                                                                          | `iris.philprime.dev/v1alpha1`                                   |         |            |
| `kind` _string_                                                                                                | `RelayList`                                                     |         |            |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |         |            |
| `items` _[Relay](#relay) array_                                                                                |                                                                 |         |            |

#### RelaySpec

RelaySpec defines the desired state of a Relay. One Relay compiles into a
set of Postfix routes plus a single transformer Deployment and Service.

_Appears in:_

- [Relay](#relay)

| Field                                               | Description                                                                                                                                        | Default   | Validation                                           |
| --------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- | --------- | ---------------------------------------------------- |
| `routes` _[Route](#route) array_                    | routes declares the recipient addresses and domains this relay claims.<br />They are compiled into the Postfix transport and relay_recipient_maps. |           | MinItems: 1 <br />                                   |
| `filters` _[Filters](#filters)_                     | filters optionally rejects inbound mail with an SMTP 5xx before the<br />message is transformed and delivered.                                     |           | Optional: \{\} <br />                                |
| `idempotency` _[IdempotencyMode](#idempotencymode)_ | idempotency selects the stable key sent to every destination so<br />downstreams can deduplicate redelivered messages.                             | messageId | Enum: [messageId sha256] <br />Optional: \{\} <br /> |
| `destinations` _[Destination](#destination) array_  | destinations receive every accepted message. Fan-out is a broadcast to<br />all destinations.                                                      |           | MinItems: 1 <br />                                   |
| `deployment` _[DeploymentSpec](#deploymentspec)_    | deployment tunes the per-relay transformer Deployment. Sensible<br />defaults apply when it is omitted.                                            |           | Optional: \{\} <br />                                |

#### Route

Route claims either an exact recipient address or a whole domain. Exactly
one of address or domain must be set. An exact address wins over a domain
route (Postfix transport semantics).

_Appears in:_

- [RelaySpec](#relayspec)

| Field              | Description                                                                         | Default | Validation            |
| ------------------ | ----------------------------------------------------------------------------------- | ------- | --------------------- |
| `address` _string_ | address is an exact recipient address, for example<br />invites@invite.example.com. |         | Optional: \{\} <br /> |
| `domain` _string_  | domain matches any local-part on the domain, for example<br />invite.example.com.   |         | Optional: \{\} <br /> |

#### SMTPDestination

SMTPDestination delivers a message to a downstream SMTP server. Pointing
this at another relay's Service is how manual relay-to-relay chaining works.

_Appears in:_

- [Destination](#destination)

| Field                                           | Description                                               | Default | Validation                             |
| ----------------------------------------------- | --------------------------------------------------------- | ------- | -------------------------------------- |
| `host` _string_                                 | host is the downstream SMTP host.                         |         | MinLength: 1 <br />                    |
| `port` _integer_                                | port is the downstream SMTP port.                         |         | Maximum: 65535 <br />Minimum: 1 <br /> |
| `startTLS` _boolean_                            | startTLS upgrades the connection with STARTTLS when true. |         | Optional: \{\} <br />                  |
| `authSecretRef` _[SecretKeyRef](#secretkeyref)_ | authSecretRef supplies SMTP AUTH credentials.             |         | Optional: \{\} <br />                  |

#### ScoreSignal

_Underlying type:_ _string_

ScoreSignal names a reusable heuristic check contributing to the score.

_Validation:_

- Enum: [fromDomain messageIdDomain dkimDomain authResults bodyLinkDomain]

_Appears in:_

- [Filters](#filters)

| Field             | Description                                                                                                    |
| ----------------- | -------------------------------------------------------------------------------------------------------------- |
| `fromDomain`      | ScoreSignalFromDomain matches when the From header domain is allowed.<br />                                    |
| `messageIdDomain` | ScoreSignalMessageIDDomain matches when the Message-ID domain is allowed.<br />                                |
| `dkimDomain`      | ScoreSignalDKIMDomain matches when the DKIM d= domain is allowed.<br />                                        |
| `authResults`     | ScoreSignalAuthResults matches when Authentication-Results shows a DKIM<br />pass for an allowed domain.<br /> |
| `bodyLinkDomain`  | ScoreSignalBodyLinkDomain matches when the body links to an allowed domain.<br />                              |

#### SecretKeyRef

SecretKeyRef references a single key in a Secret in the Relay's namespace.

_Appears in:_

- [HTTPDestination](#httpdestination)
- [SMTPDestination](#smtpdestination)

| Field           | Description                       | Default | Validation          |
| --------------- | --------------------------------- | ------- | ------------------- |
| `name` _string_ | name is the Secret name.          |         | MinLength: 1 <br /> |
| `key` _string_  | key is the key within the Secret. |         | MinLength: 1 <br /> |

#### ServiceReference

ServiceReference names a Service in the Relay's namespace.

_Appears in:_

- [RelayStatus](#relaystatus)

| Field           | Description               | Default | Validation |
| --------------- | ------------------------- | ------- | ---------- |
| `name` _string_ | name is the Service name. |         |            |

#### Transform

Transform references a Jsonnet program that remaps the canonical envelope
into a destination's own schema.

_Appears in:_

- [HTTPDestination](#httpdestination)

| Field                                                       | Description                                                                        | Default | Validation |
| ----------------------------------------------------------- | ---------------------------------------------------------------------------------- | ------- | ---------- |
| `jsonnetConfigMapRef` _[ConfigMapKeyRef](#configmapkeyref)_ | jsonnetConfigMapRef references the ConfigMap key holding the Jsonnet<br />program. |         |            |
