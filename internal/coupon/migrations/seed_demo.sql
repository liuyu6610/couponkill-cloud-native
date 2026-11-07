-- 演示种子数据：仅当 CK_SEED_DEMO=true 时由 coupon 服务执行，生产禁用。不是迁移（文件名不符合 NNNN_name.sql）。
-- 幂等，可重复执行。使用 schema 全限定名，调用方无需设置 search_path。
--   1001：秒杀券（type=2）满 100 减 50，总库存 1600
--   1002：常驻券（type=1）满 50 减 10，总库存 1600
-- 库存拆成 8 个桶（与 CK_COUPON_STOCK_BUCKETS 默认值一致），每桶 200。
-- 服务启动后仍需把 ck:stock:1001 等 Redis 计数器预热（由 coupon 服务的预热逻辑负责）。

INSERT INTO cpn.coupons (id, name, description, type, face_value, min_spend, valid_days, per_user_limit, total_stock, status)
VALUES
    (1001, '演示秒杀券-满100减50', '本地演示用秒杀券，每用户限领1张', 2, 50.00, 100.00, 7, 1, 1600, 1),
    (1002, '演示常驻券-满50减10',  '本地演示用常驻券，每用户限领1张', 1, 10.00,  50.00, 15, 1, 1600, 1)
ON CONFLICT (id) DO NOTHING;

INSERT INTO cpn.coupon_stock (coupon_id, bucket, remaining)
SELECT c.id, b, 200
FROM (VALUES (1001::BIGINT), (1002::BIGINT)) AS c (id)
CROSS JOIN generate_series(0, 7) AS b
ON CONFLICT (coupon_id, bucket) DO NOTHING;
