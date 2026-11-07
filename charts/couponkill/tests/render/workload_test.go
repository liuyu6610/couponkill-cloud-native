package render

import (
	"strings"
	"testing"
)

// appDeployments 返回五个服务的稳定版 Deployment。
func appDeployments(t *testing.T, objs []obj) []obj {
	t.Helper()
	var out []obj
	for _, sp := range serviceSpecs {
		dep := find(objs, "Deployment", sp.name)
		if dep == nil {
			t.Fatalf("缺少 Deployment %s", sp.name)
		}
		out = append(out, dep)
	}
	return out
}

// TestWorkloadHardening 每个服务 Pod：非 root、只读根文件系统、drop ALL、不挂 API Token、
// 探针指向 /healthz（存活）与 /readyz（就绪）的服务端口，资源 requests/limits 齐全，镜像命名符合约定。
func TestWorkloadHardening(t *testing.T) {
	for _, c := range valueCombos {
		t.Run(c.name, func(t *testing.T) {
			objs := renderChart(t, c.args...)
			for i, dep := range appDeployments(t, objs) {
				sp := serviceSpecs[i]
				ps := podSpec(dep)
				if ps.str("automountServiceAccountToken") != "false" {
					t.Errorf("%s 不应挂载 ServiceAccount Token", sp.name)
				}
				if ps.str("serviceAccountName") != sp.name || find(objs, "ServiceAccount", sp.name) == nil {
					t.Errorf("%s 应使用同名 ServiceAccount", sp.name)
				}
				if ps.str("securityContext", "runAsNonRoot") != "true" || ps.str("securityContext", "runAsUser") != "65532" {
					t.Errorf("%s Pod 必须以 65532 非 root 运行: %v", sp.name, ps.m("securityContext"))
				}
				if ps.str("securityContext", "seccompProfile", "type") != "RuntimeDefault" {
					t.Errorf("%s 缺少 seccomp RuntimeDefault", sp.name)
				}
				cnt := container(dep)
				if cnt.str("securityContext", "readOnlyRootFilesystem") != "true" ||
					cnt.str("securityContext", "allowPrivilegeEscalation") != "false" ||
					cnt.str("securityContext", "capabilities", "drop", 0) != "ALL" {
					t.Errorf("%s 容器安全上下文不达标: %v", sp.name, cnt.m("securityContext"))
				}
				if !strings.Contains(cnt.str("image"), "/couponkill-"+sp.key+":") {
					t.Errorf("%s 镜像 %q 不符合 <registry>/couponkill-%s:<tag>", sp.name, cnt.str("image"), sp.key)
				}
				if cnt.str("ports", 0, "name") != "http" || cnt.str("ports", 0, "containerPort") != sp.port {
					t.Errorf("%s 容器端口应为 http=%s: %v", sp.name, sp.port, cnt.list("ports"))
				}
				for _, probe := range []struct{ field, path string }{
					{"startupProbe", "/healthz"}, {"livenessProbe", "/healthz"}, {"readinessProbe", "/readyz"},
				} {
					if got := cnt.str(probe.field, "httpGet", "path"); got != probe.path {
						t.Errorf("%s %s 路径=%q，期望 %s", sp.name, probe.field, got, probe.path)
					}
					if got := cnt.str(probe.field, "httpGet", "port"); got != "http" {
						t.Errorf("%s %s 端口=%q，期望命名端口 http", sp.name, probe.field, got)
					}
				}
				for _, r := range []string{"requests", "limits"} {
					for _, res := range []string{"cpu", "memory"} {
						if cnt.str("resources", r, res) == "" {
							t.Errorf("%s 缺少 resources.%s.%s", sp.name, r, res)
						}
					}
				}
				if dep.str("spec", "strategy", "rollingUpdate", "maxUnavailable") != "0" {
					t.Errorf("%s 滚动更新应保证 maxUnavailable=0", sp.name)
				}
				// 只读根文件系统必须配一个可写的 /tmp
				if cnt.str("volumeMounts", 0, "mountPath") != "/tmp" {
					t.Errorf("%s 缺少 /tmp 可写卷", sp.name)
				}
			}
		})
	}
}

