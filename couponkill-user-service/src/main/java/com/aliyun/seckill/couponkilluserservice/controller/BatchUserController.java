package com.aliyun.seckill.couponkilluserservice.controller;

import com.aliyun.seckill.common.api.ApiResponse;
import com.aliyun.seckill.common.pojo.User;
import com.aliyun.seckill.couponkilluserservice.util.BatchUserInserter;
import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.Parameter;
import io.swagger.v3.oas.annotations.tags.Tag;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.web.bind.annotation.*;

import java.util.List;

/**
 * 批量用户操作控制器
 */
@Slf4j
@Tag(name = "批量用户管理", description = "批量用户相关操作接口")
@RestController
@RequestMapping("/api/v1/user/batch")
public class BatchUserController {

    @Autowired
    private BatchUserInserter batchUserInserter;

    @Operation(summary = "批量生成测试用户", description = "根据指定范围批量生成测试用户")
    @PostMapping("/generate")
    public ApiResponse<String> generateTestUsers(
            @Parameter(description = "起始用户ID") @RequestParam Long startId,
            @Parameter(description = "用户数量") @RequestParam Integer count) {
        
        if (startId == null || startId <= 0) {
            return ApiResponse.fail(400, "起始用户ID必须大于0");
        }
        
        if (count == null || count <= 0 || count > 100000) {
            return ApiResponse.fail(400, "用户数量必须大于0且不超过100000");
        }

        try {
            List<User> users = batchUserInserter.generateTestUsers(startId, count);
            batchUserInserter.batchInsertUsers(users);
            return ApiResponse.success("成功生成并插入 " + count + " 个测试用户");
        } catch (Exception e) {
            log.error("批量生成测试用户失败: startId={}, count={}", startId, count, e);
            return ApiResponse.fail(500, "批量生成测试用户失败: " + e.getMessage());
        }
    }
}