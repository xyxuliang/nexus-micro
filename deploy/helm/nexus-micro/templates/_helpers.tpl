{{- define "nexus-micro.fullname" -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "nexus-micro.labels" -}}
app.kubernetes.io/name: {{ include "nexus-micro.fullname" . }}
app.kubernetes.io/part-of: nexus-micro
app.kubernetes.io/version: {{ .Chart.AppVersion }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end }}

{{- define "nexus-micro.selectorLabels" -}}
app: {{ include "nexus-micro.fullname" . }}
{{- end }}