-- 插入测试用户数据
INSERT INTO user (username, password, phone, email, status, create_time, update_time) VALUES
                                                                                          ('admin', '$2a$10$wQ8vI4d4Q7S3k2h5Q7S3kujKzJ3Pv3z3H6j6P5j5H5n5H5n5H5n5.', '13800138000', 'admin@example.com', 1, NOW(), NOW()),
                                                                                          ('testuser1', '$2a$10$wQ8vI4d4Q7S3k2h5Q7S3kujKzJ3Pv3z3H6j6P5j5H5n5H5n5H5n5.', '13800138001', 'test1@example.com', 1, NOW(), NOW()),
                                                                                          ('testuser2', '$2a$10$wQ8vI4d4Q7S3k2h5Q7S3kujKzJ3Pv3z3H6j6P5j5H5n5H5n5H5n5.', '13800138002', 'test2@example.com', 1, NOW(), NOW()),
                                                                                          ('testuser3', '$2a$10$wQ8vI4d4Q7S3k2h5Q7S3kujKzJ3Pv3z3H6j6P5j5H5n5H5n5H5n5.', '13800138003', 'test3@example.com', 1, NOW(), NOW());

-- 初始化用户优惠券统计表
INSERT INTO user_coupon_count (user_id, total_count, seckill_count, normal_count, expired_count, update_time, version) VALUES
                                                                                                                           (1, 0, 0, 0, 0, NOW(), 0),
                                                                                                                           (2, 0, 0, 0, 0, NOW(), 0),
                                                                                                                           (3, 0, 0, 0, 0, NOW(), 0),
                                                                                                                           (4, 0, 0, 0, 0, NOW(), 0);

-- 插入测试优惠券数据
INSERT INTO coupon (name, description, type, face_value, min_spend, valid_days, per_user_limit, total_stock, seckill_total_stock, remaining_stock, seckill_remaining_stock, status, create_time, update_time) VALUES
                                                                                                                                                                                                                  ('新人专享券', '新用户注册专享优惠券', 1, 10.00, 50.00, 30, 1, 1000, 0, 1000, 0, 1, NOW(), NOW()),
                                                                                                                                                                                                                  ('满减优惠券', '满100减20优惠券', 1, 20.00, 100.00, 15, 2, 500, 0, 500, 0, 1, NOW(), NOW()),
                                                                                                                                                                                                                  ('秒杀优惠券', '限时秒杀优惠券', 2, 5.00, 30.00, 7, 1, 1000, 100, 1000, 100, 1, NOW(), NOW()),
                                                                                                                                                                                                                  ('节日特惠券', '节日 special offer', 1, 50.00, 200.00, 30, 1, 200, 0, 200, 0, 1, NOW(), NOW()),
                                                                                                                                                                                                                  ('通用优惠券', '全场通用优惠券', 1, 15.00, 60.00, 20, 3, 800, 0, 800, 0, 1, NOW(), NOW());

-- 插入测试订单数据（可选）
-- 注意：订单表中有一些约束，需要确保user_id和coupon_id存在
INSERT INTO `order` (id, user_id, coupon_id, status, get_time, expire_time, create_time, update_time, created_by_java, created_by_go, request_id, version) VALUES
                                                                                                                                                               (1000000000000000001, 2, 1, 1, NOW(), DATE_ADD(NOW(), INTERVAL 30 DAY), NOW(), NOW(), 1, 0, 'REQ001', 0),
                                                                                                                                                               (1000000000000000002, 3, 2, 1, NOW(), DATE_ADD(NOW(), INTERVAL 15 DAY), NOW(), NOW(), 1, 0, 'REQ002', 0);

-- 插入库存日志数据（可选）
INSERT INTO stock_log (coupon_id, order_id, quantity, operate_type, operate_id, stock_after, remark, create_time) VALUES
                                                                                                                      (1, 1000000000000000001, -1, 2, 2, 999, '用户领取优惠券', NOW()),
                                                                                                                      (2, 1000000000000000002, -1, 2, 3, 499, '用户领取优惠券', NOW());

