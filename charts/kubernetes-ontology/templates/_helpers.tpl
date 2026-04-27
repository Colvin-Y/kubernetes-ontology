{{/*
Expand the chart name.
*/}}
{{- define "kubernetes-ontology.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a fully qualified app name.
*/}}
{{- define "kubernetes-ontology.fullname" -}}
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
Chart label.
*/}}
{{- define "kubernetes-ontology.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels.
*/}}
{{- define "kubernetes-ontology.labels" -}}
helm.sh/chart: {{ include "kubernetes-ontology.chart" . }}
{{ include "kubernetes-ontology.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels.
*/}}
{{- define "kubernetes-ontology.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kubernetes-ontology.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Server selector labels.
*/}}
{{- define "kubernetes-ontology.serverSelectorLabels" -}}
{{ include "kubernetes-ontology.selectorLabels" . }}
app.kubernetes.io/component: server
{{- end }}

{{/*
Viewer selector labels.
*/}}
{{- define "kubernetes-ontology.viewerSelectorLabels" -}}
{{ include "kubernetes-ontology.selectorLabels" . }}
app.kubernetes.io/component: viewer
{{- end }}

{{/*
Service account name.
*/}}
{{- define "kubernetes-ontology.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "kubernetes-ontology.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Image reference.
*/}}
{{- define "kubernetes-ontology.image" -}}
{{- $tag := default .Chart.AppVersion .Values.image.tag }}
{{- printf "%s:%s" .Values.image.repository $tag }}
{{- end }}
