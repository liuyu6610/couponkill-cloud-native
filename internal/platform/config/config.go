// Package config 把环境变量加载进带 tag 的结构体，并在加载时完成校验。
//
// 约定（design §3）：全部配置来自 `CK_*` 环境变量；`CK_ENV=prod` 时带开发默认值的密钥
// 必须显式设置且足够长。加载过程一次性收集并报告所有问题，而不是遇到第一个就停。
//
// 字段 tag：
//
//	env:"CK_X"       环境变量名（没有 env tag 的结构体字段会被递归展开，其余字段忽略）
//	default:"..."    环境变量缺失（或为空串）时的取值；对 Secret 字段它就是“开发默认值”
//	required:"true"  最终取值为空则报错
//	oneof:"a,b"      取值必须属于集合
//	min:"N" max:"N"  字符串/切片按长度，数字按值，time.Duration 按时长（如 "1ms"）
package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Lookup 是读取环境变量的函数，签名与 os.LookupEnv 相同。
type Lookup func(key string) (string, bool)

type options struct {
	lookup   Lookup
	defaults map[string]string
}

// Option 调整 Load 的行为。
type Option func(*options)

// WithLookup 用自定义函数替代 os.LookupEnv（测试或从别处读取配置时使用）。
func WithLookup(l Lookup) Option { return func(o *options) { o.lookup = l } }

// WithMap 用一个 map 作为环境（测试常用）。
func WithMap(m map[string]string) Option {
	return WithLookup(func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	})
}

// WithDefaults 按环境变量名覆盖 `default` tag，用于各服务调整公共字段的默认值
// （例如 order 服务把 CK_HTTP_PORT 的默认值设为 8082）。显式设置的环境变量优先于它。
func WithDefaults(m map[string]string) Option { return func(o *options) { o.defaults = m } }

// ValidationError 汇总一次加载中发现的全部问题。
type ValidationError struct {
	// Problems 每项形如 “CK_X: 原因”，不包含任何密钥取值。
	Problems []string
}

// Error 实现 error 接口。
func (e *ValidationError) Error() string {
	return "invalid configuration:\n  - " + strings.Join(e.Problems, "\n  - ")
}

var (
	durationType = reflect.TypeFor[time.Duration]()
	secretType   = reflect.TypeFor[Secret]()
)

// field 是展开后的一个叶子配置项。
type field struct {
	env   string
	value reflect.Value
	sf    reflect.StructField
}

func (f field) isSecret() bool { return f.value.Type() == secretType }

// walk 递归展开结构体，对每个带 env tag 的导出叶子字段调用 fn。
func walk(v reflect.Value, fn func(field)) {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		fv := v.Field(i)
		env, hasEnv := sf.Tag.Lookup("env")
		if !hasEnv {
			if fv.Kind() == reflect.Struct && fv.Type() != durationType {
				walk(fv, fn)
			}
			continue
		}
		fn(field{env: env, value: fv, sf: sf})
	}
}

func structOf(dst any) (reflect.Value, error) {
	rv := reflect.ValueOf(dst)
	if !rv.IsValid() || rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("config: Load needs a non-nil pointer to a struct, got %T", dst)
	}
	return rv.Elem(), nil
}

// Load 读取环境变量填充 dst（必须是指向结构体的指针）。
// 任何解析或校验问题都以 *ValidationError 一次性返回；此时 dst 的内容不应被使用。
func Load(dst any, opts ...Option) error {
	o := options{lookup: os.LookupEnv}
	for _, opt := range opts {
		opt(&o)
	}
	sv, err := structOf(dst)
	if err != nil {
		return err
	}
	prod := false
	if v, ok := o.lookup("CK_ENV"); ok && strings.EqualFold(strings.TrimSpace(v), "prod") {
		prod = true
	}

	var problems []string
	walk(sv, func(f field) {
		problems = append(problems, loadField(f, o, prod)...)
	})
	if len(problems) > 0 {
		return &ValidationError{Problems: problems}
	}
	return nil
}

// loadField 解析并校验单个字段，返回发现的问题（可能为空）。
func loadField(f field, o options, prod bool) []string {
	def, hasDef := f.sf.Tag.Lookup("default")
	if d, ok := o.defaults[f.env]; ok {
		def, hasDef = d, true
	}
	raw, ok := o.lookup(f.env)
	raw = strings.TrimSpace(raw)
	if !ok || raw == "" {
		raw = def
	}
	secret := f.isSecret()
	show := func(s string) string { // 报错里展示取值：密钥永远不展示
		if secret {
			return "<hidden>"
		}
		return strconv.Quote(s)
	}

	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, f.env+": "+fmt.Sprintf(format, args...))
	}

	if raw != "" {
		if err := setValue(f.value, raw); err != nil {
			add("invalid value %s (%v)", show(raw), errKind(err))
			return problems
		}
	}
	empty := raw == ""
	required := f.sf.Tag.Get("required") == "true"
	if empty && required {
		add("is required")
	}
	if secret && prod && (required || hasDef && def != "") {
		switch {
		case empty:
			if !required {
				add("must be set explicitly when CK_ENV=prod")
			}
		case hasDef && raw == def:
			add("must not use the built-in development default when CK_ENV=prod")
		}
	}
	if empty && !isNumeric(f.value) {
		return problems
	}
	if oneof := f.sf.Tag.Get("oneof"); oneof != "" && !empty {
		if !contains(strings.Split(oneof, ","), raw) {
			add("invalid value %s, want one of %s", show(raw), oneof)
		}
	}
	if min := f.sf.Tag.Get("min"); min != "" {
		if err := checkBound(f.value, min, true); err != nil {
			add("%v", err)
		}
	}
	if max := f.sf.Tag.Get("max"); max != "" {
		if err := checkBound(f.value, max, false); err != nil {
			add("%v", err)
		}
	}
	return problems
}

