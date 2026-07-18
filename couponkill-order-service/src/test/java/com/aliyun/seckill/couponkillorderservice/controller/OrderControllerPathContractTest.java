package com.aliyun.seckill.couponkillorderservice.controller;

import com.aliyun.seckill.common.config.ServiceGoConfig;
import com.aliyun.seckill.common.context.UserContext;
import com.aliyun.seckill.common.pojo.Order;
import com.aliyun.seckill.couponkillorderservice.feign.GoSeckillFeignClient;
import com.aliyun.seckill.couponkillorderservice.service.AsyncSeckillEnterService;
import com.aliyun.seckill.couponkillorderservice.service.OrderService;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.InjectMocks;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;
import org.springframework.http.MediaType;
import org.springframework.test.web.servlet.MockMvc;
import org.springframework.test.web.servlet.setup.MockMvcBuilders;

import java.util.List;

import static org.mockito.ArgumentMatchers.anyInt;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

/**
 * Phase1+2 契约：新旧路径均可达，且响应为 ApiResponse(code=0)。
 */
@ExtendWith(MockitoExtension.class)
class OrderControllerPathContractTest {

    @Mock
    private OrderService orderService;
    @Mock
    private GoSeckillFeignClient goSeckillFeignClient;
    @Mock
    private ServiceGoConfig serviceGoConfig;
    @Mock
    private AsyncSeckillEnterService asyncSeckillEnterService;

    @InjectMocks
    private OrderController orderController;

    private MockMvc mockMvc;

    @BeforeEach
    void setUp() {
        mockMvc = MockMvcBuilders.standaloneSetup(orderController).build();
        Order sample = new Order();
        sample.setId("1");
        sample.setUserId(10000L);
        when(orderService.getOrderByUserId(eq(10000L), anyInt(), anyInt())).thenReturn(List.of(sample));
    }

    @Test
    void legacyOrderPath_returnsApiResponseCodeZero() throws Exception {
        mockMvc.perform(get("/order/user/me")
                        .header(UserContext.USER_ID_HEADER, "10000")
                        .accept(MediaType.APPLICATION_JSON))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.code").value(0))
                .andExpect(jsonPath("$.data[0].userId").value(10000));
    }

    @Test
    void apiV1OrderPath_returnsApiResponseCodeZero() throws Exception {
        mockMvc.perform(get("/api/v1/order/user/me")
                        .header(UserContext.USER_ID_HEADER, "10000")
                        .accept(MediaType.APPLICATION_JSON))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.code").value(0))
                .andExpect(jsonPath("$.data[0].userId").value(10000));
    }
}