// TestServiceObjects 每个服务一个 Service（80 → 命名端口 http），按 app 标签选择 Pod；
// 选择器与 Deployment 标签对得上；PDB 默认存在；Service 带抓取标签。
func TestServiceObjects(t *testing.T) {
	objs := renderCombo(t, "default")
	for _, sp := range serviceSpecs {
		svc := find(objs, "Service", sp.name)
		if svc == nil {
			t.Fatalf("缺少 Service %s", sp.name)
		}
		if svc.str("spec", "ports", 0, "port") != "80" || svc.str("spec", "ports", 0, "targetPort") != "http" || svc.str("spec", "ports", 0, "name") != "http" {
			t.Errorf("%s Service 端口应为 80→http: %v", sp.name, svc.list("spec", "ports"))
		}
		if svc.str("spec", "selector", "app") != sp.name || len(svc.m("spec", "selector")) != 1 {
			t.Errorf("%s Service 只应按 app 选择（以便与金丝雀共用）: %v", sp.name, svc.m("spec", "selector"))
		}
		dep := find(objs, "Deployment", sp.name)
		if dep.str("spec", "template", "metadata", "labels", "app") != sp.name {
			t.Errorf("%s Pod 缺少 app 标签", sp.name)
		}
		pdb := find(objs, "PodDisruptionBudget", sp.name)
		if pdb == nil {
			t.Errorf("%s 缺少 PodDisruptionBudget", sp.name)
		} else if pdb.str("spec", "maxUnavailable") != "1" || pdb.str("spec", "selector", "matchLabels", "app") != sp.name {
			t.Errorf("%s PDB 不符合预期: %v", sp.name, pdb.m("spec"))
		}
		if svc.str("metadata", "labels", "couponkill.io/scrape") != "true" {
			t.Errorf("%s Service 缺少抓取标签", sp.name)
		}
	}
	if find(renderCombo(t, "bare"), "PodDisruptionBudget", serviceSpecs[0].name) != nil {
		t.Error("pdb.enabled=false 时不应渲染 PDB")
	}
}

// TestAutoscaling 默认不带 HPA 且 Deployment 写 replicas；prod 对 gateway/user/coupon 开 HPA 并且 Deployment 不再写 replicas
// （否则每次 helm upgrade 都会把副本数重置）；order 由 KEDA 接管，既无 HPA 也不写 replicas。
func TestAutoscaling(t *testing.T) {
	objs := renderCombo(t, "default")
	if n := len(ofKind(objs, "HorizontalPodAutoscaler")); n != 0 {
		t.Errorf("默认不应有 HPA，实际 %d", n)
	}
	for _, dep := range appDeployments(t, objs) {
		if dep.str("spec", "replicas") != "1" {
			t.Errorf("%s 默认 replicas 应为 1，实际 %q", dep.name(), dep.str("spec", "replicas"))
		}
	}

	prod := renderCombo(t, "prod")
	for _, sp := range serviceSpecs {
		hpa := find(prod, "HorizontalPodAutoscaler", sp.name)
		dep := find(prod, "Deployment", sp.name)
		switch sp.key {
		case "gateway", "user", "coupon":
			if hpa == nil {
				t.Errorf("prod 下 %s 应有 HPA", sp.name)
				continue
			}
			if hpa.str("spec", "scaleTargetRef", "name") != sp.name {
				t.Errorf("%s HPA 目标不对: %v", sp.name, hpa.m("spec", "scaleTargetRef"))
			}
			if dep.get("spec", "replicas") != nil {
				t.Errorf("%s 由 HPA 管理时 Deployment 不应写 replicas", sp.name)
			}
		case "order":
			if hpa != nil {
				t.Errorf("KEDA 接管 order 时不应渲染它的 HPA")
			}
			if dep.get("spec", "replicas") != nil {
				t.Error("KEDA 接管 order 时 Deployment 不应写 replicas")
			}
		default:
			if hpa != nil {
				t.Errorf("%s 在 prod 下未开启 autoscaling，不应有 HPA", sp.name)
			}
			if dep.str("spec", "replicas") != "2" {
				t.Errorf("%s prod replicas 应为 2", sp.name)
			}
		}
	}
}

