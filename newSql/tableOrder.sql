-- 创建订单分表的存储过程（完全修复版）
DELIMITER $$

-- 先删除已存在的存储过程（避免重复定义报错）
DROP PROCEDURE IF EXISTS create_order_tables$$

CREATE PROCEDURE create_order_tables()
BEGIN
    -- 声明循环变量
    DECLARE i INT DEFAULT 0;
    DECLARE table_name VARCHAR(50);

    -- 循环创建16个分表（0-15）
    WHILE i < 16 DO
        -- 拼接表名：order_0 到 order_15
        SET table_name = CONCAT('order_', i);

        -- 创建表（如果不存在）
        SET @sql = CONCAT(
            'CREATE TABLE IF NOT EXISTS `', table_name, '` (',
            'id BIGINT NOT NULL COMMENT ''订单ID（雪花算法）'' PRIMARY KEY,',
            'user_id BIGINT NOT NULL COMMENT ''用户ID'',',
            'coupon_id BIGINT NOT NULL COMMENT ''优惠券ID'',',
            'virtual_id VARCHAR(32) NOT NULL COMMENT ''优惠券虚拟ID'',',
            'status TINYINT NOT NULL COMMENT ''状态(1-已创建,2-已使用,3-已过期,4-已取消)'',',
            'get_time DATETIME NOT NULL COMMENT ''领取时间'',',
            'expire_time DATETIME NOT NULL COMMENT ''过期时间'',',
            'use_time DATETIME NULL COMMENT ''使用时间'',',
            'cancel_time DATETIME NULL COMMENT ''取消时间'',',
            'create_time DATETIME DEFAULT CURRENT_TIMESTAMP NOT NULL COMMENT ''创建时间'',',
            'update_time DATETIME DEFAULT CURRENT_TIMESTAMP NOT NULL ON UPDATE CURRENT_TIMESTAMP COMMENT ''更新时间'',',
            'created_by_java TINYINT DEFAULT 0 NOT NULL COMMENT ''是否Java端创建'',',
            'created_by_go TINYINT DEFAULT 0 NOT NULL COMMENT ''是否Go端创建'',',
            'request_id VARCHAR(64) NULL COMMENT ''请求唯一标识'',',
            'version INT DEFAULT 0 NOT NULL COMMENT ''乐观锁版本'',',
            'CONSTRAINT uk_user_coupon UNIQUE (user_id, coupon_id, status),',
            'CONSTRAINT uk_user_coupon_source UNIQUE (user_id, coupon_id, created_by_java, created_by_go)',
            ') COMMENT ''订单表（分表）'' ROW_FORMAT = DYNAMIC'
        );

        PREPARE stmt FROM @sql;
        EXECUTE stmt;
        DEALLOCATE PREPARE stmt;

        -- 创建索引 idx_user_createtime
        SET @sql = CONCAT(
            'CREATE INDEX idx_user_createtime ON `', table_name, '` (user_id, create_time)'
        );
        -- 使用错误处理避免索引已存在时报错
        BEGIN
            DECLARE CONTINUE HANDLER FOR 1061 BEGIN END; -- 忽略重复键错误
            PREPARE stmt FROM @sql;
            EXECUTE stmt;
            DEALLOCATE PREPARE stmt;
        END;

        -- 创建索引 idx_request_id
        SET @sql = CONCAT(
            'CREATE INDEX idx_request_id ON `', table_name, '` (request_id)'
        );
        BEGIN
            DECLARE CONTINUE HANDLER FOR 1061 BEGIN END; -- 忽略重复键错误
            PREPARE stmt FROM @sql;
            EXECUTE stmt;
            DEALLOCATE PREPARE stmt;
        END;

        -- 创建索引 idx_coupon_id
        SET @sql = CONCAT(
            'CREATE INDEX idx_coupon_id ON `', table_name, '` (coupon_id)'
        );
        BEGIN
            DECLARE CONTINUE HANDLER FOR 1061 BEGIN END; -- 忽略重复键错误
            PREPARE stmt FROM @sql;
            EXECUTE stmt;
            DEALLOCATE PREPARE stmt;
        END;

        -- 变量自增
        SET i = i + 1;
    END WHILE;

END$$

-- 恢复默认分隔符
DELIMITER ;

-- 执行存储过程（需在每个订单分库执行）
-- 使用前请先切换到目标数据库：USE order_db_0; （依次执行order_db_0到order_db_3）
-- CALL create_order_tables();

-- 执行完成后删除存储过程（可选）
-- DROP PROCEDURE IF EXISTS create_order_tables;
