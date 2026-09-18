{{/*
统一应用 Secret 名称。生产请设 secrets.existingSecret（或 ExternalSecret 同名）。
*/}}
{{- define "couponkill.appSecretName" -}}
{{- if .Values.secrets.existingSecret -}}
{{- .Values.secrets.existingSecret -}}
{{- else -}}
{{- default "couponkill-app-secrets" .Values.secrets.name -}}
{{- end -}}
{{- end -}}

{{- define "couponkill.secretKey.postgresPassword" -}}
{{- .Values.secrets.keys.postgresPassword | default "postgres-password" -}}
{{- end -}}

{{- define "couponkill.secretKey.jwtSecret" -}}
{{- .Values.secrets.keys.jwtSecret | default "jwt-secret" -}}
{{- end -}}

{{- define "couponkill.secretKey.connectorInternalToken" -}}
{{- .Values.secrets.keys.connectorInternalToken | default "internal-token" -}}
{{- end -}}

{{- define "couponkill.secretKey.nacosAuthToken" -}}
{{- .Values.secrets.keys.nacosAuthToken | default "nacos-auth-token" -}}
{{- end -}}

{{/*
Postgres 口令：注入 POSTGRES_PASSWORD + SPRING_DATASOURCE_PASSWORD（secretKeyRef）。
*/}}
{{- define "couponkill.env.postgresPassword" -}}
- name: POSTGRES_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ include "couponkill.appSecretName" . }}
      key: {{ include "couponkill.secretKey.postgresPassword" . }}
- name: SPRING_DATASOURCE_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ include "couponkill.appSecretName" . }}
      key: {{ include "couponkill.secretKey.postgresPassword" . }}
{{- end -}}

{{- define "couponkill.env.jwtSecret" -}}
- name: JWT_SECRET
  valueFrom:
    secretKeyRef:
      name: {{ include "couponkill.appSecretName" . }}
      key: {{ include "couponkill.secretKey.jwtSecret" . }}
{{- end -}}

{{- define "couponkill.env.connectorInternalToken" -}}
{{- $secretName := .Values.services.connector.internalTokenSecret.name | default "" -}}
{{- if not $secretName -}}
{{- $secretName = include "couponkill.appSecretName" . -}}
{{- end -}}
- name: CONNECTOR_INTERNAL_TOKEN
  valueFrom:
    secretKeyRef:
      name: {{ $secretName }}
      key: {{ .Values.services.connector.internalTokenSecret.key | default (include "couponkill.secretKey.connectorInternalToken" .) }}
      optional: {{ .Values.services.connector.internalTokenSecret.optional | default true }}
{{- end -}}
