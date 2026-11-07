package config

import "log/slog"

// Report 在日志初始化之后调用：以 Info 输出脱敏后的配置快照，并对仍在使用开发默认密钥的
// 字段逐条输出 Warn。Load 本身不打日志，因为日志级别要等配置加载完才知道。
func Report(l *slog.Logger, cfg any) {
	l.Info("configuration loaded", "config", Summary(cfg))
	for _, w := range Warnings(cfg) {
		l.Warn("insecure configuration", "detail", w)
	}
}
