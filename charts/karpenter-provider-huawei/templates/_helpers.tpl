{{/*
Expand the name of the chart.
*/}}
{{- define "karpenter-provider-huawei.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "karpenter-provider-huawei.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "karpenter-provider-huawei.labels" -}}
app.kubernetes.io/name: karpenter-provider-huawei
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
helm.sh/chart: {{ include "karpenter-provider-huawei.chart" . }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "karpenter-provider-huawei.selectorLabels" -}}
control-plane: controller-manager
app.kubernetes.io/name: karpenter-provider-huawei
{{- end }}

{{/*
Controller image
*/}}
{{- define "karpenter-provider-huawei.controllerImage" -}}
{{- $tag := default .Chart.AppVersion .Values.image.tag -}}
{{- printf "%s:%s" .Values.image.repository $tag -}}
{{- end }}

{{/*
ServiceAccount name
*/}}
{{- define "karpenter-provider-huawei.serviceAccountName" -}}
{{- printf "%s%s" .Values.namePrefix .Values.serviceAccount.name }}
{{- end }}

{{/*
Resource name with prefix
*/}}
{{- define "karpenter-provider-huawei.fullname" -}}
{{- printf "%s%s" .Values.namePrefix "controller-manager" }}
{{- end }}
