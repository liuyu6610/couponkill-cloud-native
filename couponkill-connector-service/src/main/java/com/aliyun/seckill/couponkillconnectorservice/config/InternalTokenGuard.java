package com.aliyun.seckill.couponkillconnectorservice.config;

import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.ApplicationArguments;
import org.springframework.boot.ApplicationRunner;
import org.springframework.core.env.Environment;
import org.springframework.stereotype.Component;

@Slf4j
@Component
public class InternalTokenGuard implements ApplicationRunner {

    private static final String DEFAULT_TOKEN = "couponkill-internal";

    @Value("${connector.internal-token:${CONNECTOR_INTERNAL_TOKEN:couponkill-internal}}")
    private String internalToken;

    @Value("${connector.internal-token-strict:false}")
    private boolean strict;

    private final Environment environment;

    public InternalTokenGuard(Environment environment) {
        this.environment = environment;
    }

    @Override
    public void run(ApplicationArguments args) {
        boolean prod = false;
        for (String p : environment.getActiveProfiles()) {
            if ("prod".equalsIgnoreCase(p) || "production".equalsIgnoreCase(p)) {
                prod = true;
                break;
            }
        }
        boolean isDefault = internalToken == null || internalToken.isBlank() || DEFAULT_TOKEN.equals(internalToken);
        if (isDefault && (prod || strict)) {
            throw new IllegalStateException(
                    "connector.internal-token 不能使用默认弱口令，请通过 CONNECTOR_INTERNAL_TOKEN 注入强随机值");
        }
        if (isDefault) {
            log.warn("【安全】connector.internal-token 仍为默认值，仅限本地联调");
        }
    }
}
