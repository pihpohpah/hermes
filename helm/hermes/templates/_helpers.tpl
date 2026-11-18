{{- define "hermes.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "hermes.fullname" -}}
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

{{- define "hermes.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "hermes.labels" -}}
helm.sh/chart: {{ include "hermes.chart" . }}
app.kubernetes.io/name: {{ include "hermes.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: {{ include "hermes.name" . }}
{{- end -}}

{{- define "hermes.selectorLabels" -}}
app.kubernetes.io/name: {{ include "hermes.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "hermes.componentSelectorLabels" -}}
{{ include "hermes.selectorLabels" .root }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{- define "hermes.image" -}}
{{- printf "%s:%s" .Values.image.repository (default .Chart.AppVersion .Values.image.tag) -}}
{{- end -}}

{{- define "hermes.database.image" -}}
{{- printf "%s:%s" .Values.database.image.repository .Values.database.image.tag -}}
{{- end -}}

{{- define "hermes.configMapName" -}}
{{- printf "%s-config" (include "hermes.fullname" . | trunc 56 | trimSuffix "-") -}}
{{- end -}}

{{- define "hermes.appSecretName" -}}
{{- if .Values.secrets.existingSecret -}}
{{- .Values.secrets.existingSecret -}}
{{- else if .Values.secrets.create -}}
{{- default (printf "%s-secrets" (include "hermes.fullname" . | trunc 55 | trimSuffix "-")) .Values.secrets.name -}}
{{- end -}}
{{- end -}}

{{- define "hermes.postgresql.fullname" -}}
{{- printf "%s-postgresql" (include "hermes.fullname" . | trunc 52 | trimSuffix "-") -}}
{{- end -}}

{{- define "hermes.database.secretName" -}}
{{- default (include "hermes.postgresql.fullname" .) .Values.database.auth.existingSecret -}}
{{- end -}}

{{- define "hermes.persistence.maps.claimName" -}}
{{- default (printf "%s-maps" (include "hermes.fullname" . | trunc 58 | trimSuffix "-")) .Values.persistence.maps.existingClaim -}}
{{- end -}}

{{- define "hermes.persistence.uploads.claimName" -}}
{{- default (printf "%s-uploads" (include "hermes.fullname" . | trunc 55 | trimSuffix "-")) .Values.persistence.uploads.existingClaim -}}
{{- end -}}

{{/*
  DATABASE_URL источника:
  - database.enabled=true -> Secret встроенной БД;
  - иначе external.existingSecret;
  - иначе external.url (рендерится как обычный env, не секрет).
*/}}
{{- define "hermes.database.env" -}}
{{- if .Values.database.enabled }}
- name: DATABASE_URL
  valueFrom:
    secretKeyRef:
      name: {{ include "hermes.database.secretName" . }}
      key: {{ .Values.database.auth.urlKey | default "DATABASE_URL" }}
{{- else if .Values.database.external.existingSecret }}
- name: DATABASE_URL
  valueFrom:
    secretKeyRef:
      name: {{ .Values.database.external.existingSecret }}
      key: {{ .Values.database.external.key | default "DATABASE_URL" }}
{{- else if .Values.database.external.url }}
- name: DATABASE_URL
  value: {{ .Values.database.external.url | quote }}
{{- else }}
{{- fail "database.enabled=false: задайте database.external.existingSecret или database.external.url" }}
{{- end }}
{{- end -}}

{{- define "hermes.probes" -}}
{{- with .Values.probes.startup }}
{{- if .enabled }}
startupProbe:
  httpGet:
    path: {{ .path }}
    port: http
  periodSeconds: {{ .periodSeconds }}
  timeoutSeconds: {{ .timeoutSeconds }}
  failureThreshold: {{ .failureThreshold }}
{{- end }}
{{- end }}
{{- with .Values.probes.readiness }}
{{- if .enabled }}
readinessProbe:
  httpGet:
    path: {{ .path }}
    port: http
  periodSeconds: {{ .periodSeconds }}
  timeoutSeconds: {{ .timeoutSeconds }}
  failureThreshold: {{ .failureThreshold }}
{{- end }}
{{- end }}
{{- with .Values.probes.liveness }}
{{- if .enabled }}
livenessProbe:
  {{- if eq .type "http" }}
  httpGet:
    path: {{ .path }}
    port: http
  {{- else }}
  tcpSocket:
    port: http
  {{- end }}
  periodSeconds: {{ .periodSeconds }}
  timeoutSeconds: {{ .timeoutSeconds }}
  failureThreshold: {{ .failureThreshold }}
{{- end }}
{{- end }}
{{- end -}}
