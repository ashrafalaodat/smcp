{{- define "tekton.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "tekton.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "tekton.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace ":" "_" | replace "+" "_" -}}
{{- end -}}

{{- define "tekton.labels" -}}
app.kubernetes.io/name: {{ include "tekton.name" . }}
helm.sh/chart: {{ include "tekton.chart" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "tekton.selectorLabels" -}}
app.kubernetes.io/name: {{ include "tekton.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "tekton.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "tekton.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "tekton.componentJobName" -}}
{{- $root := index . "root" -}}
{{- $component := index . "component" -}}
{{- $suffix := index . "suffix" -}}
{{- printf "%s-%s-%s" (include "tekton.fullname" $root) $component $suffix | trunc 63 | trimSuffix "-" -}}
{{- end -}}