// TestMonitoring 默认没有抓取注解与 ServiceMonitor；monitoring.enabled 后 Pod 带 prometheus.io 注解（路径 /metrics，端口=服务端口），
// ServiceMonitor 的选择器能命中全部五个 Service，并抓取命名端口 http 的 /metrics。
func TestMonitoring(t *testing.T) {
	def := renderCombo(t, "default")
	if len(ofKind(def, "ServiceMonitor")) != 0 {
		t.Error("默认不应渲染 ServiceMonitor")
	}
	for _, dep := range appDeployments(t, def) {
		if dep.get("spec", "template", "metadata", "annotations", "prometheus.io/scrape") != nil {
			t.Errorf("%s 默认不应带 prometheus.io 注解", dep.name())
		}
	}

	objs := renderCombo(t, "monitoring")
	for i, dep := range appDeployments(t, objs) {
		ann := dep.m("spec", "template", "metadata", "annotations")
		if ann.str("prometheus.io/scrape") != "true" || ann.str("prometheus.io/path") != "/metrics" || ann.str("prometheus.io/port") != serviceSpecs[i].port {
			t.Errorf("%s 抓取注解不对: %v", dep.name(), ann)
		}
	}
	sm := find(objs, "ServiceMonitor", "couponkill")
	if sm == nil {
		t.Fatal("monitoring.enabled=true 应渲染 ServiceMonitor")
	}
	if sm.str("spec", "endpoints", 0, "path") != "/metrics" || sm.str("spec", "endpoints", 0, "port") != "http" {
		t.Errorf("ServiceMonitor 端点不对: %v", sm.list("spec", "endpoints"))
	}
	want := sm.m("spec", "selector", "matchLabels")
	for _, sp := range serviceSpecs {
		svc := find(objs, "Service", sp.name)
		labels := svc.m("metadata", "labels")
		for k, v := range want {
			if labels.str(k) != v.(string) {
				t.Errorf("ServiceMonitor 选择器 %s=%v 命不中 %s", k, v, sp.name)
			}
		}
	}
	// 基础设施 Service 不应被抓取（它们没有 /metrics）
	for _, name := range []string{"postgres", "redis", "kafka"} {
		if find(objs, "Service", name).str("metadata", "labels", "couponkill.io/scrape") == "true" {
			t.Errorf("%s 不应带抓取标签", name)
		}
	}
	if np := find(objs, "NetworkPolicy", "allow-prometheus-scrape"); np == nil {
		t.Error("monitoring.enabled=true 时应放行 Prometheus 抓取")
	}
}

// TestNetworkPolicy default-deny 存在；只有 gateway 对入口命名空间开放；关闭后整套策略消失。
func TestNetworkPolicy(t *testing.T) {
	objs := renderCombo(t, "default")
	for _, name := range []string{"default-deny", "allow-same-namespace", "allow-dns", "allow-gateway-ingress", "allow-connector-egress"} {
		if find(objs, "NetworkPolicy", name) == nil {
			t.Errorf("缺少 NetworkPolicy %s", name)
		}
	}
	gw := find(objs, "NetworkPolicy", "allow-gateway-ingress")
	if gw.str("spec", "podSelector", "matchLabels", "app") != "couponkill-gateway" {
		t.Errorf("入口只应对 gateway 开放: %v", gw.m("spec", "podSelector"))
	}
	if n := len(ofKind(renderCombo(t, "bare"), "NetworkPolicy")); n != 0 {
		t.Errorf("networkPolicy.enabled=false 时不应有 NetworkPolicy，实际 %d", n)
	}
}

// TestDependenciesStateful 自带的 PostgreSQL / Redis / Kafka 是带持久卷的单副本 StatefulSet；
// 持久化可以关闭；Redis 禁止淘汰库存键；Redis 口令与 Secret 对齐。
func TestDependenciesStateful(t *testing.T) {
	objs := renderCombo(t, "default")
	for _, name := range []string{"postgres", "redis", "kafka"} {
		sts := find(objs, "StatefulSet", name)
		if sts == nil {
			t.Fatalf("缺少 StatefulSet %s", name)
		}
		if sts.str("spec", "replicas") != "1" {
			t.Errorf("%s 应为单副本", name)
		}
		if len(sts.list("spec", "volumeClaimTemplates")) != 1 {
			t.Errorf("%s 默认应有持久卷", name)
		}
		if podSpec(sts).str("securityContext", "runAsNonRoot") != "true" {
			t.Errorf("%s 应非 root 运行", name)
		}
		if podSpec(sts).str("automountServiceAccountToken") != "false" {
			t.Errorf("%s 不应挂载 API Token", name)
		}
	}
	args := strings.Join(toStrings(container(find(objs, "StatefulSet", "redis")).list("args")), " ")
	for _, want := range []string{"--appendonly yes", "--maxmemory-policy noeviction", "--requirepass $(REDIS_PASSWORD)"} {
		if !strings.Contains(args, want) {
			t.Errorf("Redis 参数缺少 %q: %s", want, args)
		}
	}
	bare := renderCombo(t, "bare")
	for _, name := range []string{"postgres", "redis", "kafka"} {
		sts := find(bare, "StatefulSet", name)
		if len(sts.list("spec", "volumeClaimTemplates")) != 0 {
			t.Errorf("%s 关闭持久化后不应有 PVC 模板", name)
		}
	}
	rbare := strings.Join(toStrings(container(find(bare, "StatefulSet", "redis")).list("args")), " ")
	if !strings.Contains(rbare, "--appendonly no") || strings.Contains(rbare, "requirepass") {
		t.Errorf("无持久化/无口令的 Redis 参数不对: %s", rbare)
	}
	pg := container(find(objs, "StatefulSet", "postgres"))
	_, env := containerEnv(pg)
	if env["POSTGRES_PASSWORD"].SecretKey != "postgres-password" {
		t.Errorf("自带 PostgreSQL 的口令应取自 Secret: %+v", env["POSTGRES_PASSWORD"])
	}
}

