package render

import (
	"path/filepath"
	"strings"
	"testing"
)

// serviceSpec 描述一个服务应当收到的环境变量。
type serviceSpec struct {
	key      string
	name     string
	port     string
	config   []string // 必须出现在 ConfigMap 里的变量
	secrets  []string // 必须以 secretKeyRef 注入的变量
	absent   []string // 绝不能出现的变量（最小权限）
	platform []string // internal/platform/config 里该服务内嵌的配置组
}

var peerVars = []string{"CK_USER_URL", "CK_COUPON_URL", "CK_ORDER_URL", "CK_CONNECTOR_URL"}

// serviceSpecs 是设计文档 §3 与 internal/platform/config 配置组落到各服务上的期望。
var serviceSpecs = []serviceSpec{
	{
		key: "gateway", name: "couponkill-gateway", port: "8088",
		config:   append([]string{"CK_ENV", "CK_HTTP_PORT", "CK_LOG_LEVEL", "CK_REDIS_ADDR", "CK_REDIS_DB", "CK_JWT_ISSUER", "CK_JWT_AUDIENCE", "CK_JWT_TTL"}, peerVars...),
		secrets:  []string{"CK_REDIS_PASSWORD", "CK_JWT_SECRET", "CK_GATEWAY_SIGN_KEY"},
		absent:   []string{"CK_PG_DSN", "CK_PG_PASSWORD", "CK_KAFKA_BROKERS", "CK_INTERNAL_TOKEN", "CK_SEED_DEMO"},
		platform: []string{"Common", "Redis", "JWT", "GatewaySign", "Peers"},
	},
	{
		key: "user", name: "couponkill-user-service", port: "8081",
		config:   []string{"CK_ENV", "CK_HTTP_PORT", "CK_LOG_LEVEL", "CK_REDIS_ADDR", "CK_REDIS_DB", "CK_JWT_ISSUER", "CK_JWT_AUDIENCE", "CK_JWT_TTL", "CK_SEED_DEMO"},
		secrets:  []string{"CK_PG_DSN", "CK_REDIS_PASSWORD", "CK_JWT_SECRET", "CK_GATEWAY_SIGN_KEY", "CK_INTERNAL_TOKEN"},
		absent:   append([]string{"CK_KAFKA_BROKERS"}, peerVars...), // user 的 Config 不内嵌 Peers：它不调用其他服务
		platform: []string{"Common", "Postgres", "Redis", "JWT", "GatewaySign", "Internal"},
	},
	{
		key: "coupon", name: "couponkill-coupon-service", port: "8080",
		config:   append([]string{"CK_ENV", "CK_HTTP_PORT", "CK_LOG_LEVEL", "CK_REDIS_ADDR", "CK_REDIS_DB", "CK_SEED_DEMO"}, peerVars...),
		secrets:  []string{"CK_PG_DSN", "CK_REDIS_PASSWORD", "CK_GATEWAY_SIGN_KEY", "CK_INTERNAL_TOKEN"},
		absent:   []string{"CK_KAFKA_BROKERS", "CK_JWT_SECRET", "CK_JWT_ISSUER"},
		platform: []string{"Common", "Postgres", "Redis", "GatewaySign", "Internal", "Peers"},
	},
	{
		key: "order", name: "couponkill-order-service", port: "8082",
		config:   append([]string{"CK_ENV", "CK_HTTP_PORT", "CK_LOG_LEVEL", "CK_REDIS_ADDR", "CK_REDIS_DB", "CK_KAFKA_BROKERS"}, peerVars...),
		secrets:  []string{"CK_PG_DSN", "CK_REDIS_PASSWORD", "CK_GATEWAY_SIGN_KEY", "CK_INTERNAL_TOKEN"},
		absent:   []string{"CK_JWT_SECRET", "CK_SEED_DEMO"},
		platform: []string{"Common", "Postgres", "Redis", "Kafka", "GatewaySign", "Internal", "Peers"},
	},
	{
		key: "connector", name: "couponkill-connector-service", port: "8085",
		config:   append([]string{"CK_ENV", "CK_HTTP_PORT", "CK_LOG_LEVEL", "CK_REDIS_ADDR", "CK_REDIS_DB"}, peerVars...),
		secrets:  []string{"CK_PG_DSN", "CK_REDIS_PASSWORD", "CK_GATEWAY_SIGN_KEY", "CK_INTERNAL_TOKEN"},
		absent:   []string{"CK_KAFKA_BROKERS", "CK_JWT_SECRET", "CK_SEED_DEMO"},
		platform: []string{"Common", "Postgres", "Redis", "GatewaySign", "Internal", "Peers"},
	},
}

