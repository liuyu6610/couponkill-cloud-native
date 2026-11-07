package render

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// overlayDocs 解析 k8s-istio/<file> 里的全部 YAML 文档。
func overlayDocs(t *testing.T, file string) []obj {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoDir(t), "k8s-istio", file))
	if err != nil {
		t.Fatalf("读取 k8s-istio/%s 失败: %v", file, err)
	}
	return parseDocs(t, b)
}

var principalRe = regexp.MustCompile(`^cluster\.local/ns/([^/]+)/sa/([^/]+)$`)

// TestIstioOverlayMatchesChart k8s-istio 里的授权策略必须与 chart 渲染出的对象一致：
// 每条策略的 selector 能选中真实存在的 Deployment，每个 principal 对应真实存在的 ServiceAccount
// （入口网关的 istio-system 账户除外），并且五个服务各有一条策略（gateway 经 gateway-policy）。
func TestIstioOverlayMatchesChart(t *testing.T) {
	objs := renderCombo(t, "istio")
	apps := map[string]bool{}
	for _, dep := range appDeployments(t, objs) {
		apps[dep.str("spec", "template", "metadata", "labels", "app")] = true
	}

	policies := overlayDocs(t, "istio_AuthorizationPolicy.yaml")
	covered := map[string]bool{}
	for _, p := range policies {
		if p.kind() != "AuthorizationPolicy" {
			t.Errorf("意外的对象 %s/%s", p.kind(), p.name())
			continue
		}
		if p.str("metadata", "namespace") != testNamespace {
			t.Errorf("%s 命名空间应为 %s", p.name(), testNamespace)
		}
		if app := p.str("spec", "selector", "matchLabels", "app"); app != "" {
			if !apps[app] {
				t.Errorf("%s 选择的 app=%s 在 chart 里不存在", p.name(), app)
			}
			if p.str("spec", "action") == "ALLOW" {
				covered[app] = true
			}
		}
		for _, rule := range p.list("spec", "rules") {
			ro := obj(rule.(map[string]any))
			for _, from := range ro.list("from") {
				for _, pr := range obj(from.(map[string]any)).list("source", "principals") {
					m := principalRe.FindStringSubmatch(pr.(string))
					if m == nil {
						t.Errorf("%s 的 principal %q 格式不对", p.name(), pr)
						continue
					}
					if m[1] == "istio-system" {
						continue
					}
					if m[1] != testNamespace || find(objs, "ServiceAccount", m[2]) == nil {
						t.Errorf("%s 的 principal %q 在 chart 里没有对应的 ServiceAccount", p.name(), pr)
					}
				}
			}
		}
	}
	for _, sp := range serviceSpecs {
		if !covered[sp.name] {
			t.Errorf("服务 %s 没有 ALLOW 授权策略", sp.name)
		}
	}
	// 入口网关的账户只能出现在 gateway-policy 里
	for _, p := range policies {
		if p.name() == "gateway-policy" {
			continue
		}
		for _, rule := range p.list("spec", "rules") {
			for _, from := range obj(rule.(map[string]any)).list("from") {
				for _, pr := range obj(from.(map[string]any)).list("source", "principals") {
					if strings.Contains(pr.(string), "istio-ingressgateway") {
						t.Errorf("%s 不应允许入口网关直接访问业务服务", p.name())
					}
				}
			}
		}
	}
}

// TestIstioOverlayInternalPaths 内部接口只对设计文档 §5/§10 列出的调用方开放，gateway 的身份被 DENY 访问 /internal/*。
func TestIstioOverlayInternalPaths(t *testing.T) {
	var deny obj
	allowed := map[string]map[string][]string{} // 目标 app → 调用方 SA → 允许的路径
	for _, p := range overlayDocs(t, "istio_AuthorizationPolicy.yaml") {
		if p.str("spec", "action") == "DENY" {
			deny = p
		}
		app := p.str("spec", "selector", "matchLabels", "app")
		for _, rule := range p.list("spec", "rules") {
			ro := obj(rule.(map[string]any))
			var paths []string
			for _, to := range ro.list("to") {
				for _, pa := range obj(to.(map[string]any)).list("operation", "paths") {
					paths = append(paths, pa.(string))
				}
			}
			for _, from := range ro.list("from") {
				for _, pr := range obj(from.(map[string]any)).list("source", "principals") {
					if allowed[app] == nil {
						allowed[app] = map[string][]string{}
					}
					sa := principalRe.FindStringSubmatch(pr.(string))
					if sa != nil {
						allowed[app][sa[2]] = append(allowed[app][sa[2]], paths...)
					}
				}
			}
		}
	}
	if deny == nil {
		t.Fatal("缺少 deny-internal-from-gateway")
	}
	if deny.str("spec", "rules", 0, "from", 0, "source", "principals", 0) != "cluster.local/ns/couponkill/sa/couponkill-gateway" ||
		deny.str("spec", "rules", 0, "to", 0, "operation", "paths", 0) != "/internal/*" {
		t.Errorf("DENY 策略应拒绝 gateway 访问 /internal/*: %v", deny.m("spec"))
	}

	// 设计文档里的内部调用关系：调用方 → (被调用方, 路径)
	want := []struct{ caller, callee, path string }{
		{"couponkill-order-service", "couponkill-coupon-service", "/internal/v1/stock/*"},
		{"couponkill-order-service", "couponkill-coupon-service", "/internal/v1/coupons/*"},
		{"couponkill-connector-service", "couponkill-coupon-service", "/internal/v1/stock/sync"},
		{"couponkill-coupon-service", "couponkill-order-service", "/internal/v1/orders/grant"},
	}
	for _, w := range want {
		found := false
		for _, p := range allowed[w.callee][w.caller] {
			if p == w.path {
				found = true
			}
		}
		if !found {
			t.Errorf("%s → %s 的 %s 没有被授权", w.caller, w.callee, w.path)
		}
	}
	// user 与 connector 没有任何服务间调用方（internalapi.UserClient 未被使用，connector 只往外调）：只有 gateway 能到达
	for _, callee := range []string{"couponkill-user-service", "couponkill-connector-service"} {
		for caller := range allowed[callee] {
			if caller != "couponkill-gateway" {
				t.Errorf("%s 不应被 %s 直接访问", callee, caller)
			}
		}
	}
	// 其它服务调用方只能带路径限制：没有路径的规则只属于 gateway
	for callee, callers := range allowed {
		for caller, paths := range callers {
			if caller != "couponkill-gateway" && caller != "istio-ingressgateway-service-account" && len(paths) == 0 {
				t.Errorf("%s 对 %s 的授权没有路径限制", caller, callee)
			}
		}
	}
}

// TestIstioOverlayParses k8s-istio 下每个文件都是合法 YAML，且对象落在 couponkill 命名空间（命名空间对象本身除外）。
func TestIstioOverlayParses(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(repoDir(t), "k8s-istio", "*.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("没有找到 k8s-istio/*.yaml: %v", err)
	}
	for _, f := range files {
		for _, o := range overlayDocs(t, filepath.Base(f)) {
			if o.kind() == "Namespace" {
				continue
			}
			if ns := o.str("metadata", "namespace"); ns != testNamespace {
				t.Errorf("%s: %s/%s 在命名空间 %q，期望 %s", filepath.Base(f), o.kind(), o.name(), ns, testNamespace)
			}
		}
	}
}
