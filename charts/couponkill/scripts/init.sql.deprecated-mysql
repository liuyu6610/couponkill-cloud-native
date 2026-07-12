-- 创建数据库表结构
CREATE TABLE IF NOT EXISTS `user` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `username` VARCHAR(50) NOT NULL UNIQUE,
  `password` VARCHAR(100) NOT NULL,
  `email` VARCHAR(100),
  `phone` VARCHAR(20),
  `create_time` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  `update_time` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `coupon` (
  `id` BIGINT NOT NULL,
  `shard_index` INT NOT NULL,
  `name` VARCHAR(100) NOT NULL,
  `description` TEXT,
  `discount_type` INT NOT NULL COMMENT '1-满减,2-折扣',
  `discount_value` DECIMAL(10,2) NOT NULL,
  `min_amount` DECIMAL(10,2) DEFAULT 0.00,
  `total_stock` INT NOT NULL DEFAULT 0,
  `available_stock` INT NOT NULL DEFAULT 0,
  `frozen_stock` INT NOT NULL DEFAULT 0,
  `start_time` DATETIME NOT NULL,
  `end_time` DATETIME NOT NULL,
  `status` INT NOT NULL DEFAULT 1 COMMENT '1-正常,2-已结束,3-已删除',
  `create_time` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  `update_time` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`, `shard_index`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `order` (
  `id` VARCHAR(64) NOT NULL,
  `user_id` BIGINT NOT NULL,
  `coupon_id` BIGINT NOT NULL,
  `virtual_id` VARCHAR(64) NOT NULL,
  `status` INT NOT NULL DEFAULT 1 COMMENT '1-已创建,2-已使用,3-已过期,4-已取消',
  `get_time` DATETIME NOT NULL,
  `expire_time` DATETIME NOT NULL,
  `use_time` DATETIME DEFAULT NULL,
  `cancel_time` DATETIME DEFAULT NULL,
  `create_time` DATETIME NOT NULL,
  `update_time` DATETIME NOT NULL,
  `created_by_java` INT NOT NULL DEFAULT 0 COMMENT '0-否,1-是',
  `created_by_go` INT NOT NULL DEFAULT 0 COMMENT '0-否,1-是',
  `request_id` VARCHAR(64) NOT NULL,
  `version` INT NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_user_id` (`user_id`),
  KEY `idx_coupon_id` (`coupon_id`),
  KEY `idx_virtual_id` (`virtual_id`),
  KEY `idx_create_time` (`create_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `user_coupon` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `user_id` BIGINT NOT NULL,
  `coupon_id` BIGINT NOT NULL,
  `order_id` VARCHAR(64) DEFAULT NULL,
  `status` INT NOT NULL DEFAULT 1 COMMENT '1-未使用,2-已使用,3-已过期',
  `get_time` DATETIME NOT NULL,
  `use_time` DATETIME DEFAULT NULL,
  `expire_time` DATETIME NOT NULL,
  `create_time` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  `update_time` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_coupon` (`user_id`, `coupon_id`),
  KEY `idx_user_id` (`user_id`),
  KEY `idx_coupon_id` (`coupon_id`),
  KEY `idx_expire_time` (`expire_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;