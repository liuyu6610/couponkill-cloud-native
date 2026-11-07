package render

import (
	"fmt"
	"strings"
	"testing"
)

// fqdn 返回服务在测试命名空间里的 Service FQDN。
func fqdn(name string) string {
	return fmt.Sprintf("%s.%s.svc.cluster.local", name, testNamespace)
}

// TestIstioOffByDefault 默认不渲染任何 Istio 资源。
func TestIstioOffByDefault(t *testing.T) {
	for _, kind := range []string{"Gateway", "VirtualService", "DestinationRule"} {
		if n := len(ofKind(renderCombo(t, "default"), kind)); n != 0 {
			t.Errorf("默认不应渲染 %s（%d 个）", kind, n)
		}
	}
}

// TestIstioOnlyGatewayExposed 开启 Istio 后入口只通向 gateway 服务：
// 入口 VirtualService 只放行 /api/ 与 /healthz，其余 404（所以 /metrics、/readyz、/internal 出不去）；
// 其他服务没有任何入口路由。每个 host 只有一条 DestinationRule，且都是 mTLS。
func TestIstioOnlyGatewayExposed(t *testing.T) {
	for _, name := range []string{"istio", "prod", "canary-keda", "prod+canary-keda"} {
		t.Run(name, func(t *testing.T) {
			objs := renderCombo(t, name)

			gw := ofKind(objs, "Gateway")
			if len(gw) != 1 {
				t.Fatalf("应有且只有一个入口 Gateway，实际 %d", len(gw))
			}
			vs := find(objs, "VirtualService", "couponkill-vs")
			if vs == nil {
				t.Fatal("缺少入口 VirtualService couponkill-vs")
			}
			if vs.str("spec", "gateways", 0) != gw[0].name() {
				t.Errorf("入口 VirtualService 应绑定 Gateway %s", gw[0].name())
			}

			// 入口路由：目标只能是 gateway
			routes := vs.list("spec", "http")
			if len(routes) != 2 {
				t.Fatalf("入口应有 2 条路由（放行 + 兜底 404），实际 %d", len(routes))
			}
			gwRoute := obj(routes[0].(map[string]any))
			for _, r := range gwRoute.list("route") {
				host := obj(r.(map[string]any)).str("destination", "host")
				if host != fqdn("couponkill-gateway") {
					t.Errorf("入口只能路由到 gateway，实际 %s", host)
				}
			}
			var matched []string
			for _, m := range gwRoute.list("match") {
				mo := obj(m.(map[string]any))
				matched = append(matched, mo.str("uri", "prefix")+mo.str("uri", "exact"))
			}
			if strings.Join(matched, ",") != "/api/,/healthz" {
				t.Errorf("入口放行的路径=%v，期望 /api/ 与 /healthz", matched)
			}
			fallback := obj(routes[1].(map[string]any))
			if fallback.str("directResponse", "status") != "404" || fallback.get("route") != nil {
				t.Errorf("兜底路由必须直接返回 404: %v", fallback)
			}

			// 绑定入口 Gateway 的 VirtualService 只有这一个
			for _, v := range ofKind(objs, "VirtualService") {
				if len(v.list("spec", "gateways")) > 0 && v.name() != "couponkill-vs" {
					t.Errorf("VirtualService %s 不应绑定入口 Gateway（只有 gateway 服务可以对外）", v.name())
				}
			}

			// 每个 host 一条 DestinationRule，覆盖五个服务，mTLS
			hosts := map[string]string{}
			for _, dr := range ofKind(objs, "DestinationRule") {
				h := dr.str("spec", "host")
				if prev, dup := hosts[h]; dup {
					t.Errorf("host %s 有多条 DestinationRule: %s / %s", h, prev, dr.name())
				}
				hosts[h] = dr.name()
				if dr.str("spec", "trafficPolicy", "tls", "mode") != "ISTIO_MUTUAL" {
					t.Errorf("%s 应使用 ISTIO_MUTUAL", dr.name())
				}
				if dr.get("spec", "trafficPolicy", "outlierDetection") == nil || dr.get("spec", "trafficPolicy", "connectionPool") == nil {
					t.Errorf("%s 缺少连接池/熔断策略", dr.name())
				}
			}
			for _, sp := range serviceSpecs {
				if _, ok := hosts[fqdn(sp.name)]; !ok {
					t.Errorf("缺少 %s 的 DestinationRule", sp.name)
				}
			}
		})
	}
}

