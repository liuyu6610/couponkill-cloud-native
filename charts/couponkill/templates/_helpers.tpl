{{/*
目标命名空间：values.namespace 优先，否则 helm --namespace。
*/}}
{{- define "couponkill.ns" -}}
{{- default .Release.Namespace .Values.namespace -}}
{{- end -}}

{{/*
业务服务键（渲染顺序固定）。
*/}}
{{- define "couponkill.serviceKeys" -}}
gateway user coupon order connector
{{- end -}}

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

{{/*
Secret 键名。用法：include "couponkill.secretKey" (list . "jwtSecret")
*/}}
{{- define "couponkill.secretKey" -}}
{{- $root := index . 0 -}}
{{- $k := index . 1 -}}
{{- $defaults := dict "jwtSecret" "jwt-secret" "internalToken" "internal-token" "gatewaySignKey" "gateway-sign-key" "postgresPassword" "postgres-password" "pgDsn" "pg-dsn" "redisPassword" "redis-password" -}}
{{- index (($root.Values.secrets).keys | default dict) $k | default (index $defaults $k) -}}
{{- end -}}

{{/*
infra / 初始化 Job / 依赖 StatefulSet 共用的 ServiceAccount。
*/}}
{{- define "couponkill.infraServiceAccountName" -}}
{{- $sa := .Values.serviceAccountInfra | default dict -}}
{{- $sa.name | default "couponkill-infra" -}}
{{- end -}}

{{/*
每个服务需要哪些配置组（与 internal/platform/config 的组对应）。
  pg 数据库  redis 缓存/会话/锁  kafka 消息  jwt 登录令牌  sign 网关身份签名密钥
  internal 内部调用令牌  peers 其他服务 URL  seed 演示数据开关  platforms 电商平台凭证
与各服务 Config 内嵌的 platform/config 组一一对应（user 不内嵌 Peers，它不调用其他服务）。
最小权限：不需要的机密不注入（gateway 没有数据库口令，coupon 没有 JWT 密钥……）。
*/}}
{{- define "couponkill.caps" -}}
{{- $m := dict
  "gateway" "redis jwt sign peers"
  "user" "pg redis jwt sign internal seed"
  "coupon" "pg redis sign internal peers seed"
  "order" "pg redis kafka sign internal peers"
  "connector" "pg redis sign internal peers platforms" -}}
{{- index $m . -}}
{{- end -}}

{{/*
镜像引用。用法：include "couponkill.image" (list . "order") 或 (list . "order" "canaryTag")
services.<svc>.image.repository 非空时视为完整仓库地址（含 registry），否则 <registry>/<prefix><svc>。
*/}}
{{- define "couponkill.image" -}}
{{- $root := index . 0 -}}
{{- $key := index . 1 -}}
{{- $svc := index $root.Values.services $key -}}
{{- $img := $svc.image | default dict -}}
{{- $tag := $img.tag | default $root.Values.image.tag | default "latest" | toString -}}
{{- if gt (len .) 2 -}}{{- $tag = index . 2 | toString -}}{{- end -}}
{{- $reg := trimSuffix "/" ($root.Values.image.registry | default "") -}}
{{- if $img.repository -}}
{{- printf "%s:%s" $img.repository $tag -}}
{{- else if $reg -}}
{{- printf "%s/%s%s:%s" $reg $root.Values.image.repositoryPrefix $key $tag -}}
{{- else -}}
{{- printf "%s%s:%s" $root.Values.image.repositoryPrefix $key $tag -}}
{{- end -}}
{{- end -}}