// serviceEnv 汇总服务的全部环境变量来源：ConfigMap 数据 + 容器 env（含 secretKeyRef）。
func serviceEnv(t *testing.T, objs []obj, sp serviceSpec) (cfg map[string]string, env map[string]envEntry, envOrder []string) {
	t.Helper()
	cm := find(objs, "ConfigMap", sp.name+"-config")
	if cm == nil {
		t.Fatalf("缺少 ConfigMap %s-config", sp.name)
	}
	dep := find(objs, "Deployment", sp.name)
	if dep == nil {
		t.Fatalf("缺少 Deployment %s", sp.name)
	}
	refs := container(dep).list("envFrom")
	if len(refs) != 1 || obj(refs[0].(map[string]any)).str("configMapRef", "name") != sp.name+"-config" {
		t.Fatalf("%s 应当 envFrom 自己的 ConfigMap，实际 %v", sp.name, refs)
	}
	envOrder, env = containerEnv(container(dep))
	return configMapData(cm), env, envOrder
}

// TestServiceEnvironment 每个服务都拿到它需要的 CK_* 配置（非机密走 ConfigMap，机密走 secretKeyRef），
// 且不多给：gateway 没有数据库口令，coupon 没有 JWT 密钥……
func TestServiceEnvironment(t *testing.T) {
	for _, comboName := range []string{"default", "bare", "external-deps", "prod"} {
		objs := renderCombo(t, comboName)
		for _, sp := range serviceSpecs {
			t.Run(comboName+"/"+sp.key, func(t *testing.T) {
				cfg, env, _ := serviceEnv(t, objs, sp)
				if cfg["CK_HTTP_PORT"] != sp.port {
					t.Errorf("CK_HTTP_PORT=%q，期望 %s", cfg["CK_HTTP_PORT"], sp.port)
				}
				for _, name := range sp.config {
					if cfg[name] == "" {
						t.Errorf("ConfigMap 缺少 %s", name)
					}
					if _, dup := env[name]; dup {
						t.Errorf("%s 同时出现在 ConfigMap 与容器 env 中（取值来源含糊）", name)
					}
				}
				for _, name := range sp.secrets {
					if comboName == "bare" && name == "CK_REDIS_PASSWORD" {
						if _, ok := env[name]; ok {
							t.Errorf("redis.auth.enabled=false 时不应注入 %s", name)
						}
						continue
					}
					e, ok := env[name]
					if !ok {
						t.Errorf("容器缺少机密变量 %s", name)
						continue
					}
					if name == "CK_PG_DSN" {
						continue // DSN 由 CK_PG_PASSWORD 拼出（TestDatabaseDSN 单独断言），不是 secretKeyRef
					}
					if e.SecretName == "" || e.SecretKey == "" {
						t.Errorf("%s 必须来自 secretKeyRef，实际 value=%q", name, e.Value)
					}
					if e.Value != "" {
						t.Errorf("%s 不得直接写明文: %q", name, e.Value)
					}
				}
				for _, name := range sp.absent {
					if _, ok := cfg[name]; ok {
						t.Errorf("ConfigMap 不应含 %s", name)
					}
					if _, ok := env[name]; ok {
						t.Errorf("容器 env 不应含 %s", name)
					}
				}
				// 机密绝不能落进 ConfigMap
				for name := range cfg {
					for _, bad := range []string{"SECRET", "PASSWORD", "TOKEN", "SIGN_KEY", "DSN"} {
						if strings.Contains(name, bad) && name != "CK_JWT_TTL" {
							t.Errorf("机密变量 %s 不得出现在 ConfigMap", name)
						}
					}
				}
			})
		}
	}
}

