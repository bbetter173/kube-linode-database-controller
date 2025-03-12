{{/*
Expand the name of the chart.
*/}}
{{- define "linode-db-allowlist.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "linode-db-allowlist.fullname" -}}
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
{{- define "linode-db-allowlist.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "linode-db-allowlist.labels" -}}
helm.sh/chart: {{ include "linode-db-allowlist.chart" . }}
{{ include "linode-db-allowlist.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "linode-db-allowlist.selectorLabels" -}}
app.kubernetes.io/name: {{ include "linode-db-allowlist.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "linode-db-allowlist.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "linode-db-allowlist.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Get the Linode Token value from either an existing secret or from the values
*/}}
{{- define "linode-db-allowlist.linodeToken" -}}
{{- if .Values.linode.token.existingSecret }}
valueFrom:
  secretKeyRef:
    name: {{ .Values.linode.token.existingSecret }}
    key: {{ .Values.linode.token.secretKey }}
{{- else }}
value: {{ .Values.linode.token.value | quote }}
{{- end }}
{{- end }} 