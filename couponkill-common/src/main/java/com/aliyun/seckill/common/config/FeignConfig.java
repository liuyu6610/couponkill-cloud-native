package com.aliyun.seckill.common.config;

import com.aliyun.seckill.common.context.UserContext;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.datatype.jsr310.JavaTimeModule;
import feign.RequestInterceptor;
import feign.codec.Decoder;
import feign.codec.Encoder;
import org.springframework.beans.BeansException;
import org.springframework.beans.factory.ObjectProvider;
import org.springframework.cloud.openfeign.support.FeignHttpMessageConverters;
import org.springframework.cloud.openfeign.support.HttpMessageConverterCustomizer;
import org.springframework.cloud.openfeign.support.ResponseEntityDecoder;
import org.springframework.cloud.openfeign.support.SpringDecoder;
import org.springframework.cloud.openfeign.support.SpringEncoder;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.context.annotation.Primary;
import org.springframework.http.converter.HttpMessageConverter;
import org.springframework.http.converter.json.MappingJackson2HttpMessageConverter;

import java.util.Iterator;
import java.util.List;
import java.util.function.Consumer;
import java.util.function.Supplier;
import java.util.stream.Stream;

/**
 * OpenFeign 公共配置（对齐 Spring Cloud OpenFeign 5 / Boot 4）。
 */
@Configuration
public class FeignConfig {

    @Bean
    @Primary
    public ObjectMapper feignObjectMapper() {
        ObjectMapper objectMapper = new ObjectMapper();
        objectMapper.registerModule(new JavaTimeModule());
        return objectMapper;
    }

    /** 将网关身份透传到下游（尤其 Go 秒杀），避免 body 伪造 userId */
    @Bean
    public RequestInterceptor userContextFeignInterceptor() {
        return template -> {
            String userId = UserContext.getCurrentUserId();
            if (userId != null && !userId.isBlank()) {
                template.header(UserContext.USER_ID_HEADER, userId);
                template.header(UserContext.AUTHENTICATED_HEADER, "true");
            }
        };
    }

    @Bean
    public FeignHttpMessageConverters feignHttpMessageConverters() {
        MappingJackson2HttpMessageConverter jackson =
                new MappingJackson2HttpMessageConverter(feignObjectMapper());
        ObjectProvider<HttpMessageConverter<?>> converters = singletonProvider(jackson);
        ObjectProvider<HttpMessageConverterCustomizer> customizers = emptyProvider();
        return new FeignHttpMessageConverters(converters, customizers);
    }

    @Bean
    public Decoder feignDecoder(ObjectProvider<FeignHttpMessageConverters> messageConverters) {
        return new ResponseEntityDecoder(new SpringDecoder(messageConverters));
    }

    @Bean
    public Encoder feignEncoder(ObjectProvider<FeignHttpMessageConverters> messageConverters) {
        return new SpringEncoder(messageConverters);
    }

    private static <T> ObjectProvider<T> singletonProvider(T value) {
        return new ObjectProvider<>() {
            @Override
            public T getObject() throws BeansException {
                return value;
            }

            @Override
            public T getObject(Object... args) throws BeansException {
                return value;
            }

            @Override
            public T getIfAvailable() throws BeansException {
                return value;
            }

            @Override
            public T getIfAvailable(Supplier<T> defaultSupplier) throws BeansException {
                return value;
            }

            @Override
            public void ifAvailable(Consumer<T> dependencyConsumer) throws BeansException {
                dependencyConsumer.accept(value);
            }

            @Override
            public T getIfUnique() throws BeansException {
                return value;
            }

            @Override
            public T getIfUnique(Supplier<T> defaultSupplier) throws BeansException {
                return value;
            }

            @Override
            public void ifUnique(Consumer<T> dependencyConsumer) throws BeansException {
                dependencyConsumer.accept(value);
            }

            @Override
            public Stream<T> stream() {
                return Stream.of(value);
            }

            @Override
            public Stream<T> orderedStream() {
                return Stream.of(value);
            }

            @Override
            public Iterator<T> iterator() {
                return List.of(value).iterator();
            }
        };
    }

    private static <T> ObjectProvider<T> emptyProvider() {
        return new ObjectProvider<>() {
            @Override
            public T getObject() throws BeansException {
                throw new BeansException("empty") {};
            }

            @Override
            public T getObject(Object... args) throws BeansException {
                throw new BeansException("empty") {};
            }

            @Override
            public T getIfAvailable() {
                return null;
            }

            @Override
            public T getIfUnique() {
                return null;
            }

            @Override
            public Stream<T> stream() {
                return Stream.empty();
            }

            @Override
            public Stream<T> orderedStream() {
                return Stream.empty();
            }
        };
    }
}
