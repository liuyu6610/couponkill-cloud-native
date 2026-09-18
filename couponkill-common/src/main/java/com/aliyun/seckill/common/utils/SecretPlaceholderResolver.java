package com.aliyun.seckill.common.utils;

import java.util.regex.Matcher;
import java.util.regex.Pattern;

/**
 * 将配置文本中的 {@code ${ENV}} / {@code ${ENV:default}} 替换为进程环境变量。
 * 用于 Nacos 下发的 ShardingSphere YAML 等非 Spring 解析路径，避免把真实口令写进配置中心。
 */
public final class SecretPlaceholderResolver {

    private SecretPlaceholderResolver() {
    }

    /**
     * 解析常见密钥占位符。未设置环境变量时：有 default 则用 default，否则保留原占位符（失败关闭）。
     */
    public static String resolve(String content) {
        if (content == null || content.isEmpty()) {
            return content;
        }
        String postgres = firstEnv("POSTGRES_PASSWORD", "SPRING_DATASOURCE_PASSWORD");
        String jwt = firstEnv("JWT_SECRET");
        String internal = firstEnv("CONNECTOR_INTERNAL_TOKEN");
        String out = content;
        out = replaceNamed(out, "POSTGRES_PASSWORD", postgres);
        out = replaceNamed(out, "JWT_SECRET", jwt);
        out = replaceNamed(out, "CONNECTOR_INTERNAL_TOKEN", internal);
        return out;
    }

    private static String firstEnv(String... names) {
        for (String name : names) {
            String v = System.getenv(name);
            if (v != null && !v.isEmpty()) {
                return v;
            }
        }
        return null;
    }

    private static String replaceNamed(String content, String name, String value) {
        Pattern p = Pattern.compile("\\$\\{" + Pattern.quote(name) + "(?::([^}]*))?\\}");
        Matcher m = p.matcher(content);
        StringBuffer sb = new StringBuffer();
        while (m.find()) {
            String def = m.group(1);
            String replacement;
            if (value != null && !value.isEmpty()) {
                replacement = value;
            } else if (def != null) {
                replacement = def;
            } else {
                replacement = m.group(0);
            }
            m.appendReplacement(sb, Matcher.quoteReplacement(replacement));
        }
        m.appendTail(sb);
        return sb.toString();
    }
}