-- 插入更多测试用户数据
INSERT INTO user (username, password, phone, email, status, create_time, update_time) VALUES
                                                                                          ('user001', '$2a$10$wQ8vI4d4Q7S3k2h5Q7S3kujKzJ3Pv3z3H6j6P5j5H5n5H5n5H5n5.', '13800138101', 'user001@example.com', 1, NOW(), NOW()),
                                                                                          ('user002', '$2a$10$wQ8vI4d4Q7S3k2h5Q7S3kujKzJ3Pv3z3H6j6P5j5H5n5H5n5H5n5.', '13800138102', 'user002@example.com', 1, NOW(), NOW()),
                                                                                          ('user003', '$2a$10$wQ8vI4d4Q7S3k2h5Q7S3kujKzJ3Pv3z3H6j6P5j5H5n5H5n5H5n5.', '13800138103', 'user003@example.com', 1, NOW(), NOW()),
                                                                                          ('user004', '$2a$10$wQ8vI4d4Q7S3k2h5Q7S3kujKzJ3Pv3z3H6j6P5j5H5n5H5n5H5n5.', '13800138104', 'user004@example.com', 1, NOW(), NOW()),
                                                                                          ('user005', '$2a$10$wQ8vI4d4Q7S3k2h5Q7S3kujKzJ3Pv3z3H6j6P5j5H5n5H5n5H5n5.', '13800138105', 'user005@example.com', 1, NOW(), NOW()),
                                                                                          ('user006', '$2a$10$wQ8vI4d4Q7S3k2h5Q7S3kujKzJ3Pv3z3H6j6P5j5H5n5H5n5H5n5.', '13800138106', 'user006@example.com', 1, NOW(), NOW()),
                                                                                          ('user007', '$2a$10$wQ8vI4d4Q7S3k2h5Q7S3kujKzJ3Pv3z3H6j6P5j5H5n5H5n5H5n5.', '13800138107', 'user007@example.com', 1, NOW(), NOW()),
                                                                                          ('user008', '$2a$10$wQ8vI4d4Q7S3k2h5Q7S3kujKzJ3Pv3z3H6j6P5j5H5n5H5n5H5n5.', '13800138108', 'user008@example.com', 1, NOW(), NOW()),
                                                                                          ('user009', '$2a$10$wQ8vI4d4Q7S3k2h5Q7S3kujKzJ3Pv3z3H6j6P5j5H5n5H5n5H5n5.', '13800138109', 'user009@example.com', 1, NOW(), NOW()),
                                                                                          ('user010', '$2a$10$wQ8vI4d4Q7S3k2h5Q7S3kujKzJ3Pv3z3H6j6P5j5H5n5H5n5H5n5.', '13800138110', 'user010@example.com', 1, NOW(), NOW());

-- 初始化更多用户优惠券统计表
INSERT INTO user_coupon_count (user_id, total_count, seckill_count, normal_count, expired_count, update_time, version) VALUES
                                                                                                                           (5, 0, 0, 0, 0, NOW(), 0),
                                                                                                                           (6, 0, 0, 0, 0, NOW(), 0),
                                                                                                                           (7, 0, 0, 0, 0, NOW(), 0),
                                                                                                                           (8, 0, 0, 0, 0, NOW(), 0),
                                                                                                                           (9, 0, 0, 0, 0, NOW(), 0),
                                                                                                                           (10, 0, 0, 0, 0, NOW(), 0),
                                                                                                                           (11, 0, 0, 0, 0, NOW(), 0),
                                                                                                                           (12, 0, 0, 0, 0, NOW(), 0),
                                                                                                                           (13, 0, 0, 0, 0, NOW(), 0),
                                                                                                                           (14, 0, 0, 0, 0, NOW(), 0);

