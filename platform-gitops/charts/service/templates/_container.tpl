{{/*
Shared pod/container spec so the Deployment (default) and the canary Rollout
(rollout.enabled) render from ONE source and cannot drift (add-canary-deploys D2).

- "service.container" renders the single container list item (image, ports,
  resources, envFrom/env, and the gRPC/HTTP probe blocks) at base indent 0.
- "service.podSpec" renders the pod `spec` body (serviceAccountName + containers)
  at base indent 0; callers `include` it with `nindent 6` under `template.spec:`.

The output is byte-identical to the pre-canary inline Deployment pod spec.
*/}}
{{- define "service.container" -}}
- name: {{ .Values.name }}
  image: "{{ .Values.image.repo }}:{{ index .Values.images .Values.name | default .Values.defaultTag }}"
  imagePullPolicy: {{ .Values.image.pullPolicy | default "IfNotPresent" }}
  ports:
    - containerPort: {{ .Values.containerPort }}
    {{- if .Values.jwksPort }}
    - name: jwks
      containerPort: {{ .Values.jwksPort }}
    {{- end }}
  {{- with .Values.resources }}
  resources:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- if .Values.secrets.enabled }}
  envFrom:
    - secretRef:
        name: {{ .Values.name }}-env
  {{- end }}
  {{- with .Values.env }}
  env:
    {{- range $k, $v := . }}
    - name: {{ $k }}
      value: {{ $v | quote }}
    {{- end }}
  {{- end }}
  {{- if gt (int .Values.grpcPort) 0 }}
  {{- /* gRPC service: native K8s gRPC probes (readiness + liveness + startup) */}}
  readinessProbe:
    grpc:
      port: {{ .Values.grpcPort }}
    initialDelaySeconds: {{ .Values.probes.initialDelaySeconds }}
    periodSeconds: {{ .Values.probes.periodSeconds }}
    failureThreshold: {{ .Values.probes.failureThreshold }}
  livenessProbe:
    grpc:
      port: {{ .Values.grpcPort }}
    initialDelaySeconds: {{ .Values.probes.initialDelaySeconds }}
    periodSeconds: {{ .Values.probes.periodSeconds }}
    failureThreshold: {{ .Values.probes.failureThreshold }}
  startupProbe:
    grpc:
      port: {{ .Values.grpcPort }}
    initialDelaySeconds: {{ .Values.probes.initialDelaySeconds }}
    periodSeconds: {{ .Values.probes.periodSeconds }}
    failureThreshold: {{ .Values.probes.startup.failureThreshold }}
  {{- else if .Values.healthPath }}
  {{- /* HTTP service: readiness (as before) + liveness + startup */}}
  readinessProbe:
    httpGet:
      path: {{ .Values.healthPath }}
      port: {{ .Values.containerPort }}
    initialDelaySeconds: {{ .Values.probes.initialDelaySeconds }}
    periodSeconds: {{ .Values.probes.periodSeconds }}
    failureThreshold: {{ .Values.probes.failureThreshold }}
  livenessProbe:
    httpGet:
      path: {{ .Values.healthPath }}
      port: {{ .Values.containerPort }}
    initialDelaySeconds: {{ .Values.probes.initialDelaySeconds }}
    periodSeconds: {{ .Values.probes.periodSeconds }}
    failureThreshold: {{ .Values.probes.failureThreshold }}
  startupProbe:
    httpGet:
      path: {{ .Values.healthPath }}
      port: {{ .Values.containerPort }}
    initialDelaySeconds: {{ .Values.probes.initialDelaySeconds }}
    periodSeconds: {{ .Values.probes.periodSeconds }}
    failureThreshold: {{ .Values.probes.startup.failureThreshold }}
  {{- end }}
{{- end -}}

{{- define "service.podSpec" -}}
{{- if .Values.secrets.enabled -}}
serviceAccountName: {{ .Values.name }}
{{ end -}}
containers:
  {{- include "service.container" . | nindent 2 }}
{{- end -}}
