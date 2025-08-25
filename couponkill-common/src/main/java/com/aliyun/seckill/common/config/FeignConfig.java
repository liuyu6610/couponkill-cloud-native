// 在 common 模块中完善 FeignConfig.java
package com.aliyun.seckill.common.config;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.datatype.jsr310.JavaTimeModule;
import feign.codec.Decoder;
import feign.codec.Encoder;
import org.springframework.beans.factory.ObjectFactory;
import org.springframework.boot.autoconfigure.http.HttpMessageConverters;
import org.springframework.cloud.openfeign.support.ResponseEntityDecoder;
import org.springframework.cloud.openfeign.support.SpringDecoder;
import org.springframework.cloud.openfeign.support.SpringEncoder;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.context.annotation.Primary;
import org.springframework.http.converter.json.MappingJackson2HttpMessageConverter;

@Configuration
public class FeignConfig {

    @Bean
    @Primary
    public ObjectMapper feignObjectMapper() {
        ObjectMapper objectMapper = new ObjectMapper();
        // 注册 JavaTimeModule 以支持 LocalDateTime 等时间类型
        objectMapper.registerModule(new JavaTimeModule());

        // 启用泛型类型处理
        objectMapper.activateDefaultTyping(
            objectMapper.getPolymorphicTypeValidator(),
            ObjectMapper.DefaultTyping.NON_FINAL
        );
        return objectMapper;
    }

    @Bean
    public Decoder feignDecoder() {
        return new ResponseEntityDecoder(new SpringDecoder(feignHttpMessageConverter()));
    }

    @Bean
    public Encoder feignEncoder() {
        return new SpringEncoder(feignHttpMessageConverter());
    }

    @Bean
    public ObjectFactory<HttpMessageConverters> feignHttpMessageConverter() {
        final MappingJackson2HttpMessageConverter jacksonConverter =
            new MappingJackson2HttpMessageConverter(feignObjectMapper());
        final HttpMessageConverters httpMessageConverters =
            new HttpMessageConverters(jacksonConverter);
        return () -> httpMessageConverters;
    }
}