// TestCanary 金丝雀：coupon / order 各多一个 version=canary 的 Deployment，与稳定版共用 Service；
// Istio 开启时 VirtualService 权重合计 100，子集与 Pod 标签对得上；未开启的服务没有金丝雀。
func TestCanary(t *testing.T) {
	objs := renderCombo(t, "canary-keda", "--set", "canary.coupon.tag=77", "--set", "canary.coupon.replicas=2")
	for _, svc := range []struct {
		key, name string
		weight    string
	}{{"coupon", "couponkill-coupon-service", "10"}, {"order", "couponkill-order-service", "5"}} {
		stable := find(objs, "Deployment", svc.name)
		canary := find(objs, "Deployment", svc.name+"-canary")
		if stable == nil || canary == nil {
			t.Fatalf("%s 应同时有稳定版与金丝雀版 Deployment", svc.name)
		}
		if got := canary.str("spec", "template", "metadata", "labels", "version"); got != "canary" {
			t.Errorf("%s 金丝雀 Pod version=%q", svc.name, got)
		}
		if got := stable.str("spec", "template", "metadata", "labels", "version"); got != "stable" {
			t.Errorf("%s 稳定版 Pod version=%q", svc.name, got)
		}
		if canary.str("spec", "selector", "matchLabels", "version") != "canary" || stable.str("spec", "selector", "matchLabels", "version") != "stable" {
			t.Errorf("%s 两个 Deployment 的选择器必须互不重叠", svc.name)
		}
		if canary.str("spec", "template", "metadata", "labels", "app") != svc.name {
			t.Errorf("%s 金丝雀 Pod 需带 app 标签才能挂在同一 Service 下", svc.name)
		}
		// 与稳定版共用同一个 ConfigMap
		if canary.str("spec", "template", "spec", "containers", 0, "envFrom", 0, "configMapRef", "name") != svc.name+"-config" {
			t.Errorf("%s 金丝雀应与稳定版共用 ConfigMap", svc.name)
		}

		vs := find(objs, "VirtualService", svc.name+"-canary-vs")
		if vs == nil {
			t.Fatalf("缺少 %s-canary-vs", svc.name)
		}
		routes := vs.list("spec", "http", 0, "route")
		total := 0
		subsets := map[string]string{}
		for _, r := range routes {
			ro := obj(r.(map[string]any))
			var w int
			fmt.Sscan(ro.str("weight"), &w)
			total += w
			subsets[ro.str("destination", "subset")] = ro.str("weight")
		}
		if total != 100 || subsets["canary"] != svc.weight {
			t.Errorf("%s 金丝雀权重 %v（合计 %d），期望 canary=%s 合计 100", svc.name, subsets, total, svc.weight)
		}
		dr := find(objs, "DestinationRule", svc.name+"-dr")
		var names []string
		for _, s := range dr.list("spec", "subsets") {
			so := obj(s.(map[string]any))
			names = append(names, so.str("name")+"="+so.str("labels", "version"))
		}
		if strings.Join(names, ",") != "stable=stable,canary=canary" {
			t.Errorf("%s 子集=%v", svc.name, names)
		}
	}
	if img := container(find(objs, "Deployment", "couponkill-coupon-service-canary")).str("image"); !strings.HasSuffix(img, "/couponkill-coupon:77") {
		t.Errorf("金丝雀镜像应使用 canary.coupon.tag=77: %s", img)
	}
	if got := find(objs, "Deployment", "couponkill-coupon-service-canary").str("spec", "replicas"); got != "2" {
		t.Errorf("canary.coupon.replicas=2 未生效: %q", got)
	}
	if img := container(find(objs, "Deployment", "couponkill-order-service-canary")).str("image"); !strings.HasSuffix(img, "/couponkill-order:canary") {
		t.Errorf("order 金丝雀默认标签应为 canary: %s", img)
	}
	// 稳定版镜像不受金丝雀标签影响
	if img := container(find(objs, "Deployment", "couponkill-coupon-service")).str("image"); !strings.HasSuffix(img, "/couponkill-coupon:latest") {
		t.Errorf("稳定版镜像不应被金丝雀标签改变: %s", img)
	}
	// 其他服务没有金丝雀
	for _, name := range []string{"couponkill-gateway", "couponkill-user-service", "couponkill-connector-service"} {
		if find(objs, "Deployment", name+"-canary") != nil {
			t.Errorf("%s 不应有金丝雀", name)
		}
	}
	// 默认没有任何金丝雀
	for _, o := range ofKind(renderCombo(t, "default"), "Deployment") {
		if strings.HasSuffix(o.name(), "-canary") {
			t.Errorf("默认不应渲染金丝雀 %s", o.name())
		}
	}
}

