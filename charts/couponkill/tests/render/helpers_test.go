// Package render 对 charts/couponkill 的 helm template 输出做离线断言（不连集群）。
//
// 运行：cd charts/couponkill/tests/render && go test ./...
// 需要 PATH 上有 helm（没有时整体 Skip，CI 的 helm-chart 作业会安装它）。
package render

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// testNamespace 是所有渲染使用的命名空间（helm template -n）。
const testNamespace = "couponkill"

// obj 是一份渲染出的 Kubernetes 对象。
type obj map[string]any

// combo 是一个需要能渲染通过的 values 组合（CI / 发布前都会用到）。
type combo struct {
	name string
	args []string
}

// valueCombos 是全部要覆盖的 values 组合。-f 的路径相对 chart 目录。
var valueCombos = []combo{
	{"default", nil},
	{"prod", []string{"-f", "values-prod.yaml", "--set-string", "image.tag=42"}},
	{"canary-keda", []string{"-f", "values.canary-keda.yaml"}},
	{"prod+canary-keda", []string{"-f", "values-prod.yaml", "-f", "values.canary-keda.yaml", "--set-string", "image.tag=42"}},
	{"istio", []string{"--set", "istio.enabled=true"}},
	{"ingress", []string{"--set", "ingress.enabled=true", "--set", "ingress.tls.secretName=tls-couponkill"}},
	{"monitoring", []string{"--set", "monitoring.enabled=true"}},
	{"external-deps", []string{
		"--set", "postgres.enabled=false", "--set", "postgres.host=pg.example.internal",
		"--set", "redis.enabled=false", "--set", "redis.host=redis.example.internal",
		"--set", "kafka.enabled=false", "--set", "kafka.brokers=k1:9092\\,k2:9092",
	}},
	{"bare", []string{
		"--set", "networkPolicy.enabled=false", "--set", "pdb.enabled=false",
		"--set", "postgres.persistence.enabled=false", "--set", "redis.persistence.enabled=false", "--set", "redis.auth.enabled=false",
		"--set", "kafka.persistence.enabled=false", "--set", "kafka.topicInit.enabled=false",
	}},
	{"operator", []string{"--set", "operator.enabled=true", "--set", "examples.seckill=true"}},
	{"existing-secret", []string{"--set", "secrets.create=false", "--set", "secrets.existingSecret=my-secret", "--set", "postgres.dsnFromSecret=true"}},
}

// comboArgs 按名字取 values 组合参数。
func comboArgs(t *testing.T, name string) []string {
	t.Helper()
	for _, c := range valueCombos {
		if c.name == name {
			return c.args
		}
	}
	t.Fatalf("没有名为 %q 的 values 组合", name)
	return nil
}

// chartDir 返回 charts/couponkill 的绝对路径。
func chartDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("定位 chart 目录失败: %v", err)
	}
	return dir
}

// repoDir 返回仓库根目录。
func repoDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(chartDir(t), "..", "..")
}

// helmArgs 把相对 chart 目录的 -f 路径展开成绝对路径。
func helmArgs(t *testing.T, args []string) []string {
	t.Helper()
	dir := chartDir(t)
	full := []string{"template", "t", dir, "-n", testNamespace}
	for i := 0; i < len(args); i++ {
		full = append(full, args[i])
		if args[i] == "-f" && i+1 < len(args) {
			i++
			full = append(full, filepath.Join(dir, args[i]))
		}
	}
	return full
}

// helmTemplate 执行 helm template，返回标准输出与执行错误（含 stderr）。
func helmTemplate(t *testing.T, args ...string) ([]byte, error) {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("PATH 上没有 helm，跳过渲染断言")
	}
	full := helmArgs(t, args)
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("helm", full...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("helm %s: %w\n%s", strings.Join(full, " "), err, stderr.String())
	}
	return stdout.Bytes(), nil
}

// renderChart 执行 helm template 并解析为对象列表；失败即终止测试。
func renderChart(t *testing.T, args ...string) []obj {
	t.Helper()
	out, err := helmTemplate(t, args...)
	if err != nil {
		t.Fatalf("%v", err)
	}
	return parseDocs(t, out)
}

// renderCombo 渲染命名的 values 组合，并在其上追加额外参数。
func renderCombo(t *testing.T, name string, extra ...string) []obj {
	t.Helper()
	args := append(append([]string{}, comboArgs(t, name)...), extra...)
	return renderChart(t, args...)
}

// expectRenderError 断言渲染失败且错误里含 want。
func expectRenderError(t *testing.T, want string, args ...string) {
	t.Helper()
	out, err := helmTemplate(t, args...)
	if err == nil {
		t.Fatalf("期望渲染失败（含 %q），但成功了，输出 %d 字节", want, len(out))
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("渲染失败，但错误里没有 %q:\n%v", want, err)
	}
}

// parseDocs 把多文档 YAML 解析为对象列表，跳过空文档。
func parseDocs(t *testing.T, data []byte) []obj {
	t.Helper()
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var out []obj
	for i := 0; ; i++ {
		var m map[string]any
		err := dec.Decode(&m)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("第 %d 个 YAML 文档解析失败: %v", i, err)
		}
		if m != nil {
			out = append(out, obj(m))
		}
	}
	return out
}

