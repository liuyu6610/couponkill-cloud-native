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
            virtual_id              varchar(32) NOT NULL comment ''虚拟分片ID（格式：coupon_id_序号）'',
            name                    varchar(100) NOT NULL comment ''优惠券名称'',
            description             varchar(500) NULL comment ''描述'',
            type                    tinyint NOT NULL comment ''类型(1-常驻,2-秒抢)'',
            face_value              decimal(10,2) NOT NULL comment ''面值'',
            min_spend               decimal(10,2) DEFAULT 0.00 NOT NULL comment ''最低消费'',
            valid_days              int DEFAULT 15 NOT NULL comment ''有效期'',
            per_user_limit          int DEFAULT 1 NOT NULL comment ''每人限领'',
            total_stock             int NOT NULL comment ''总库存（虚拟分片）'',
            seckill_total_stock     int DEFAULT 0 NOT NULL comment ''秒杀库存（虚拟分片）'',
            remaining_stock         int DEFAULT 0 NOT NULL comment ''剩余库存（虚拟分片）'',
            seckill_remaining_stock int DEFAULT 0 NOT NULL comment ''秒杀剩余库存（虚拟分片）'',
            status                  tinyint DEFAULT 0 NOT NULL comment ''状态'',
            create_time             datetime DEFAULT CURRENT_TIMESTAMP NOT NULL,
            update_time             datetime DEFAULT CURRENT_TIMESTAMP NOT NULL ON UPDATE CURRENT_TIMESTAMP,
            version                 int DEFAULT 0 NOT NULL comment ''乐观锁'',
            PRIMARY KEY (id, virtual_id)
        ) comment ''优惠券表（含虚拟分片）'' row_format = DYNAMIC;');
            PREPARE stmt FROM @sql;
            EXECUTE stmt;
            DEALLOCATE PREPARE stmt;

            -- 创建优惠券表索引
            SET @sql = CONCAT('CREATE INDEX idx_coupon_virtual_id ON `', @coupon_table, '` (virtual_id);');
            PREPARE stmt FROM @sql;
            EXECUTE stmt;
            DEALLOCATE PREPARE stmt;

            SET @sql = CONCAT('CREATE INDEX idx_coupon_virtual_status ON `', @coupon_table, '` (virtual_id, status);');
            PREPARE stmt FROM @sql;
            EXECUTE stmt;
            DEALLOCATE PREPARE stmt;

            -- 创建库存日志分表
            SET @sql = CONCAT('
        CREATE TABLE IF NOT EXISTS `', @stock_log_table, '` (
            id           bigint AUTO_INCREMENT comment ''日志ID''
                primary key,
            coupon_id    bigint NOT NULL comment ''实际优惠券ID'',
            virtual_id   varchar(32) NOT NULL comment ''虚拟分片ID'',
            order_id     bigint NULL comment ''订单ID'',
            quantity     int NOT NULL comment ''变动数量'',
            operate_type tinyint NOT NULL comment ''操作类型'',
            stock_after  int NOT NULL comment ''操作后库存'',
            create_time  datetime DEFAULT CURRENT_TIMESTAMP NOT NULL,
            KEY idx_virtual_id (virtual_id)
        ) comment ''库存日志表（关联虚拟分片）'';');
            PREPARE stmt FROM @sql;
            EXECUTE stmt;
            DEALLOCATE PREPARE stmt;

            SET i = i + 1;
        END WHILE;

    -- 创建Seata undo_log表（每个库只需要创建一次）
    CREATE TABLE IF NOT EXISTS `undo_log` (
                                              branch_id     bigint       not null comment '分支事务ID'
                                                  primary key,
                                              xid           varchar(128) not null comment '全局事务ID',
                                              context       varchar(128) not null comment '上下文信息',
                                              rollback_info longblob     not null comment '回滚信息',
                                              log_status    int          not null comment '日志状态：0-正常，1-已删除',
                                              log_created   datetime     not null comment '创建时间',
                                              log_modified  datetime     not null comment '修改时间'
    ) comment 'AT模式undo日志表' row_format = DYNAMIC;

    CREATE INDEX  idx_xid on undo_log (xid);
END$$
DELIMITER ;

-- 执行存储过程（在每个coupon_db_0到coupon_db_3库中分别执行）
CALL CreateCouponTables();

-- 删除存储过程（可选）
DROP PROCEDURE IF EXISTS CreateCouponTables;
