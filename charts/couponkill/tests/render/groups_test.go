package render

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var embeddedGroupRe = regexp.MustCompile(`(?m)^\s+config\.(Common|Postgres|Redis|Kafka|JWT|Internal|GatewaySign|Peers)\s*$`)

// embeddedGroups 读取 internal/<svc>/ 下（仅该目录本层）非测试 Go 文件里 Config 结构体内嵌的 platform/config 配置组。
func embeddedGroups(t *testing.T, svc string) []string {
	t.Helper()
	dir := filepath.Join(repoDir(t), "internal", svc)
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	set := map[string]bool{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range embeddedGroupRe.FindAllStringSubmatch(string(b), -1) {
			set[m[1]] = true
		}
	}
	var out []string
	for g := range set {
		out = append(out, g)
	}
	sort.Strings(out)
	return out
}

// TestChartGroupsMatchGoConfig chart 给每个服务注入的配置组，必须与该服务 Go 源码里 Config 内嵌的 platform/config 组完全一致：
// 少了会让服务落回开发默认值（prod 下直接启动失败），多了就是把不需要的机密发给了它。
// 服务目录里还没有 Config 时跳过。
func TestChartGroupsMatchGoConfig(t *testing.T) {
	for _, sp := range serviceSpecs {
		t.Run(sp.key, func(t *testing.T) {
			got := embeddedGroups(t, sp.key)
			if len(got) == 0 {
				t.Skipf("internal/%s 里还没有内嵌 platform/config 组的 Config", sp.key)
			}
			want := append([]string{}, sp.platform...)
			sort.Strings(want)
			if strings.Join(got, ",") != strings.Join(want, ",") {
				t.Errorf("%s：Go 配置内嵌的组是 %v，chart 的期望表是 %v（env_test.go 的 serviceSpecs 与 templates/_helpers.tpl 的 couponkill.caps 需要同步）", sp.key, got, want)
			}
		})
	}
}