// get 按 key 路径取值，路径中的整数下标用于数组。
func (o obj) get(path ...any) any {
	var cur any = map[string]any(o)
	for _, p := range path {
		switch k := p.(type) {
		case string:
			m, ok := cur.(map[string]any)
			if !ok {
				return nil
			}
			cur = m[k]
		case int:
			s, ok := cur.([]any)
			if !ok || k >= len(s) {
				return nil
			}
			cur = s[k]
		}
	}
	return cur
}

// str 取路径上的值并格式化为字符串，缺失返回空串。
func (o obj) str(path ...any) string {
	v := o.get(path...)
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

// m 取路径上的映射（缺失返回 nil）。
func (o obj) m(path ...any) obj {
	v, _ := o.get(path...).(map[string]any)
	return obj(v)
}

// list 取路径上的数组（缺失返回 nil）。
func (o obj) list(path ...any) []any {
	v, _ := o.get(path...).([]any)
	return v
}

func (o obj) kind() string { return o.str("kind") }
func (o obj) name() string { return o.str("metadata", "name") }

// find 返回第一个 kind/name 匹配的对象。
func find(objs []obj, kind, name string) obj {
	for _, o := range objs {
		if o.kind() == kind && o.name() == name {
			return o
		}
	}
	return nil
}

// ofKind 返回指定 kind 的全部对象。
func ofKind(objs []obj, kind string) []obj {
	var out []obj
	for _, o := range objs {
		if o.kind() == kind {
			out = append(out, o)
		}
	}
	return out
}

// podSpec 返回工作负载的 Pod spec（Deployment / StatefulSet / Job）。
func podSpec(o obj) obj {
	return o.m("spec", "template", "spec")
}

// container 返回工作负载 Pod 里第一个容器。
func container(o obj) obj {
	if ps := podSpec(o); ps != nil {
		if cs := ps.list("containers"); len(cs) > 0 {
			c, _ := cs[0].(map[string]any)
			return obj(c)
		}
	}
	return nil
}

// envEntry 是容器 env 里的一项（取值或 secretKeyRef）。
type envEntry struct {
	Value      string
	SecretName string
	SecretKey  string
	Optional   bool
}

// containerEnv 返回容器 env（保持顺序的名字列表 + 按名字索引）。
func containerEnv(c obj) ([]string, map[string]envEntry) {
	var order []string
	m := map[string]envEntry{}
	for _, e := range c.list("env") {
		em, _ := e.(map[string]any)
		eo := obj(em)
		name := eo.str("name")
		order = append(order, name)
		ref := eo.m("valueFrom", "secretKeyRef")
		m[name] = envEntry{
			Value:      eo.str("value"),
			SecretName: ref.str("name"),
			SecretKey:  ref.str("key"),
			Optional:   ref.str("optional") == "true",
		}
	}
	return order, m
}

// configMapData 返回 ConfigMap 的 data。
func configMapData(cm obj) map[string]string {
	out := map[string]string{}
	if d := cm.m("data"); d != nil {
		for k, v := range d {
			out[k] = fmt.Sprint(v)
		}
	}
	return out
}

// sortedKeys 返回映射的有序键。
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

var envTagRe = regexp.MustCompile(`env:"(CK_[A-Z0-9_]+)"`)

// goEnvTags 读取 Go 源码文件里 `env:"CK_..."` 标签，按所在 struct 类型分组（struct 名 → 变量名）。
func goEnvTags(t *testing.T, path string) map[string][]string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", path, err)
	}
	structRe := regexp.MustCompile(`^type (\w+) struct \{`)
	out := map[string][]string{}
	cur := ""
	for _, line := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		if m := structRe.FindStringSubmatch(line); m != nil {
			cur = m[1]
			continue
		}
		if strings.HasPrefix(line, "}") {
			cur = ""
			continue
		}
		if cur == "" {
			continue
		}
		for _, m := range envTagRe.FindAllStringSubmatch(line, -1) {
			out[cur] = append(out[cur], m[1])
		}
	}
	return out
}

// allGoEnvVars 收集 internal/ 下全部非测试 Go 文件里出现的 CK_* 环境变量，
// 以及按服务目录（internal/<svc>）分别有没有声明过环境变量。
func allGoEnvVars(t *testing.T) (all map[string]bool, perService map[string]bool) {
	t.Helper()
	all = map[string]bool{}
	perService = map[string]bool{}
	root := filepath.Join(repoDir(t), "internal")
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		found := envTagRe.FindAllStringSubmatch(string(b), -1)
		for _, m := range found {
			all[m[1]] = true
		}
		if len(found) > 0 {
			rel, _ := filepath.Rel(root, path)
			svc := strings.SplitN(filepath.ToSlash(rel), "/", 2)[0]
			perService[svc] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("扫描 internal/ 失败: %v", err)
	}
	return all, perService
}
