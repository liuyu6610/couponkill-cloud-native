package render

import (
	"fmt"
	"strings"
	"testing"
)

// clusterScoped 是没有 namespace 的对象种类（本 chart 目前没有，列出以便将来扩展）。
var clusterScoped = map[string]bool{
	"CustomResourceDefinition": true,
	"ClusterRole":              true,
	"ClusterRoleBinding":       true,
	"Namespace":                true,
}

// TestAllValueCombosRender 每个 values 组合都能渲染、解析，且没有重复对象（重复对象会让 helm install 直接失败），
// 镜像引用合法，命名空间对象都落在目标命名空间。
func TestAllValueCombosRender(t *testing.T) {
	for _, c := range valueCombos {
		t.Run(c.name, func(t *testing.T) {
			objs := renderChart(t, c.args...)
			if len(objs) == 0 {
				t.Fatal("没有渲染出任何对象")
			}
			seen := map[string]bool{}
			for _, o := range objs {
				key := fmt.Sprintf("%s/%s/%s", o.kind(), o.str("metadata", "namespace"), o.name())
				if seen[key] {
					t.Errorf("重复对象: %s", key)
				}
				seen[key] = true
				if o.kind() == "" || o.name() == "" {
					t.Errorf("对象缺 kind/name: %v", strings.TrimSpace(fmt.Sprint(o)))
				}
				if ns := o.str("metadata", "namespace"); !clusterScoped[o.kind()] && ns != testNamespace {
					t.Errorf("%s/%s 的命名空间是 %q，期望 %q", o.kind(), o.name(), ns, testNamespace)
				}
				checkImages(t, o)
			}
		})
	}
}

// checkImages 校验容器镜像引用合法：非空、带标签，最后一个 "/" 之后恰好一个 ":"（排除 "repo:tag:tag" 与缺标签）。
func checkImages(t *testing.T, o obj) {
	t.Helper()
	spec := podSpec(o)
	if spec == nil {
		return
	}
	for _, field := range []string{"containers", "initContainers"} {
		for _, c := range spec.list(field) {
			cm, _ := c.(map[string]any)
			image := obj(cm).str("image")
			last := image[strings.LastIndex(image, "/")+1:]
			if image == "" || strings.Count(last, ":") != 1 || strings.HasSuffix(image, ":") || strings.ContainsAny(image, " \t") {
				t.Errorf("%s/%s 镜像引用非法: %q", o.kind(), o.name(), image)
			}
		}
	}
}

// TestNoRemovedComponents Java 时代的组件不能再出现：Nacos、Sentinel、db-init、redis-init、config-watcher，
// 工作负载里也不能再有 SPRING_/NACOS_/JAVA_ 环境变量；唯一允许的 Job 是 Kafka 主题初始化。
func TestNoRemovedComponents(t *testing.T) {
	for _, c := range valueCombos {
		t.Run(c.name, func(t *testing.T) {
			for _, o := range renderChart(t, c.args...) {
				lower := strings.ToLower(o.name())
				for _, banned := range []string{"nacos", "sentinel", "db-init", "redis-init", "config-watcher"} {
					if strings.Contains(lower, banned) {
						t.Errorf("%s/%s 属于已删除的组件 %q", o.kind(), o.name(), banned)
					}
				}
				if o.kind() == "Job" && o.name() != "kafka-topic-init" {
					t.Errorf("不应有 Job %s（迁移由服务自己执行）", o.name())
				}
				if cnt := container(o); cnt != nil {
					for _, e := range cnt.list("env") {
						em, _ := e.(map[string]any)
						name := obj(em).str("name")
						for _, prefix := range []string{"SPRING_", "NACOS_", "JAVA_", "SENTINEL_"} {
							if strings.HasPrefix(name, prefix) {
								t.Errorf("%s/%s 仍注入 Java 时代的环境变量 %s", o.kind(), o.name(), name)
							}
						}
					}
				}
			}
		})
	}
}