{{/*
拉取策略。用法：include "couponkill.pullPolicy" (list . "order" "tagOverride?")
*/}}
{{- define "couponkill.pullPolicy" -}}
{{- $root := index . 0 -}}
{{- $key := index . 1 -}}
{{- $svc := index $root.Values.services $key -}}
{{- $img := $svc.image | default dict -}}
{{- $tag := $img.tag | default $root.Values.image.tag | default "latest" | toString -}}
{{- if gt (len .) 2 -}}{{- $tag = index . 2 | toString -}}{{- end -}}
{{- $root.Values.image.pullPolicy | default (ternary "Always" "IfNotPresent" (eq $tag "latest")) -}}
{{- end -}}

{{/*
通用标签。用法：include "couponkill.labels" (list . "order" "stable")
*/}}
{{- define "couponkill.labels" -}}
{{- $root := index . 0 -}}
{{- $key := index . 1 -}}
{{- $svc := index $root.Values.services $key -}}
app: {{ $svc.name }}
version: {{ index . 2 }}
app.kubernetes.io/name: {{ $svc.name }}
app.kubernetes.io/instance: {{ $root.Release.Name }}
app.kubernetes.io/component: {{ $key }}
app.kubernetes.io/part-of: couponkill
app.kubernetes.io/managed-by: {{ $root.Release.Service }}
helm.sh/chart: {{ printf "%s-%s" $root.Chart.Name $root.Chart.Version | replace "+" "_" }}
{{- end -}}

{{/*
基础设施工作负载（PG / Redis / Kafka / Job）标签。用法：include "couponkill.infraLabels" (list . "postgres")
*/}}
{{- define "couponkill.infraLabels" -}}
{{- $root := index . 0 -}}
app: {{ index . 1 }}
app.kubernetes.io/name: {{ index . 1 }}
app.kubernetes.io/instance: {{ $root.Release.Name }}
app.kubernetes.io/component: {{ index . 1 }}
app.kubernetes.io/part-of: couponkill
app.kubernetes.io/managed-by: {{ $root.Release.Service }}
{{- end -}}

{{/* 服务的集群内基础 URL（Service 80 端口，FQDN 避免 ndots 搜索路径的额外查询）。 */}}
{{- define "couponkill.peerURL" -}}
{{- $root := index . 0 -}}
{{- $svc := index $root.Values.services (index . 1) -}}
{{- printf "http://%s.%s.svc.cluster.local" $svc.name (include "couponkill.ns" $root) -}}
{{- end -}}

{{/* Redis 地址 host:port。 */}}
{{- define "couponkill.redisAddr" -}}
{{- if .Values.redis.enabled -}}
{{- printf "%s:%d" .Values.redis.name (int .Values.redis.port) -}}
{{- else -}}
{{- printf "%s:%d" (required "redis.enabled=false 时必须设置 redis.host" .Values.redis.host) (int .Values.redis.port) -}}
{{- end -}}
{{- end -}}

{{/* PostgreSQL 主机名。 */}}
{{- define "couponkill.pgHost" -}}
{{- if .Values.postgres.enabled -}}
{{- .Values.postgres.name -}}
{{- else -}}
{{- required "postgres.enabled=false 时必须设置 postgres.host" .Values.postgres.host -}}
{{- end -}}
{{- end -}}

{{/* Kafka bootstrap 地址（逗号分隔）。 */}}
{{- define "couponkill.kafkaBrokers" -}}
{{- if .Values.kafka.enabled -}}
{{- printf "%s:%d" .Values.kafka.name (int .Values.kafka.port) -}}
{{- else -}}
{{- required "kafka.enabled=false 时必须设置 kafka.brokers" .Values.kafka.brokers -}}
{{- end -}}
{{- end -}}

