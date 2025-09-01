-- 为所有user表添加索引以提高查询性能
-- 适用于user_db_0和user_db_1数据库

USE user_db_0;

ALTER TABLE user 
ADD INDEX idx_status (status),
ADD INDEX idx_create_time (create_time);

ALTER TABLE user_coupon_count
ADD INDEX idx_update_time (update_time);

USE user_db_1;

ALTER TABLE user 
ADD INDEX idx_status (status),
ADD INDEX idx_create_time (create_time);

ALTER TABLE user_coupon_count
ADD INDEX idx_update_time (update_time);