-- 插入更多测试优惠券数据
INSERT INTO coupon (name, description, type, face_value, min_spend, valid_days, per_user_limit, total_stock, seckill_total_stock, remaining_stock, seckill_remaining_stock, status, create_time, update_time) VALUES
                                                                                                                                                                                                                  ('夏季清凉券', '夏季专属清凉优惠券', 1, 15.00, 50.00, 30, 1, 500, 0, 500, 0, 1, NOW(), NOW()),
                                                                                                                                                                                                                  ('开学季优惠券', '开学季学习用品满减券', 1, 25.00, 100.00, 15, 1, 300, 0, 300, 0, 1, NOW(), NOW()),
                                                                                                                                                                                                                  ('秒杀券A', '限时秒杀优惠券A', 2, 5.00, 30.00, 1, 1, 1000, 100, 1000, 100, 1, NOW(), NOW()),
                                                                                                                                                                                                                  ('秒杀券B', '限时秒杀优惠券B', 2, 10.00, 50.00, 1, 1, 500, 50, 500, 50, 1, NOW(), NOW()),
                                                                                                                                                                                                                  ('秒杀券C', '限时秒杀优惠券C', 2, 20.00, 100.00, 1, 1, 200, 20, 200, 20, 1, NOW(), NOW()),
                                                                                                                                                                                                                  ('会员专享券', 'VIP会员专享大额优惠券', 1, 50.00, 200.00, 7, 1, 100, 0, 100, 0, 1, NOW(), NOW()),
                                                                                                                                                                                                                  ('新人首单券', '新用户首单专享券', 1, 10.00, 30.00, 3, 1, 1000, 0, 1000, 0, 1, NOW(), NOW()),
                                                                                                                                                                                                                  ('周末狂欢券', '周末购物狂欢优惠券', 1, 30.00, 150.00, 2, 2, 200, 0, 200, 0, 1, NOW(), NOW()),
                                                                                                                                                                                                                  ('节日特惠券A', '节日 special offer A', 1, 20.00, 80.00, 5, 1, 400, 0, 400, 0, 1, NOW(), NOW()),
                                                                                                                                                                                                                  ('节日特惠券B', '节日 special offer B', 1, 40.00, 160.00, 5, 1, 200, 0, 200, 0, 1, NOW(), NOW());

-- 插入更多测试订单数据
-- 注意：订单表中有一些约束，需要确保user_id和coupon_id存在
INSERT INTO `order` (id, user_id, coupon_id, status, get_time, expire_time, create_time, update_time, created_by_java, created_by_go, request_id, version) VALUES
                                                                                                                                                               (1000000000000000003, 5, 3, 1, NOW(), DATE_ADD(NOW(), INTERVAL 30 DAY), NOW(), NOW(), 1, 0, 'REQ003', 0),
                                                                                                                                                               (1000000000000000004, 6, 4, 1, NOW(), DATE_ADD(NOW(), INTERVAL 15 DAY), NOW(), NOW(), 1, 0, 'REQ004', 0),
                                                                                                                                                               (1000000000000000005, 7, 5, 2, NOW(), DATE_ADD(NOW(), INTERVAL 1 DAY), DATE_ADD(NOW(), INTERVAL -2 DAY), DATE_ADD(NOW(), INTERVAL -1 DAY), 1, 0, 'REQ005', 0),
                                                                                                                                                               (1000000000000000006, 8, 6, 1, NOW(), DATE_ADD(NOW(), INTERVAL 7 DAY), NOW(), NOW(), 1, 0, 'REQ006', 0),
                                                                                                                                                               (1000000000000000007, 9, 7, 1, NOW(), DATE_ADD(NOW(), INTERVAL 3 DAY), NOW(), NOW(), 1, 0, 'REQ007', 0),
                                                                                                                                                               (1000000000000000008, 10, 8, 4, NOW(), DATE_ADD(NOW(), INTERVAL 2 DAY), DATE_ADD(NOW(), INTERVAL -3 DAY), DATE_ADD(NOW(), INTERVAL -2 DAY), 1, 0, 'REQ008', 0),
                                                                                                                                                               (1000000000000000009, 11, 9, 1, NOW(), DATE_ADD(NOW(), INTERVAL 5 DAY), NOW(), NOW(), 1, 0, 'REQ009', 0),
                                                                                                                                                               (1000000000000000010, 12, 10, 1, NOW(), DATE_ADD(NOW(), INTERVAL 5 DAY), NOW(), NOW(), 1, 0, 'REQ010', 0),
                                                                                                                                                               (1000000000000000011, 13, 11, 1, NOW(), DATE_ADD(NOW(), INTERVAL 30 DAY), NOW(), NOW(), 1, 0, 'REQ011', 0),
                                                                                                                                                               (1000000000000000012, 14, 12, 1, NOW(), DATE_ADD(NOW(), INTERVAL 30 DAY), NOW(), NOW(), 1, 0, 'REQ012', 0);