{{/*
服务的 ConfigMap 数据（YAML 映射）。ConfigMap 与 Deployment 的 checksum 注解共用，保证配置变化会滚动重启。
用法：include "couponkill.configData" (list . "order")
*/}}
{{- define "couponkill.configData" -}}
{{- $root := index . 0 -}}
{{- $key := index . 1 -}}
{{- $svc := index $root.Values.services $key -}}
{{- $caps := splitList " " (include "couponkill.caps" $key) -}}
{{- $d := dict "CK_ENV" $root.Values.config.env "CK_HTTP_PORT" (int $svc.port | toString) "CK_LOG_LEVEL" $root.Values.config.logLevel -}}
{{- if has "redis" $caps -}}
{{- $_ := set $d "CK_REDIS_ADDR" (include "couponkill.redisAddr" $root) -}}
{{- $_ := set $d "CK_REDIS_DB" (int $root.Values.redis.db | toString) -}}
{{- end -}}
{{- if has "kafka" $caps -}}
{{- $_ := set $d "CK_KAFKA_BROKERS" (include "couponkill.kafkaBrokers" $root) -}}
{{- end -}}
{{- if has "jwt" $caps -}}
{{- $_ := set $d "CK_JWT_ISSUER" $root.Values.config.jwt.issuer -}}
{{- $_ := set $d "CK_JWT_AUDIENCE" $root.Values.config.jwt.audience -}}
{{- $_ := set $d "CK_JWT_TTL" $root.Values.config.jwt.ttl -}}
{{- end -}}
{{- if has "peers" $caps -}}
{{- $_ := set $d "CK_USER_URL" (include "couponkill.peerURL" (list $root "user")) -}}
{{- $_ := set $d "CK_COUPON_URL" (include "couponkill.peerURL" (list $root "coupon")) -}}
{{- $_ := set $d "CK_ORDER_URL" (include "couponkill.peerURL" (list $root "order")) -}}
{{- $_ := set $d "CK_CONNECTOR_URL" (include "couponkill.peerURL" (list $root "connector")) -}}
{{- end -}}
{{- if has "seed" $caps -}}
{{- $_ := set $d "CK_SEED_DEMO" ($root.Values.config.seedDemo | toString) -}}
{{- end -}}
{{- range $k, $v := ($svc.config | default dict) -}}
{{- if ne (toString $v) "" -}}
{{- $_ := set $d $k (toString $v) -}}
{{- end -}}
{{- end -}}
{{- toYaml $d -}}
{{- end -}}

{{/*
secretKeyRef 环境变量项。用法：include "couponkill.secretEnv" (list . "CK_JWT_SECRET" "jwtSecret")
*/}}
{{- define "couponkill.secretEnv" -}}
- name: {{ index . 1 }}
  valueFrom:
    secretKeyRef:
      name: {{ include "couponkill.appSecretName" (index . 0) }}
      key: {{ include "couponkill.secretKey" (list (index . 0) (index . 2)) }}
{{- end -}}

{{/*
服务容器的机密环境变量（按 caps 注入）。用法：include "couponkill.serviceSecretEnv" (list . "order")
*/}}
{{- define "couponkill.serviceSecretEnv" -}}
{{- $root := index . 0 -}}
{{- $key := index . 1 -}}
{{- $svc := index $root.Values.services $key -}}
{{- $caps := splitList " " (include "couponkill.caps" $key) -}}
{{- if has "pg" $caps }}
{{- if $root.Values.postgres.dsnFromSecret }}
{{ include "couponkill.secretEnv" (list $root "CK_PG_DSN" "pgDsn") }}
{{- else }}
{{ include "couponkill.secretEnv" (list $root "CK_PG_PASSWORD" "postgresPassword") }}
- name: CK_PG_DSN
  value: {{ printf "postgres://%s:$(CK_PG_PASSWORD)@%s:%d/%s?sslmode=%s" $root.Values.postgres.username (include "couponkill.pgHost" $root) (int $root.Values.postgres.port) $root.Values.postgres.database $root.Values.postgres.sslmode | quote }}
{{- end }}
{{- end }}
{{- if and (has "redis" $caps) $root.Values.redis.auth.enabled }}
{{ include "couponkill.secretEnv" (list $root "CK_REDIS_PASSWORD" "redisPassword") }}
{{- end }}
{{- if has "jwt" $caps }}
{{ include "couponkill.secretEnv" (list $root "CK_JWT_SECRET" "jwtSecret") }}
{{- end }}
{{- if has "sign" $caps }}
{{ include "couponkill.secretEnv" (list $root "CK_GATEWAY_SIGN_KEY" "gatewaySignKey") }}
{{- end }}
{{- if has "internal" $caps }}
{{ include "couponkill.secretEnv" (list $root "CK_INTERNAL_TOKEN" "internalToken") }}
{{- end }}
{{- if has "platforms" $caps }}
{{- range $e := ($svc.secretEnv | default list) }}
- name: {{ $e.env }}
  valueFrom:
    secretKeyRef:
      name: {{ include "couponkill.appSecretName" $root }}
      key: {{ $e.key }}
      optional: true
{{- end }}
{{- end }}
{{- end -}}

