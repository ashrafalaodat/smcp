{{/*
Expand the name of the chart.
*/}}
{{- define "fluxcd.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "fluxcd.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "fluxcd.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "fluxcd.labels" -}}
helm.sh/chart: {{ include "fluxcd.chart" . }}
{{ include "fluxcd.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "fluxcd.selectorLabels" -}}
app.kubernetes.io/name: {{ include "fluxcd.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Return the namespace Flux controllers should run in.
*/}}
{{- define "fluxcd.namespace" -}}
{{- if .Values.targetNamespace }}
{{- .Values.targetNamespace }}
{{- else if .Release.Namespace }}
{{- .Release.Namespace }}
{{- else -}}
flux-system
{{- end -}}
{{- end }}

{{/*
Format an image reference using repository+tag or repository@digest.
*/}}
{{- define "fluxcd.image" -}}
{{- $repository := .repository | default "" -}}
{{- $digest := .digest | default "" -}}
{{- $tag := .tag | default "" -}}
{{- if and $repository $digest -}}
{{- printf "%s@%s" $repository $digest -}}
{{- else if and $repository $tag -}}
{{- printf "%s:%s" $repository $tag -}}
{{- else -}}
{{- $repository -}}
{{- end -}}
{{- end }}
