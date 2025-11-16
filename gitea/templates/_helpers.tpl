{{- define "gitea.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "gitea.fullname" -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "gitea.labels" -}}
app.kubernetes.io/name: {{ include "gitea.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | trunc 63 | trimSuffix "-" }}
{{- end -}}

{{- define "gitea.selectorLabels" -}}
app.kubernetes.io/name: {{ include "gitea.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "gitea.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "gitea.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "gitea.adminSecretName" -}}
{{- if .Values.admin.secretName -}}
{{ .Values.admin.secretName -}}
{{- else -}}
{{ printf "%s-admin-cert" (include "gitea.fullname" .) -}}
{{- end -}}
{{- end -}}

{{- define "gitea.boolStr" -}}
{{- if . }}true{{ else }}false{{ end -}}
{{- end -}}

{{- define "gitea.disabledRepoUnits" -}}
{{- $units := list -}}
{{- if not .Values.features.issues -}}
{{- $units = append $units "repo.issues" -}}
{{- $units = append $units "repo.ext_issues" -}}
{{- end -}}
{{- if not .Values.features.pulls -}}
{{- $units = append $units "repo.pulls" -}}
{{- end -}}
{{- if not .Values.features.wiki -}}
{{- $units = append $units "repo.wiki" -}}
{{- $units = append $units "repo.ext_wiki" -}}
{{- end -}}
{{- if not .Values.features.projects -}}
{{- $units = append $units "repo.projects" -}}
{{- end -}}
{{- if not .Values.features.packages -}}
{{- $units = append $units "repo.packages" -}}
{{- end -}}
{{- if not .Values.features.actions -}}
{{- $units = append $units "repo.actions" -}}
{{- end -}}
{{- join "," (uniq $units) -}}
{{- end -}}

{{- define "gitea.defaultRepoUnits" -}}
{{- $units := list "repo.code" "repo.releases" -}}
{{- if .Values.features.issues -}}
{{- $units = append $units "repo.issues" -}}
{{- end -}}
{{- if .Values.features.pulls -}}
{{- $units = append $units "repo.pulls" -}}
{{- end -}}
{{- if .Values.features.wiki -}}
{{- $units = append $units "repo.wiki" -}}
{{- end -}}
{{- if .Values.features.projects -}}
{{- $units = append $units "repo.projects" -}}
{{- end -}}
{{- if .Values.features.packages -}}
{{- $units = append $units "repo.packages" -}}
{{- end -}}
{{- if .Values.features.actions -}}
{{- $units = append $units "repo.actions" -}}
{{- end -}}
{{- join "," $units -}}
{{- end -}}

{{- define "gitea.defaultForkRepoUnits" -}}
{{- $units := list "repo.code" -}}
{{- if .Values.features.pulls -}}
{{- $units = append $units "repo.pulls" -}}
{{- end -}}
{{- join "," $units -}}
{{- end -}}

{{- define "gitea.defaultMirrorRepoUnits" -}}
{{- $units := list "repo.code" "repo.releases" -}}
{{- if .Values.features.issues -}}
{{- $units = append $units "repo.issues" -}}
{{- end -}}
{{- if .Values.features.wiki -}}
{{- $units = append $units "repo.wiki" -}}
{{- end -}}
{{- if .Values.features.projects -}}
{{- $units = append $units "repo.projects" -}}
{{- end -}}
{{- if .Values.features.packages -}}
{{- $units = append $units "repo.packages" -}}
{{- end -}}
{{- join "," $units -}}
{{- end -}}

{{- define "gitea.defaultTemplateRepoUnits" -}}
{{- $units := list "repo.code" "repo.releases" -}}
{{- if .Values.features.issues -}}
{{- $units = append $units "repo.issues" -}}
{{- end -}}
{{- if .Values.features.pulls -}}
{{- $units = append $units "repo.pulls" -}}
{{- end -}}
{{- if .Values.features.wiki -}}
{{- $units = append $units "repo.wiki" -}}
{{- end -}}
{{- if .Values.features.projects -}}
{{- $units = append $units "repo.projects" -}}
{{- end -}}
{{- if .Values.features.packages -}}
{{- $units = append $units "repo.packages" -}}
{{- end -}}
{{- if .Values.features.actions -}}
{{- $units = append $units "repo.actions" -}}
{{- end -}}
{{- join "," $units -}}
{{- end -}}

{{- define "gitea.env" -}}
- name: GITEA_APP_INI
  value: /data/gitea/conf/app.ini
- name: GITEA__server__ROOT_URL
  value: {{ default (printf "http://%s:%d/" (include "gitea.fullname" .) (int .Values.service.httpPort)) .Values.env.ROOT_URL | quote }}
- name: GITEA__server__SSH_DOMAIN
  value: {{ .Values.env.SSH_DOMAIN | default (printf "%s-ssh" (include "gitea.fullname" .)) | quote }}
- name: GITEA__server__SSH_PORT
  value: "{{ .Values.env.SSH_PORT }}"
- name: GITEA__database__DB_TYPE
  value: {{ .Values.env.DB_TYPE | quote }}
- name: GITEA__database__HOST
  value: {{ .Values.env.DB_HOST | quote }}
- name: GITEA__database__NAME
  value: {{ .Values.env.DB_NAME | quote }}
- name: GITEA__database__USER
  value: {{ .Values.env.DB_USER | quote }}
- name: GITEA_DB_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ default (printf "%s-db" (include "gitea.fullname" .)) .Values.existingSecret }}
      key: db-password
- name: GITEA_ADMIN_USERNAME
  value: {{ .Values.admin.username | quote }}
- name: GITEA_ADMIN_EMAIL
  value: {{ .Values.admin.email | quote }}
- name: GITEA_ADMIN_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ include "gitea.adminSecretName" . }}
      key: {{ .Values.admin.secretKey }}
- name: GITEA__actions__ENABLED
  value: {{ include "gitea.boolStr" .Values.features.actions | quote }}
- name: GITEA__repository__DEFAULT_ENABLE_WIKI
  value: {{ include "gitea.boolStr" .Values.features.wiki | quote }}
- name: GITEA__repository__DEFAULT_ENABLE_ISSUES
  value: {{ include "gitea.boolStr" .Values.features.issues | quote }}
- name: GITEA__repository__DEFAULT_ENABLE_PULLS
  value: {{ include "gitea.boolStr" .Values.features.pulls | quote }}
- name: GITEA__repository__DEFAULT_ENABLE_PROJECTS
  value: {{ include "gitea.boolStr" .Values.features.projects | quote }}
- name: GITEA__repository__ENABLE_PUSH_CREATE_USER
  value: "false"
- name: GITEA__repository__ENABLE_PUSH_CREATE_ORG
  value: "false"
- name: GITEA__packages__ENABLED
  value: {{ include "gitea.boolStr" .Values.features.packages | quote }}
- name: GITEA__activitypub__ENABLED
  value: {{ include "gitea.boolStr" .Values.features.activities | quote }}
- name: GITEA__repository__DISABLED_REPO_UNITS
  value: {{ include "gitea.disabledRepoUnits" . | quote }}
- name: GITEA__repository__DEFAULT_REPO_UNITS
  value: {{ include "gitea.defaultRepoUnits" . | quote }}
- name: GITEA__repository__DEFAULT_FORK_REPO_UNITS
  value: {{ include "gitea.defaultForkRepoUnits" . | quote }}
- name: GITEA__repository__DEFAULT_MIRROR_REPO_UNITS
  value: {{ include "gitea.defaultMirrorRepoUnits" . | quote }}
- name: GITEA__repository__DEFAULT_TEMPLATE_REPO_UNITS
  value: {{ include "gitea.defaultTemplateRepoUnits" . | quote }}
{{- with .Values.additionalEnv }}
{{ toYaml . }}
{{- end }}
{{- end -}}