// TestOperatorOptional Operator / CRD / 示例资源默认不渲染，开启后才出现。
func TestOperatorOptional(t *testing.T) {
	def := renderCombo(t, "default")
	for _, kind := range []string{"CustomResourceDefinition", "ClusterRole", "ClusterRoleBinding", "Role", "RoleBinding"} {
		if n := len(ofKind(def, kind)); n != 0 {
			t.Errorf("默认不应渲染 %s（%d 个）", kind, n)
		}
	}
	if find(def, "Deployment", "couponkill-operator") != nil {
		t.Error("默认不应渲染 Operator")
	}
	on := renderCombo(t, "operator")
	op := find(on, "Deployment", "couponkill-operator")
	if op == nil {
		t.Fatal("operator.enabled=true 应渲染 Operator Deployment")
	}
	if !strings.HasSuffix(container(op).str("image"), "/couponkill-operator:latest") {
		t.Errorf("Operator 镜像=%q", container(op).str("image"))
	}
	if find(on, "CustomResourceDefinition", "seckills.ops.couponkill.io") == nil {
		t.Error("缺少 Seckill CRD")
	}
	if find(on, "Seckill", "example-seckill") == nil {
		t.Error("examples.seckill=true 应渲染示例")
	}
}

// TestIngress 只暴露 gateway。
func TestIngress(t *testing.T) {
	if len(ofKind(renderCombo(t, "default"), "Ingress")) != 0 {
		t.Error("默认不应渲染 Ingress")
	}
	ing := find(renderCombo(t, "ingress"), "Ingress", "couponkill")
	if ing == nil {
		t.Fatal("ingress.enabled=true 应渲染 Ingress")
	}
	rules := ing.list("spec", "rules")
	if len(rules) != 1 || ing.str("spec", "rules", 0, "http", "paths", 0, "backend", "service", "name") != "couponkill-gateway" {
		t.Errorf("Ingress 只应指向 gateway: %v", rules)
	}
	if ing.str("spec", "tls", 0, "secretName") != "tls-couponkill" {
		t.Errorf("Ingress TLS 不对: %v", ing.list("spec", "tls"))
	}
}

// TestConfigChecksumRollsPods ConfigMap 内容变化时 Pod 模板的 checksum 注解随之变化（触发滚动重启）。
func TestConfigChecksumRollsPods(t *testing.T) {
	a := find(renderCombo(t, "default"), "Deployment", "couponkill-order-service")
	b := find(renderCombo(t, "default", "--set", "services.order.config.CK_ORDER_SECKILL_COOLDOWN=5s"), "Deployment", "couponkill-order-service")
	ca := a.str("spec", "template", "metadata", "annotations", "checksum/config")
	cb := b.str("spec", "template", "metadata", "annotations", "checksum/config")
	if ca == "" || ca == cb {
		t.Errorf("配置变化后 checksum 应改变: %q vs %q", ca, cb)
	}
	cm := find(renderCombo(t, "default", "--set", "services.order.config.CK_ORDER_SECKILL_COOLDOWN=5s"), "ConfigMap", "couponkill-order-service-config")
	if got := configMapData(cm)["CK_ORDER_SECKILL_COOLDOWN"]; got != "5s" {
		t.Errorf("服务私有配置没有进入 ConfigMap: %q", got)
	}
}

func toStrings(in []any) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		out = append(out, v.(string))
	}
	return out
}
