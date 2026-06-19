/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RelaySpec defines the desired state of a Relay. One Relay compiles into a
// set of Postfix routes plus a single transformer Deployment and Service.
type RelaySpec struct {
	// routes declares the recipient addresses and domains this relay claims.
	// They are compiled into the Postfix transport and relay_recipient_maps.
	// +kubebuilder:validation:MinItems=1
	Routes []Route `json:"routes"`

	// filters optionally rejects inbound mail with an SMTP 5xx before the
	// message is transformed and delivered.
	// +optional
	Filters *Filters `json:"filters,omitempty"`

	// idempotency selects the stable key sent to every destination so
	// downstreams can deduplicate redelivered messages.
	// +kubebuilder:validation:Enum=messageId;sha256
	// +kubebuilder:default=messageId
	// +optional
	Idempotency IdempotencyMode `json:"idempotency,omitempty"`

	// destinations receive every accepted message. Fan-out is a broadcast to
	// all destinations.
	// +kubebuilder:validation:MinItems=1
	Destinations []Destination `json:"destinations"`

	// deployment tunes the per-relay transformer Deployment. Sensible
	// defaults apply when it is omitted.
	// +optional
	Deployment *DeploymentSpec `json:"deployment,omitempty"`
}

// Route claims either an exact recipient address or a whole domain. Exactly
// one of address or domain must be set. An exact address wins over a domain
// route (Postfix transport semantics).
// +kubebuilder:validation:XValidation:rule="(has(self.address) ? 1 : 0) + (has(self.domain) ? 1 : 0) == 1",message="exactly one of address or domain must be set"
type Route struct {
	// address is an exact recipient address, for example
	// invites@invite.example.com.
	// +optional
	Address string `json:"address,omitempty"`

	// domain matches any local-part on the domain, for example
	// invite.example.com.
	// +optional
	Domain string `json:"domain,omitempty"`
}

// Filters declares inbound validation applied before a message is forwarded.
type Filters struct {
	// maxMessageBytes rejects messages larger than this size with SMTP 552.
	// +optional
	MaxMessageBytes int64 `json:"maxMessageBytes,omitempty"`

	// allowedSenderDomains restricts the accepted envelope sender domains.
	// +optional
	AllowedSenderDomains []string `json:"allowedSenderDomains,omitempty"`

	// requireDKIM requires a passing DKIM signature whose d= matches one of
	// these domains.
	// +optional
	RequireDKIM []string `json:"requireDKIM,omitempty"`

	// minScore accepts a message when its heuristic score is greater than or
	// equal to this value.
	// +optional
	MinScore int32 `json:"minScore,omitempty"`

	// scoreSignals lists the named checks summed into the heuristic score.
	// +optional
	ScoreSignals []ScoreSignal `json:"scoreSignals,omitempty"`
}

// ScoreSignal names a reusable heuristic check contributing to the score.
// +kubebuilder:validation:Enum=fromDomain;messageIdDomain;dkimDomain;authResults;bodyLinkDomain
type ScoreSignal string

const (
	// ScoreSignalFromDomain matches when the From header domain is allowed.
	ScoreSignalFromDomain ScoreSignal = "fromDomain"
	// ScoreSignalMessageIDDomain matches when the Message-ID domain is allowed.
	ScoreSignalMessageIDDomain ScoreSignal = "messageIdDomain"
	// ScoreSignalDKIMDomain matches when a cryptographically valid DKIM
	// signature has a d= domain that is allowed.
	ScoreSignalDKIMDomain ScoreSignal = "dkimDomain"
	// ScoreSignalAuthResults matches when a cryptographically valid DKIM
	// signature has a d= domain that is allowed. It is an alias of dkimDomain
	// kept for configurations that reference upstream authentication results.
	ScoreSignalAuthResults ScoreSignal = "authResults"
	// ScoreSignalBodyLinkDomain matches when the body links to an allowed domain.
	ScoreSignalBodyLinkDomain ScoreSignal = "bodyLinkDomain"
)

// IdempotencyMode selects how the idempotency key is derived.
// +kubebuilder:validation:Enum=messageId;sha256
type IdempotencyMode string

const (
	// IdempotencyMessageID uses the message Message-ID header.
	IdempotencyMessageID IdempotencyMode = "messageId"
	// IdempotencySHA256 uses a SHA-256 digest of the message.
	IdempotencySHA256 IdempotencyMode = "sha256"
)