{{/*
渲染前的一致性校验：把「配置不安全 / 前后矛盾」挡在 helm 阶段，而不是上线后才发现。
*/}}
{{- define "couponkill.validate" -}}
{{- if not (has .Values.config.env (list "dev" "prod")) -}}
{{- fail (printf "config.env 只能是 dev 或 prod，当前为 %q" .Values.config.env) -}}
{{- end -}}
{{- if and (eq .Values.config.env "prod") .Values.config.seedDemo -}}
{{- fail "config.seedDemo=true 不能与 config.env=prod 同时使用（演示账号口令是公开的测试值）" -}}
{{- end -}}
{{- if and (eq .Values.config.env "prod") .Values.secrets.create -}}
{{- fail "config.env=prod 时必须 secrets.create=false 并预先创建 Secret（见 README），chart 不为生产生成密钥" -}}
{{- end -}}
{{- if and (eq .Values.config.env "prod") (eq (.Values.image.tag | default "latest" | toString) "latest") -}}
{{- fail "config.env=prod 时必须用 --set image.tag=<构建号> 指定镜像标签，生产不使用 latest" -}}
{{- end -}}
{{- if and .Values.keda.enabled (gt (int .Values.keda.order.maxReplicaCount) (int .Values.kafka.topics.seckillOrderCreate.partitions)) -}}
{{- fail (printf "keda.order.maxReplicaCount(%d) 不得超过 kafka.topics.seckillOrderCreate.partitions(%d)：多出来的消费者分不到分区" (int .Values.keda.order.maxReplicaCount) (int .Values.kafka.topics.seckillOrderCreate.partitions)) -}}
{{- end -}}
{{- if and .Values.istio.enabled .Values.ingress.enabled -}}
{{- fail "istio.enabled 与 ingress.enabled 只能二选一，入口只保留一条路径" -}}
{{- end -}}
{{- range $svc := list "coupon" "order" -}}
{{- $c := index $.Values.canary $svc | default dict -}}
{{- if $c.enabled -}}
{{- $w := int ($c.weightCanary | default 0) -}}
{{- if or (lt $w 0) (gt $w 100) -}}
{{- fail (printf "canary.%s.weightCanary 必须在 0-100 之间，当前为 %d" $svc $w) -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/* Operator 镜像：<registry>/<prefix>operator:<tag>（operator.image.repository / tag 可覆盖）。 */}}
{{- define "couponkill.operatorImage" -}}
{{- $img := .Values.operator.image | default dict -}}
{{- $tag := $img.tag | default .Values.image.tag | default "latest" | toString -}}
{{- $reg := trimSuffix "/" (.Values.image.registry | default "") -}}
{{- if $img.repository -}}
{{- printf "%s:%s" $img.repository $tag -}}
{{- else if $reg -}}
{{- printf "%s/%soperator:%s" $reg .Values.image.repositoryPrefix $tag -}}
{{- else -}}
{{- printf "%soperator:%s" .Values.image.repositoryPrefix $tag -}}
{{- end -}}
{{- end -}}