// errKind 去掉 strconv 错误里重复的取值，避免把原始值（可能是密钥）带进报错。
func errKind(err error) string {
	if ne, ok := err.(*strconv.NumError); ok {
		return ne.Err.Error()
	}
	return err.Error()
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if strings.TrimSpace(x) == s {
			return true
		}
	}
	return false
}

func isNumeric(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

// setValue 把字符串解析进 v。
func setValue(v reflect.Value, raw string) error {
	if v.Type() == durationType {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return fmt.Errorf("not a duration")
		}
		v.SetInt(int64(d))
		return nil
	}
	switch v.Kind() {
	case reflect.String:
		v.SetString(raw)
	case reflect.Bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("not a boolean")
		}
		v.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(raw, 10, v.Type().Bits())
		if err != nil {
			return fmt.Errorf("not an integer")
		}
		v.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(raw, 10, v.Type().Bits())
		if err != nil {
			return fmt.Errorf("not an unsigned integer")
		}
		v.SetUint(n)
	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(raw, v.Type().Bits())
		if err != nil {
			return fmt.Errorf("not a number")
		}
		v.SetFloat(n)
	case reflect.Slice:
		if v.Type().Elem().Kind() != reflect.String {
			return fmt.Errorf("unsupported slice type %s", v.Type())
		}
		var items []string
		for _, p := range strings.Split(raw, ",") {
			if p = strings.TrimSpace(p); p != "" {
				items = append(items, p)
			}
		}
		v.Set(reflect.ValueOf(items).Convert(v.Type()))
	default:
		return fmt.Errorf("unsupported type %s", v.Type())
	}
	return nil
}

// checkBound 校验 min（lower=true）或 max（lower=false）。
func checkBound(v reflect.Value, bound string, lower bool) error {
	cmp := func(ok bool, what string, got any) error {
		if ok {
			return nil
		}
		dir := "at most"
		if lower {
			dir = "at least"
		}
		return fmt.Errorf("%s must be %s %s (got %v)", what, dir, bound, got)
	}
	switch {
	case v.Type() == durationType:
		b, err := time.ParseDuration(bound)
		if err != nil {
			return fmt.Errorf("bad bound tag %q", bound)
		}
		d := time.Duration(v.Int())
		return cmp(lower && d >= b || !lower && d <= b, "value", d)
	case v.Kind() == reflect.String || v.Kind() == reflect.Slice:
		b, err := strconv.Atoi(bound)
		if err != nil {
			return fmt.Errorf("bad bound tag %q", bound)
		}
		n := v.Len()
		return cmp(lower && n >= b || !lower && n <= b, "length", n)
	case v.CanInt():
		b, err := strconv.ParseInt(bound, 10, 64)
		if err != nil {
			return fmt.Errorf("bad bound tag %q", bound)
		}
		n := v.Int()
		return cmp(lower && n >= b || !lower && n <= b, "value", n)
	case v.CanUint():
		b, err := strconv.ParseUint(bound, 10, 64)
		if err != nil {
			return fmt.Errorf("bad bound tag %q", bound)
		}
		n := v.Uint()
		return cmp(lower && n >= b || !lower && n <= b, "value", n)
	case v.CanFloat():
		b, err := strconv.ParseFloat(bound, 64)
		if err != nil {
			return fmt.Errorf("bad bound tag %q", bound)
		}
		n := v.Float()
		return cmp(lower && n >= b || !lower && n <= b, "value", n)
	}
	return nil
}

// Warnings 返回仍在使用内置开发默认值的密钥告警（每项形如 “CK_X: ...”），不含取值。
// main 应在日志初始化后逐条以 Warn 级别输出；prod 模式下这种情况在 Load 时就已失败。
func Warnings(cfg any) []string {
	sv, err := structOf(cfg)
	if err != nil {
		return nil
	}
	var out []string
	walk(sv, func(f field) {
		if !f.isSecret() {
			return
		}
		def, has := f.sf.Tag.Lookup("default")
		if has && def != "" && f.value.String() == def {
			out = append(out, f.env+": using the built-in development default; set it explicitly (mandatory when CK_ENV=prod)")
		}
	})
	return out
}

// Summary 返回“环境变量名 → 取值”的脱敏快照，用于启动日志。Secret 字段非空时输出 [REDACTED]。
func Summary(cfg any) map[string]string {
	sv, err := structOf(cfg)
	if err != nil {
		return nil
	}
	out := map[string]string{}
	walk(sv, func(f field) {
		switch {
		case f.isSecret():
			out[f.env] = f.value.Interface().(Secret).String()
		case f.value.Kind() == reflect.Slice:
			parts := make([]string, f.value.Len())
			for i := range parts {
				parts[i] = fmt.Sprint(f.value.Index(i).Interface())
			}
			out[f.env] = strings.Join(parts, ",")
		default:
			out[f.env] = fmt.Sprint(f.value.Interface())
		}
	})
	return out
}
