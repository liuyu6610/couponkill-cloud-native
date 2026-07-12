package com.aliyun.seckill.couponkillconnectorservice.connector.jd;

import com.aliyun.seckill.couponkillconnectorservice.config.JdConnectorProperties;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.http.MediaType;
import org.springframework.http.client.JdkClientHttpRequestFactory;
import org.springframework.stereotype.Component;
import org.springframework.util.LinkedMultiValueMap;
import org.springframework.util.MultiValueMap;
import org.springframework.web.client.RestClient;

import java.net.http.HttpClient;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.time.Duration;
import java.time.ZoneId;
import java.time.ZonedDateTime;
import java.time.format.DateTimeFormatter;
import java.util.Map;
import java.util.TreeMap;

/**
 * 京东开放平台 JOS 签名 HTTP 客户端（不依赖官方 SDK）。
 */
@Slf4j
@Component
@RequiredArgsConstructor
public class JdJosClient {

    private static final DateTimeFormatter TS = DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm:ss");
    private static final ZoneId JD_ZONE = ZoneId.of("Asia/Shanghai");
    private static final Duration CONNECT_TIMEOUT = Duration.ofSeconds(5);
    private static final Duration READ_TIMEOUT = Duration.ofSeconds(10);

    private final JdConnectorProperties props;
    private final ObjectMapper objectMapper = new ObjectMapper();
    private final RestClient restClient = buildClient();

    private static RestClient buildClient() {
        HttpClient httpClient = HttpClient.newBuilder()
                .connectTimeout(CONNECT_TIMEOUT)
                .build();
        JdkClientHttpRequestFactory factory = new JdkClientHttpRequestFactory(httpClient);
        factory.setReadTimeout(READ_TIMEOUT);
        return RestClient.builder().requestFactory(factory).build();
    }

    public JsonNode invoke(String method, Map<String, String> bizParams) {
        if (!props.credentialsPresent()) {
            throw new IllegalStateException("京东 Connector 未配置或未启用（需要 appKey/appSecret/accessToken）");
        }

        TreeMap<String, String> sys = new TreeMap<>();
        sys.put("method", method);
        sys.put("app_key", props.getAppKey());
        sys.put("access_token", props.getAccessToken());
        // 京东签名时间戳使用北京时间，避免 UTC 机器签名失败
        sys.put("timestamp", ZonedDateTime.now(JD_ZONE).format(TS));
        sys.put("format", "json");
        sys.put("v", "2.0");
        sys.put("sign_method", "md5");

        StringBuilder paramJson = new StringBuilder("{");
        boolean first = true;
        if (bizParams != null) {
            for (Map.Entry<String, String> e : bizParams.entrySet()) {
                if (!first) {
                    paramJson.append(',');
                }
                first = false;
                paramJson.append('"').append(escape(e.getKey())).append('"')
                        .append(':').append('"').append(escape(e.getValue())).append('"');
            }
        }
        paramJson.append('}');
        sys.put("360buy_param_json", paramJson.toString());
        sys.put("sign", sign(props.getAppSecret(), sys));

        MultiValueMap<String, String> form = new LinkedMultiValueMap<>();
        sys.forEach(form::add);

        String body = restClient.post()
                .uri(props.getServerUrl())
                .contentType(MediaType.APPLICATION_FORM_URLENCODED)
                .body(form)
                .retrieve()
                .body(String.class);
        log.debug("JD JOS response method={} body={}", method, body);

        try {
            return objectMapper.readTree(body == null ? "{}" : body);
        } catch (Exception e) {
            throw new IllegalStateException("解析京东响应失败: " + body, e);
        }
    }

    private static String sign(String secret, TreeMap<String, String> params) {
        StringBuilder sb = new StringBuilder(secret);
        for (Map.Entry<String, String> e : params.entrySet()) {
            if ("sign".equals(e.getKey())) {
                continue;
            }
            sb.append(e.getKey()).append(e.getValue());
        }
        sb.append(secret);
        return md5Upper(sb.toString());
    }

    private static String md5Upper(String raw) {
        try {
            MessageDigest md = MessageDigest.getInstance("MD5");
            byte[] digest = md.digest(raw.getBytes(StandardCharsets.UTF_8));
            StringBuilder hex = new StringBuilder();
            for (byte b : digest) {
                String h = Integer.toHexString(b & 0xff);
                if (h.length() == 1) {
                    hex.append('0');
                }
                hex.append(h);
            }
            return hex.toString().toUpperCase();
        } catch (Exception e) {
            throw new IllegalStateException("MD5 签名失败", e);
        }
    }

    private static String escape(String s) {
        if (s == null) {
            return "";
        }
        return s.replace("\\", "\\\\").replace("\"", "\\\"");
    }
}
