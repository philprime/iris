{{/* Chart name, overridable. */}}
{{- define "iris.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* Fully qualified app name. */}}
{{- define "iris.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name (include "iris.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{/* Common labels. */}}
{{- define "iris.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/name: {{ include "iris.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/* Controller selector labels. */}}
{{- define "iris.controller.selectorLabels" -}}
app.kubernetes.io/name: {{ include "iris.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: controller
{{- end -}}

{{/* Postfix selector labels. */}}
{{- define "iris.postfix.selectorLabels" -}}
app.kubernetes.io/name: {{ include "iris.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: postfix
{{- end -}}

{{/* Controller service account name. */}}
{{- define "iris.serviceAccountName" -}}
{{- printf "%s-controller" (include "iris.fullname" .) -}}
{{- end -}}

{{/* Controller image with a tag defaulting to the chart appVersion. */}}
{{- define "iris.controller.image" -}}
{{- $tag := .Values.controller.image.tag | default .Chart.AppVersion -}}
{{- printf "%s:%s" .Values.controller.image.repository $tag -}}
{{- end -}}

{{/* Relay image the controller reconciles, tag defaulting to the chart appVersion. */}}
{{- define "iris.relay.image" -}}
{{- $tag := .Values.controller.relayImage.tag | default .Chart.AppVersion -}}
{{- printf "%s:%s" .Values.controller.relayImage.repository $tag -}}
{{- end -}}

{{/* Postfix image with a tag defaulting to the chart appVersion. */}}
{{- define "iris.postfix.image" -}}
{{- $tag := .Values.postfix.image.tag | default .Chart.AppVersion -}}
{{- printf "%s:%s" .Values.postfix.image.repository $tag -}}
{{- end -}}

{{/* Names of the shared objects the controller and Postfix tier reference. */}}
{{- define "iris.postfixMapsConfigMap" -}}
{{- printf "%s-postfix-maps" (include "iris.fullname" .) -}}
{{- end -}}

{{- define "iris.webhookService" -}}
{{- printf "%s-webhook" (include "iris.fullname" .) -}}
{{- end -}}

{{/* Secret cert-manager fills with the Postfix STARTTLS serving certificate. */}}
{{- define "iris.postfixTLSSecret" -}}
{{- .Values.postfix.tls.secretName | default (printf "%s-postfix-tls" (include "iris.fullname" .)) -}}
{{- end -}}

{{/* Sentry env block, included by each component's container. */}}
{{- define "iris.sentryEnv" -}}
- name: IRIS_SENTRY_ENABLED
  value: {{ .Values.sentry.enabled | quote }}
{{- if .Values.sentry.enabled }}
- name: IRIS_SENTRY_DSN
  value: {{ .Values.sentry.dsn | quote }}
- name: IRIS_SENTRY_ENVIRONMENT
  value: {{ .Values.sentry.environment | quote }}
{{- end }}
{{- end -}}
