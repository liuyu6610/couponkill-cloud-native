DELIMITER $$
CREATE PROCEDURE CreateCouponTables()
BEGIN
    DECLARE i INT DEFAULT 0;
    -- 循环创建16个分表（0-15）
    WHILE i < 16 DO
            -- 拼接表名
            SET @coupon_table = CONCAT('coupon_', i);
            SET @stock_log_table = CONCAT('stock_log_', i);

            -- 创建优惠券分表
            SET @sql = CONCAT('
        CREATE TABLE IF NOT EXISTS `', @coupon_table, '` (
            id                      bigint NOT NULL comment ''优惠券ID'',
            name                    varchar(100) NOT NULL comment ''优惠券名称'',
            description             varchar(500) NULL comment ''描述'',
            type                    tinyint NOT NULL comment ''类型(1-常驻,2-秒抢)'',
            face_value              decimal(10,2) NOT NULL comment ''面值'',
            min_spend               decimal(10,2) DEFAULT 0.00 NOT NULL comment ''最低消费'',
            valid_days              int DEFAULT 15 NOT NULL comment ''有效期'',
            per_user_limit          int DEFAULT 1 NOT NULL comment ''每人限领'',
            total_stock             int NOT NULL comment ''总库存'',
            seckill_total_stock     int DEFAULT 0 NOT NULL comment ''秒杀库存'',
            remaining_stock         int DEFAULT 0 NOT NULL comment ''剩余库存'',
            seckill_remaining_stock int DEFAULT 0 NOT NULL comment ''秒杀剩余库存'',
            status                  tinyint DEFAULT 0 NOT NULL comment ''状态'',
            create_time             datetime DEFAULT CURRENT_TIMESTAMP NOT NULL,
            update_time             datetime DEFAULT CURRENT_TIMESTAMP NOT NULL ON UPDATE CURRENT_TIMESTAMP,
            version                 int DEFAULT 0 NOT NULL comment ''乐观锁'',
            shard_index             int NOT NULL comment ''分片索引'',
            PRIMARY KEY (id, shard_index)
        ) comment ''优惠券表'' row_format = DYNAMIC;');
            PREPARE stmt FROM @sql;
            EXECUTE stmt;
            DEALLOCATE PREPARE stmt;

            -- 创建优惠券表索引
            SET @sql = CONCAT('CREATE INDEX idx_coupon_status ON `', @coupon_table, '` (status);');
            PREPARE stmt FROM @sql;
            EXECUTE stmt;
            DEALLOCATE PREPARE stmt;

            -- 创建库存日志分表
            SET @sql = CONCAT('
        CREATE TABLE IF NOT EXISTS `', @stock_log_table, '` (
            id           bigint AUTO_INCREMENT comment ''日志ID''
                primary key,
            coupon_id    bigint NOT NULL comment ''优惠券ID'',
            shard_index  int NOT NULL comment ''分片索引'',
            order_id     bigint NULL comment ''订单ID'',
            quantity     int NOT NULL comment ''变动数量'',
            operate_type tinyint NOT NULL comment ''操作类型'',
            stock_after  int NOT NULL comment ''操作后库存'',
            create_time  datetime DEFAULT CURRENT_TIMESTAMP NOT NULL,
            KEY idx_coupon_id (coupon_id)
        ) comment ''库存日志表'';');
            PREPARE stmt FROM @sql;
            EXECUTE stmt;
            DEALLOCATE PREPARE stmt;

            SET i = i + 1;
        END WHILE;
END$$
DELIMITER ;

-- 执行存储过程（在每个coupon_db_0到coupon_db_3库中分别执行）
CALL CreateCouponTables();

-- 删除存储过程（可选）
DROP PROCEDURE IF EXISTS CreateCouponTables;