// Destination is a single fan-out target. Exactly one of http or smtp must be
// set.
// +kubebuilder:validation:XValidation:rule="(has(self.http) ? 1 : 0) + (has(self.smtp) ? 1 : 0) == 1",message="exactly one of http or smtp must be set"
type Destination struct {
	// name identifies the destination. It is used in metrics labels and must
	// be unique within the relay.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// required gates retry. A failure to a required destination returns SMTP
	// 4xx so Postfix retries. A best-effort destination failure is logged and
	// metered without an upstream retry. Defaults to true.
	// +kubebuilder:default=true
	// +optional
	Required *bool `json:"required,omitempty"`

	// http delivers via an HTTP request.
	// +optional
	HTTP *HTTPDestination `json:"http,omitempty"`

	// smtp delivers to a downstream SMTP server.
	// +optional
	SMTP *SMTPDestination `json:"smtp,omitempty"`
}

// HTTPDestination delivers a message to an HTTP endpoint.
type HTTPDestination struct {
	// url is the endpoint that receives the request.
	// +kubebuilder:validation:MinLength=1
	URL string `json:"url"`

	// method is the HTTP method used for delivery.
	// +kubebuilder:validation:Enum=POST;PUT
	// +kubebuilder:default=POST
	// +optional
	Method string `json:"method,omitempty"`

	// payloadFormat selects the request body. json emits the canonical
	// envelope and raw emits message/rfc822.
	// +kubebuilder:validation:Enum=json;raw
	// +kubebuilder:default=json
	// +optional
	PayloadFormat PayloadFormat `json:"payloadFormat,omitempty"`

	// authSecretRef supplies the bearer token sent in the Authorization header.
	// +optional
	AuthSecretRef *SecretKeyRef `json:"authSecretRef,omitempty"`

	// transform optionally remaps the canonical envelope with Jsonnet.
	// +optional
	Transform *Transform `json:"transform,omitempty"`
}

// PayloadFormat selects the HTTP request body format.
// +kubebuilder:validation:Enum=json;raw
type PayloadFormat string

const (
	// PayloadFormatJSON emits the canonical JSON envelope.
	PayloadFormatJSON PayloadFormat = "json"
	// PayloadFormatRaw emits the full message as message/rfc822.
	PayloadFormatRaw PayloadFormat = "raw"
)

// SMTPDestination delivers a message to a downstream SMTP server. Pointing
// this at another relay's Service is how manual relay-to-relay chaining works.
type SMTPDestination struct {
	// host is the downstream SMTP host.
	// +kubebuilder:validation:MinLength=1
	Host string `json:"host"`

	// port is the downstream SMTP port.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`

	// startTLS upgrades the connection with STARTTLS when true.
	// +optional
	StartTLS bool `json:"startTLS,omitempty"`

	// authSecretRef supplies SMTP AUTH credentials.
	// +optional
	AuthSecretRef *SecretKeyRef `json:"authSecretRef,omitempty"`
}

// Transform references a Jsonnet program that remaps the canonical envelope
// into a destination's own schema.
type Transform struct {
	// jsonnetConfigMapRef references the ConfigMap key holding the Jsonnet
	// program.
	JsonnetConfigMapRef ConfigMapKeyRef `json:"jsonnetConfigMapRef"`
}

// SecretKeyRef references a single key in a Secret in the Relay's namespace.
type SecretKeyRef struct {
	// name is the Secret name.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// key is the key within the Secret.
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`
}

// ConfigMapKeyRef references a single key in a ConfigMap in the Relay's
// namespace.
type ConfigMapKeyRef struct {
	// name is the ConfigMap name.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// key is the key within the ConfigMap.
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`
}

// DeploymentSpec tunes the per-relay transformer Deployment.
type DeploymentSpec struct {
	// replicas is the number of transformer pods.
	// +kubebuilder:validation:Minimum=0
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// resources sets the container resource requests and limits.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// RelayStatus reports the observed state of a Relay.
type RelayStatus struct {
	// conditions holds the Kstatus-style conditions (Ready, Programmed,
	// Conflict).
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// observedGeneration is the generation last reconciled by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// claimedRoutes lists the route keys this relay currently owns in the
	// aggregate Postfix configuration.
	// +optional
	ClaimedRoutes []string `json:"claimedRoutes,omitempty"`

	// serviceRef references the transformer Service reconciled for this relay.
	// +optional
	ServiceRef *ServiceReference `json:"serviceRef,omitempty"`
}

// ServiceReference names a Service in the Relay's namespace.
type ServiceReference struct {
	// name is the Service name.
	Name string `json:"name"`
}

// Relay is the Schema for the relays API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=relay
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Programmed",type=string,JSONPath=`.status.conditions[?(@.type=="Programmed")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type Relay struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RelaySpec   `json:"spec,omitempty"`
	Status RelayStatus `json:"status,omitempty"`
}

// RelayList contains a list of Relay.
// +kubebuilder:object:root=true
type RelayList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Relay `json:"items"`
}
