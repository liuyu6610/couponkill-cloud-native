package com.aliyun.seckill.couponkillconnectorservice.config;

import lombok.Data;
import org.springframework.boot.context.properties.ConfigurationProperties;

@Data
@ConfigurationProperties(prefix = "connector.jd")
public class JdConnectorProperties {
    private boolean enabled;
    private String serverUrl = "https://api.jd.com/routerjson";
    private String appKey = "";
    private String appSecret = "";
    private String accessToken = "";
    private String defaultArea = "1_72_2799_0";

    public boolean credentialsPresent() {
        return enabled
                && appKey != null && !appKey.isBlank()
                && appSecret != null && !appSecret.isBlank()
                && accessToken != null && !accessToken.isBlank();
    }
}