// TestGuards 不安全或自相矛盾的配置在渲染阶段就失败，而不是上线后才发现。
func TestGuards(t *testing.T) {
	prod := []string{"-f", "values-prod.yaml"}
	cases := []struct {
		name string
		want string
		args []string
	}{
		{"prod 拒绝 latest 标签", "image.tag", prod},
		{"prod 拒绝 chart 生成密钥", "secrets.create=false", append([]string{"--set", "secrets.create=true", "--set-string", "image.tag=1"}, prod...)},
		{"prod 拒绝演示数据", "seedDemo", append([]string{"--set", "config.seedDemo=true", "--set-string", "image.tag=1"}, prod...)},
		{"未知 env", "config.env", []string{"--set", "config.env=staging"}},
		{"KEDA 上限超过分区数", "partitions", []string{"--set", "keda.enabled=true", "--set", "keda.order.maxReplicaCount=99"}},
		{"Istio 与 Ingress 二选一", "二选一", []string{"--set", "istio.enabled=true", "--set", "ingress.enabled=true"}},
		{"金丝雀权重越界", "weightCanary", []string{"--set", "canary.order.enabled=true", "--set", "canary.order.weightCanary=101"}},
		{"外部 PG 必须给 host", "postgres.host", []string{"--set", "postgres.enabled=false"}},
		{"外部 Redis 必须给 host", "redis.host", []string{"--set", "redis.enabled=false"}},
		{"外部 Kafka 必须给 brokers", "kafka.brokers", []string{"--set", "kafka.enabled=false"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			expectRenderError(t, c.want, c.args...)
		})
	}
}

// TestProdValuesRender prod 在给出镜像标签后可以渲染，并且不生成 Secret、不带演示数据。
func TestProdValuesRender(t *testing.T) {
	objs := renderCombo(t, "prod")
	if o := ofKind(objs, "Secret"); len(o) != 0 {
		t.Errorf("生产不应由 chart 生成 Secret: %v", o[0].name())
	}
	for _, key := range []string{"couponkill-user-service", "couponkill-coupon-service"} {
		cm := find(objs, "ConfigMap", key+"-config")
		if cm == nil {
			t.Fatalf("缺少 %s-config", key)
		}
		if got := configMapData(cm)["CK_SEED_DEMO"]; got != "false" {
			t.Errorf("%s CK_SEED_DEMO=%q，生产必须为 false", key, got)
		}
		if got := configMapData(cm)["CK_ENV"]; got != "prod" {
			t.Errorf("%s CK_ENV=%q，期望 prod", key, got)
		}
	}
	dep := find(objs, "Deployment", "couponkill-gateway")
	if img := container(dep).str("image"); !strings.HasSuffix(img, "/couponkill-gateway:42") {
		t.Errorf("gateway 镜像=%q，期望以 /couponkill-gateway:42 结尾", img)
	}
	if got := container(dep).str("imagePullPolicy"); got != "IfNotPresent" {
		t.Errorf("固定标签应使用 IfNotPresent，实际 %q", got)
	}
}

// TestSecretGeneration secrets.create=true 时生成 Secret，含全部必需键且每个取值非空；connector 凭证取自 values。
func TestSecretGeneration(t *testing.T) {
	objs := renderCombo(t, "default", "--set", "secrets.values.connector.connector-jd-app-key=abc")
	sec := find(objs, "Secret", "couponkill-app-secrets")
	if sec == nil {
		t.Fatal("secrets.create=true 时应生成 couponkill-app-secrets")
	}
	data := sec.m("data")
	for _, k := range []string{"jwt-secret", "internal-token", "gateway-sign-key", "postgres-password", "redis-password", "connector-jd-app-key"} {
		if data.str(k) == "" {
			t.Errorf("Secret 缺少键 %s", k)
		}
	}
	if find(renderCombo(t, "existing-secret"), "Secret", "my-secret") != nil {
		t.Error("secrets.create=false 时不应渲染 Secret")
	}
}
