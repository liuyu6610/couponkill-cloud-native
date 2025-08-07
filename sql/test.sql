create database coupon;
       use coupon;
           -- 用户表
CREATE TABLE `user` (
                        `id` bigint NOT NULL AUTO_INCREMENT COMMENT '用户ID',
                        `username` varchar(50) NOT NULL COMMENT '用户名',
                        `password` varchar(100) NOT NULL COMMENT '密码(加密存储)',
                        `phone` varchar(20) DEFAULT NULL COMMENT '手机号',
                        `email` varchar(100) DEFAULT NULL COMMENT '邮箱',
                        `status` tinyint NOT NULL DEFAULT '1' COMMENT '状态(0-禁用,1-正常)',
                        `create_time` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
                        `update_time` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
                        PRIMARY KEY (`id`),
                        UNIQUE KEY `uk_username` (`username`) COMMENT '用户名唯一'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户表';

-- 优惠券表
CREATE TABLE `coupon` (
                          `id` bigint NOT NULL AUTO_INCREMENT COMMENT '优惠券ID',
                          `name` varchar(100) NOT NULL COMMENT '优惠券名称',
                          `description` varchar(500) DEFAULT NULL COMMENT '优惠券描述',
                          `type` tinyint NOT NULL COMMENT '类型(1-常驻,2-秒抢)',
                          `face_value` decimal(10,2) NOT NULL COMMENT '面值(元)',
                          `min_spend` decimal(10,2) NOT NULL DEFAULT '0.00' COMMENT '最低消费(元)',
                          `valid_days` int NOT NULL DEFAULT 15 COMMENT '有效期(天)',
                          `total_stock` int NOT NULL COMMENT '总库存',
                          `remaining_stock` int NOT NULL COMMENT '剩余库存',
                          `status` tinyint NOT NULL DEFAULT '0' COMMENT '状态(0-未上架,1-已上架,2-已下架)',
                          `create_time` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
                          `update_time` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
                          PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='优惠券表';

-- 秒杀活动表
CREATE TABLE `seckill_activity` (
                                    `id` bigint NOT NULL AUTO_INCREMENT COMMENT '活动ID',
                                    `coupon_id` bigint NOT NULL COMMENT '优惠券ID',
                                    `start_time` datetime NOT NULL COMMENT '开始时间',
                                    `end_time` datetime NOT NULL COMMENT '结束时间',
                                    `status` tinyint NOT NULL DEFAULT '0' COMMENT '状态(0-未开始,1-进行中,2-已结束)',
                                    `create_time` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
                                    `update_time` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
                                    PRIMARY KEY (`id`),
                                    KEY `idx_coupon_id` (`coupon_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='秒杀活动表';

-- 订单表
CREATE TABLE `order` (
                         `id` varchar(36) NOT NULL COMMENT '订单ID(UUID)',
                         `user_id` bigint NOT NULL COMMENT '用户ID',
                         `coupon_id` bigint NOT NULL COMMENT '优惠券ID',
                         `status` tinyint NOT NULL COMMENT '状态(1-已创建,2-已使用,3-已过期,4-已取消)',
                         `get_time` datetime NOT NULL COMMENT '领取时间',
                         `expire_time` datetime NOT NULL COMMENT '过期时间',
                         `use_time` datetime DEFAULT NULL COMMENT '使用时间',
                         `cancel_time` datetime DEFAULT NULL COMMENT '取消时间',
                         `create_time` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
                         `update_time` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
                         PRIMARY KEY (`id`),
                         UNIQUE KEY `uk_user_coupon` (`user_id`,`coupon_id`,`status`) COMMENT '同一用户不能重复领取同一优惠券',
                         KEY `idx_user_id` (`user_id`),
                         KEY `idx_coupon_id` (`coupon_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='订单表';

-- 用户优惠券数量限制表(用于快速查询)
CREATE TABLE `user_coupon_count` (
                                     `user_id` bigint NOT NULL COMMENT '用户ID',
                                     `total_count` int NOT NULL DEFAULT '0' COMMENT '总优惠券数量',
                                     `seckill_count` int NOT NULL DEFAULT '0' COMMENT '秒杀优惠券数量',
                                     `update_time` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
                                     PRIMARY KEY (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户优惠券数量限制表';

-- 库存日志表(用于问题排查和对账)
CREATE TABLE `stock_log` (
                             `id` bigint NOT NULL AUTO_INCREMENT COMMENT '日志ID',
                             `coupon_id` bigint NOT NULL COMMENT '优惠券ID',
                             `quantity` int NOT NULL COMMENT '变动数量(正数为增加,负数为减少)',
                             `operate_type` tinyint NOT NULL COMMENT '操作类型(1-活动初始化,2-用户领取,3-用户取消,4-管理员调整)',
                             `operate_id` bigint DEFAULT NULL COMMENT '操作人ID',
                             `stock_after` int NOT NULL COMMENT '操作后库存',
                             `remark` varchar(500) DEFAULT NULL COMMENT '备注',
                             `create_time` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
                             PRIMARY KEY (`id`),
                             KEY `idx_coupon_id` (`coupon_id`),
                             KEY `idx_create_time` (`create_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='库存日志表';
-- 插入测试用户(密码为123456加密后的值)
INSERT INTO `user` (`username`, `password`, `phone`, `email`)
VALUES
    ('testuser1', '$2a$10$VQxG6L8jR6eXnQJQJQZQZ.8QZQZQZQZQZQZQZQZQZQZQZQZQZQ', '13800138001', 'test1@example.com'),
    ('testuser2', '$2a$10$VQxG6L8jR6eXnQJQJQZQZ.8QZQZQZQZQZQZQZQZQZQZQZQZQZQ', '13800138002', 'test2@example.com');

-- 插入测试优惠券
INSERT INTO `coupon` (`name`, `description`, `type`, `face_value`, `min_spend`, `valid_days`, `total_stock`, `remaining_stock`, `status`)
VALUES
    ('满100减20(常驻)', '购物满100元可减20元', 1, 20.00, 100.00, 15, 1000, 1000, 1),
    ('50元无门槛券(秒杀)', '无门槛使用50元优惠券', 2, 50.00, 0.00, 15, 100, 100, 1),
    ('满200减50(常驻)', '购物满200元可减50元', 1, 50.00, 200.00, 15, 500, 500, 1),
    ('10元无门槛券(秒杀)', '无门槛使用10元优惠券', 2, 10.00, 0.00, 15, 200, 200, 1);

-- 插入秒杀活动
INSERT INTO `seckill_activity` (`coupon_id`, `start_time`, `end_time`, `status`)
VALUES
    (2, '2024-08-01 10:00:00', '2024-08-01 12:00:00', 0),
    (4, '2024-08-02 15:00:00', '2024-08-02 16:00:00', 0);

-- 初始化用户优惠券数量
INSERT INTO `user_coupon_count` (`user_id`, `total_count`, `seckill_count`)
VALUES
    (1, 0, 0),
    (2, 0, 0);