// TestKedaOrderConsumerGroup KEDA 触发器的 consumerGroup / topic 必须是真实消费者的值
// （消费组 seckill-order-create-group，主题下划线命名），且 KEDA 接管 order 时不再渲染它的 HPA，
// ScaledObject 指向真实存在的 Deployment，最大副本数不超过主题分区数。
func TestKedaOrderConsumerGroup(t *testing.T) {
	for _, name := range []string{"canary-keda", "prod", "prod+canary-keda"} {
		t.Run(name, func(t *testing.T) {
			objs := renderCombo(t, name)
			so := find(objs, "ScaledObject", "so-couponkill-order-service")
			if so == nil {
				t.Fatal("缺少 order 的 ScaledObject")
			}
			trigger := so.m("spec", "triggers", 0)
			if trigger.str("type") != "kafka" {
				t.Errorf("触发器类型=%q", trigger.str("type"))
			}
			md := trigger.m("metadata")
			if got := md.str("consumerGroup"); got != "seckill-order-create-group" {
				t.Errorf("consumerGroup=%q", got)
			}
			if got := md.str("topic"); got != "seckill_order_create" {
				t.Errorf("topic=%q", got)
			}
			if got := md.str("bootstrapServers"); got != "kafka:9092" {
				t.Errorf("bootstrapServers=%q", got)
			}
			if strings.Contains(md.str("topic"), ".") {
				t.Errorf("主题统一下划线命名，不含 \".\": %q", md.str("topic"))
			}
			const partitions = 16 // values.yaml 默认的 seckill_order_create 分区数
			var maxReplicas int
			fmt.Sscan(so.str("spec", "maxReplicaCount"), &maxReplicas)
			if maxReplicas > partitions {
				t.Errorf("maxReplicaCount=%d 超过分区数 %d", maxReplicas, partitions)
			}
			if find(objs, "HorizontalPodAutoscaler", "couponkill-order-service") != nil {
				t.Error("KEDA 接管 order 时不应再渲染 order 的 HPA")
			}
			for _, s := range ofKind(objs, "ScaledObject") {
				target := s.str("spec", "scaleTargetRef", "name")
				if find(objs, "Deployment", target) == nil {
					t.Errorf("ScaledObject %s 指向不存在的 Deployment %s", s.name(), target)
				}
			}
		})
	}
	if n := len(ofKind(renderCombo(t, "default"), "ScaledObject")); n != 0 {
		t.Errorf("默认不应渲染 ScaledObject，实际 %d", n)
	}
}

// TestKedaFollowsKafkaSettings KEDA 的 topic 与 bootstrap 跟随 kafka.* 的配置，外部 Kafka 时用 brokers。
func TestKedaFollowsKafkaSettings(t *testing.T) {
	objs := renderCombo(t, "external-deps", "--set", "keda.enabled=true",
		"--set", "kafka.topics.seckillOrderCreate.partitions=4", "--set", "keda.order.maxReplicaCount=4")
	md := find(objs, "ScaledObject", "so-couponkill-order-service").m("spec", "triggers", 0, "metadata")
	if md.str("bootstrapServers") != "k1:9092,k2:9092" {
		t.Errorf("外部 Kafka 的 bootstrapServers=%q", md.str("bootstrapServers"))
	}
}

// TestIstioGatewayTLS 配置证书后入口 Gateway 多一个 443 服务器，80 只做跳转；不配置证书时只有 80。
func TestIstioGatewayTLS(t *testing.T) {
	plain := find(renderCombo(t, "istio"), "Gateway", "couponkill-gateway")
	if n := len(plain.list("spec", "servers")); n != 1 {
		t.Fatalf("未配置证书时应只有 1 个服务器，实际 %d", n)
	}
	if plain.get("spec", "servers", 0, "tls") != nil {
		t.Error("未配置证书时 80 端口不应有 tls 设置")
	}

	secure := find(renderCombo(t, "istio", "--set", "istio.gateway.tls.credentialName=couponkill-credential"), "Gateway", "couponkill-gateway")
	servers := secure.list("spec", "servers")
	if len(servers) != 2 {
		t.Fatalf("配置证书后应有 80 + 443 两个服务器，实际 %d", len(servers))
	}
	http, https := obj(servers[0].(map[string]any)), obj(servers[1].(map[string]any))
	if http.str("tls", "httpsRedirect") != "true" || http.str("port", "number") != "80" {
		t.Errorf("80 端口应只做 HTTPS 跳转: %v", http)
	}
	if https.str("port", "number") != "443" || https.str("tls", "mode") != "SIMPLE" || https.str("tls", "credentialName") != "couponkill-credential" {
		t.Errorf("443 服务器配置不对: %v", https)
	}
}

// TestInfraOutsideMesh 数据库 / 缓存 / 消息队列不注入 sidecar：
// k8s-istio 的 STRICT mTLS 只作用于带 sidecar 的 Pod，纯 TCP 的依赖留在网格之外（访问控制交给 NetworkPolicy）。
func TestInfraOutsideMesh(t *testing.T) {
	objs := renderCombo(t, "default")
	for _, name := range []string{"postgres", "redis", "kafka"} {
		sts := find(objs, "StatefulSet", name)
		if sts.str("spec", "template", "metadata", "annotations", "sidecar.istio.io/inject") != "false" {
			t.Errorf("%s 应声明 sidecar.istio.io/inject=false", name)
		}
	}
	// 业务服务必须留在网格里：不能带 inject=false
	for _, dep := range appDeployments(t, objs) {
		if dep.get("spec", "template", "metadata", "annotations", "sidecar.istio.io/inject") != nil {
			t.Errorf("%s 不应关闭 sidecar 注入", dep.name())
		}
	}
}
