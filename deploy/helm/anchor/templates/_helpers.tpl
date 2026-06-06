{{- define "anchor.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "anchor.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name (include "anchor.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "anchor.labels" -}}
app.kubernetes.io/name: {{ include "anchor.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end -}}

{{- define "anchor.selectorLabels" -}}
app.kubernetes.io/name: {{ include "anchor.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "anchor.mongoHost" -}}
{{- printf "%s-mongodb" (include "anchor.fullname" .) -}}
{{- end -}}

{{- define "anchor.redisHost" -}}
{{- printf "%s-redis" (include "anchor.fullname" .) -}}
{{- end -}}

{{- define "anchor.identitySecret" -}}
{{- if .Values.fabric.identity.existingSecret -}}
{{- .Values.fabric.identity.existingSecret -}}
{{- else -}}
{{- printf "%s-fabric-identity" (include "anchor.fullname" .) -}}
{{- end -}}
{{- end -}}
