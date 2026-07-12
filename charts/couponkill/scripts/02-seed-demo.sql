-- =============================================================================
-- CouponKill 演示种子数据（幂等，可重复执行）
-- 依赖：先执行 init-postgres.sql 建库建表
-- Docker Compose：挂载为 02-seed-demo.sql，在 init-postgres 之后自动跑
-- =============================================================================

-- ---------------------------------------------------------------------------
-- 演示账号：demo / demo1234
-- user.id=10000 → user_db_0（id % 2 = 0）
-- 密码为 BCrypt（Spring Security BCryptPasswordEncoder 可校验）
-- ---------------------------------------------------------------------------
\c user_db_0

INSERT INTO "user" (id, username, password, phone, email, status, create_time, update_time)
VALUES (
    10000,
    'demo',
    '$2a$10$7lN3AakPUWrcIjSRg74Fxej4IftgC3gGS.k22Ph8NKHoGzxfv0mcy',
    '13800000000',
    'demo@couponkill.local',
    1,
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
)
ON CONFLICT (username) DO NOTHING;

INSERT INTO user_coupon_count (user_id, total_count, seckill_count, normal_count, expired_count, update_time, version)
VALUES (10000, 0, 0, 0, 0, CURRENT_TIMESTAMP, 0)
ON CONFLICT (user_id) DO NOTHING;

-- 同步序列，避免后续自增撞 id
SELECT setval(pg_get_serial_sequence('"user"', 'id'), GREATEST((SELECT COALESCE(MAX(id), 1) FROM "user"), 10000));

-- ---------------------------------------------------------------------------
-- 演示优惠券（写入 coupon_db_0，shard 0..15）
-- 1001：秒杀券 type=2，总秒杀库存 1600（每分片 100）
-- 1002：常驻券 type=1，总库存 1600（每分片 100）
-- 分片规则：(shard_index/16)%2=0 → coupon_db_0；表 = coupon_{shard_index}
-- ---------------------------------------------------------------------------
\c coupon_db_0

DO $$
DECLARE i INT;
BEGIN
    FOR i IN 0..15 LOOP
        EXECUTE format($f$
            INSERT INTO coupon_%s (
                id, shard_index, name, description, type,
                face_value, min_spend, valid_days, per_user_limit,
                total_stock, seckill_total_stock, remaining_stock, seckill_remaining_stock,
                status, create_time, update_time, version
            ) VALUES (
                1001, %s, '演示秒杀券-满100减50', '本地演示用秒杀券，每用户限领1张', 2,
                50.00, 100.00, 7, 1,
                0, 100, 0, 100,
                1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 0
            )
            ON CONFLICT (id, shard_index) DO UPDATE SET
                name = EXCLUDED.name,
                description = EXCLUDED.description,
                seckill_total_stock = EXCLUDED.seckill_total_stock,
                seckill_remaining_stock = EXCLUDED.seckill_remaining_stock,
                status = 1,
                update_time = CURRENT_TIMESTAMP;
        $f$, i, i);

        EXECUTE format($f$
            INSERT INTO coupon_%s (
                id, shard_index, name, description, type,
                face_value, min_spend, valid_days, per_user_limit,
                total_stock, seckill_total_stock, remaining_stock, seckill_remaining_stock,
                status, create_time, update_time, version
            ) VALUES (
                1002, %s, '演示常驻券-满50减10', '本地演示用常驻券，每用户限领1张', 1,
                10.00, 50.00, 15, 1,
                100, 0, 100, 0,
                1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 0
            )
            ON CONFLICT (id, shard_index) DO UPDATE SET
                name = EXCLUDED.name,
                description = EXCLUDED.description,
                total_stock = EXCLUDED.total_stock,
                remaining_stock = EXCLUDED.remaining_stock,
                status = 1,
                update_time = CURRENT_TIMESTAMP;
        $f$, i, i);
    END LOOP;
END $$;
