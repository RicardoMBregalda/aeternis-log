{{- define "aeternislog.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "aeternislog.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name (include "aeternislog.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "aeternislog.labels" -}}
app.kubernetes.io/name: {{ include "aeternislog.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end -}}

{{- define "aeternislog.selectorLabels" -}}
app.kubernetes.io/name: {{ include "aeternislog.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "aeternislog.mongoHost" -}}
{{- printf "%s-mongodb" (include "aeternislog.fullname" .) -}}
{{- end -}}

{{- define "aeternislog.redisHost" -}}
{{- printf "%s-redis" (include "aeternislog.fullname" .) -}}
{{- end -}}

{{- define "aeternislog.identitySecret" -}}
{{- if .Values.fabric.identity.existingSecret -}}
{{- .Values.fabric.identity.existingSecret -}}
{{- else -}}
{{- printf "%s-fabric-identity" (include "aeternislog.fullname" .) -}}
{{- end -}}
{{- end -}}

{{/* Name of the Secret holding API auth keys and the webhook secret. */}}
{{- define "aeternislog.appSecret" -}}
{{- printf "%s-secrets" (include "aeternislog.fullname" .) -}}
{{- end -}}

{{/* Render auth.tenants as the AUTH_TENANTS env format: "id:k1,k2;id2:k3". */}}
{{- define "aeternislog.tenantsEnv" -}}
{{- $parts := list -}}
{{- range .Values.auth.tenants -}}
{{- $parts = append $parts (printf "%s:%s" .id (join "," .keys)) -}}
{{- end -}}
{{- join ";" $parts -}}
{{- end -}}