// TestDatabaseDSN CK_PG_DSN 由用户名/主机/库名拼出，口令只通过 secretKeyRef 的 CK_PG_PASSWORD 进入，
// 且 CK_PG_PASSWORD 必须排在 CK_PG_DSN 之前（K8s 的 $(VAR) 只展开前面已定义的变量）。
func TestDatabaseDSN(t *testing.T) {
	objs := renderCombo(t, "default")
	for _, sp := range serviceSpecs {
		if sp.key == "gateway" {
			continue
		}
		t.Run(sp.key, func(t *testing.T) {
			_, env, order := serviceEnv(t, objs, sp)
			dsn := env["CK_PG_DSN"].Value
			want := "postgres://couponkill:$(CK_PG_PASSWORD)@postgres:5432/couponkill?sslmode=disable"
			if dsn != want {
				t.Errorf("CK_PG_DSN=%q，期望 %q", dsn, want)
			}
			if indexOf(order, "CK_PG_PASSWORD") < 0 || indexOf(order, "CK_PG_PASSWORD") > indexOf(order, "CK_PG_DSN") {
				t.Errorf("CK_PG_PASSWORD 必须先于 CK_PG_DSN 定义: %v", order)
			}
			if e := env["CK_PG_PASSWORD"]; e.SecretKey != "postgres-password" {
				t.Errorf("CK_PG_PASSWORD 应取自键 postgres-password，实际 %+v", e)
			}
		})
	}
	ext := renderCombo(t, "external-deps")
	_, env, _ := serviceEnv(t, ext, serviceSpecs[1])
	if want := "postgres://couponkill:$(CK_PG_PASSWORD)@pg.example.internal:5432/couponkill?sslmode=disable"; env["CK_PG_DSN"].Value != want {
		t.Errorf("外部 PG：CK_PG_DSN=%q，期望 %q", env["CK_PG_DSN"].Value, want)
	}
	fromSecret := renderCombo(t, "existing-secret")
	_, env, _ = serviceEnv(t, fromSecret, serviceSpecs[1])
	if e := env["CK_PG_DSN"]; e.SecretName != "my-secret" || e.SecretKey != "pg-dsn" {
		t.Errorf("dsnFromSecret=true：CK_PG_DSN 应取自 my-secret/pg-dsn，实际 %+v", e)
	}
	if _, ok := env["CK_PG_PASSWORD"]; ok {
		t.Error("dsnFromSecret=true 时不应再注入 CK_PG_PASSWORD")
	}
}

func indexOf(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return -1
}

// TestSecretRefsResolve 容器里每个非 optional 的 secretKeyRef 都指向 chart 生成的 Secret 里真实存在的键；
// 同一个 Secret 名称贯穿所有服务与自带的 PostgreSQL / Redis。
func TestSecretRefsResolve(t *testing.T) {
	objs := renderCombo(t, "default")
	sec := find(objs, "Secret", "couponkill-app-secrets")
	if sec == nil {
		t.Fatal("缺少 Secret")
	}
	data := sec.m("data")
	for _, o := range append(ofKind(objs, "Deployment"), ofKind(objs, "StatefulSet")...) {
		_, env := containerEnv(container(o))
		for name, e := range env {
			if e.SecretName == "" {
				continue
			}
			if e.SecretName != "couponkill-app-secrets" {
				t.Errorf("%s/%s %s 指向 Secret %q", o.kind(), o.name(), name, e.SecretName)
			}
			if !e.Optional && data.str(e.SecretKey) == "" {
				t.Errorf("%s/%s %s 引用的键 %q 不在生成的 Secret 里", o.kind(), o.name(), name, e.SecretKey)
			}
		}
	}
}