-- 插入更多库存日志数据
INSERT INTO stock_log (coupon_id, order_id, quantity, operate_type, operate_id, stock_after, remark, create_time) VALUES
                                                                                                                      (3, 1000000000000000003, -1, 2, 5, 999, '用户领取秒杀券A', NOW()),
                                                                                                                      (4, 1000000000000000004, -1, 2, 6, 499, '用户领取秒杀券B', NOW()),
                                                                                                                      (5, 1000000000000000005, -1, 2, 7, 199, '用户领取秒杀券C', NOW()),
                                                                                                                      (6, 1000000000000000006, -1, 2, 8, 99, '用户领取会员专享券', NOW()),
                                                                                                                      (7, 1000000000000000007, -1, 2, 9, 999, '用户领取新人首单券', NOW()),
                                                                                                                      (8, 1000000000000000008, -1, 2, 10, 199, '用户领取周末狂欢券', NOW()),
                                                                                                                      (9, 1000000000000000009, -1, 2, 11, 399, '用户领取节日特惠券A', NOW()),
                                                                                                                      (10, 1000000000000000010, -1, 2, 12, 199, '用户领取节日特惠券B', NOW()),
                                                                                                                      (11, 1000000000000000011, -1, 2, 13, 499, '用户领取开学季优惠券', NOW()),
                                                                                                                      (12, 1000000000000000012, -1, 2, 14, 999, '用户领取夏季清凉券', NOW());

-- 更新部分用户的优惠券统计信息
UPDATE user_coupon_count SET total_count = 1, normal_count = 1, update_time = NOW() WHERE user_id = 5;
UPDATE user_coupon_count SET total_count = 1, seckill_count = 1, update_time = NOW() WHERE user_id = 6;
UPDATE user_coupon_count SET total_count = 2, seckill_count = 1, normal_count = 1, update_time = NOW() WHERE user_id = 7;
UPDATE user_coupon_count SET total_count = 1, normal_count = 1, update_time = NOW() WHERE user_id = 8;
UPDATE user_coupon_count SET total_count = 1, normal_count = 1, update_time = NOW() WHERE user_id = 9;
UPDATE user_coupon_count SET total_count = 1, normal_count = 1, update_time = NOW() WHERE user_id = 10;
UPDATE user_coupon_count SET total_count = 1, normal_count = 1, update_time = NOW() WHERE user_id = 11;
UPDATE user_coupon_count SET total_count = 1, normal_count = 1, update_time = NOW() WHERE user_id = 12;
UPDATE user_coupon_count SET total_count = 1, normal_count = 1, update_time = NOW() WHERE user_id = 13;
UPDATE user_coupon_count SET total_count = 1, normal_count = 1, update_time = NOW() WHERE user_id = 14;