// TestPeerURLsPointToServices 服务间 URL、Redis 地址、Kafka bootstrap 都指向 chart 里真实存在的 Service。
func TestPeerURLsPointToServices(t *testing.T) {
	objs := renderCombo(t, "default")
	svcOf := map[string]string{"CK_USER_URL": "couponkill-user-service", "CK_COUPON_URL": "couponkill-coupon-service", "CK_ORDER_URL": "couponkill-order-service", "CK_CONNECTOR_URL": "couponkill-connector-service"}
	for _, sp := range serviceSpecs {
		if sp.key == "user" {
			continue // user 不调用其他服务，没有 Peers
		}
		cfg, _, _ := serviceEnv(t, objs, sp)
		for envName, target := range svcOf {
			want := "http://" + target + "." + testNamespace + ".svc.cluster.local"
			if cfg[envName] != want {
				t.Errorf("%s %s=%q，期望 %q", sp.key, envName, cfg[envName], want)
			}
			if find(objs, "Service", target) == nil {
				t.Errorf("%s 指向的 Service %s 不存在", envName, target)
			}
		}
		if cfg["CK_REDIS_ADDR"] != "redis:6379" || find(objs, "Service", "redis") == nil {
			t.Errorf("%s CK_REDIS_ADDR=%q 与 Service redis 不符", sp.key, cfg["CK_REDIS_ADDR"])
		}
	}
	cfg, _, _ := serviceEnv(t, objs, serviceSpecs[3])
	if cfg["CK_KAFKA_BROKERS"] != "kafka:9092" || find(objs, "Service", "kafka") == nil {
		t.Errorf("CK_KAFKA_BROKERS=%q 与 Service kafka 不符", cfg["CK_KAFKA_BROKERS"])
	}
	ext := renderCombo(t, "external-deps")
	cfg, _, _ = serviceEnv(t, ext, serviceSpecs[3])
	if cfg["CK_KAFKA_BROKERS"] != "k1:9092,k2:9092" || cfg["CK_REDIS_ADDR"] != "redis.example.internal:6379" {
		t.Errorf("外部依赖：brokers=%q redis=%q", cfg["CK_KAFKA_BROKERS"], cfg["CK_REDIS_ADDR"])
	}
	for _, kind := range []string{"StatefulSet"} {
		if n := len(ofKind(ext, kind)); n != 0 {
			t.Errorf("全部外部依赖时不应渲染自带的 %s（%d 个）", kind, n)
		}
	}
}

// TestPlatformConfigCoverage 对照仓库里 internal/platform/config 的真实配置组：
// 每个服务内嵌的配置组里的每个 CK_* 变量，chart 都必须注入（CK_WORKER_ID 有安全默认值，豁免）。
func TestPlatformConfigCoverage(t *testing.T) {
	path := filepath.Join(repoDir(t), "internal", "platform", "config", "common.go")
	groups := goEnvTags(t, path)
	if len(groups) == 0 {
		t.Fatalf("没有从 %s 解析出任何配置组", path)
	}
	exempt := map[string]bool{"CK_WORKER_ID": true}
	objs := renderCombo(t, "default")
	for _, sp := range serviceSpecs {
		t.Run(sp.key, func(t *testing.T) {
			cfg, env, _ := serviceEnv(t, objs, sp)
			for _, g := range sp.platform {
				vars, ok := groups[g]
				if !ok {
					t.Errorf("internal/platform/config 里没有配置组 %s", g)
					continue
				}
				for _, v := range vars {
					if exempt[v] {
						continue
					}
					_, inEnv := env[v]
					if cfg[v] == "" && !inEnv {
						t.Errorf("配置组 %s 的 %s 没有被 chart 注入到 %s", g, v, sp.key)
					}
				}
			}
		})
	}
}

// TestChartVarsExistInGoConfig 反向校验：chart 注入的每个 CK_* 变量，在对应服务已经实现配置之后，
// 必须真的存在于某个 Go 配置结构里——防止 values 里的变量名写错/过期而被服务静默忽略。
// 服务目录里还没有任何 env 标签（配置尚未合入）时跳过该服务。
func TestChartVarsExistInGoConfig(t *testing.T) {
	all, perService := allGoEnvVars(t)
	objs := renderCombo(t, "default")
	for _, sp := range serviceSpecs {
		t.Run(sp.key, func(t *testing.T) {
			if !perService[sp.key] {
				t.Skipf("internal/%s 里还没有 env 配置标签，跳过变量名核对", sp.key)
			}
			cfg, env, _ := serviceEnv(t, objs, sp)
			names := map[string]bool{}
			for n := range cfg {
				names[n] = true
			}
			for n := range env {
				names[n] = true
			}
			delete(names, "CK_PG_PASSWORD") // chart 内部用于拼 DSN，不是服务配置项
			for _, n := range sortedKeys(names) {
				if !all[n] {
					t.Errorf("chart 为 %s 注入了 %s，但 Go 配置里没有这个环境变量", sp.key, n)
				}
			}
		})
	}
}
