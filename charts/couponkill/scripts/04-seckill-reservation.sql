-- =============================================================================
-- P0 预约帮抢：活动时间窗 + seckill_reservation
-- 幂等：可重复执行。绿场请先跑 init-postgres.sql，再跑本脚本。
-- =============================================================================

-- ---------------------------------------------------------------------------
-- 1. coupon 分片表增加 seckill_start_at / seckill_end_at
-- ---------------------------------------------------------------------------
\c coupon_db_0
DO $$
DECLARE i INT;
BEGIN
    FOR i IN 0..15 LOOP
        EXECUTE format(
            'ALTER TABLE coupon_%s ADD COLUMN IF NOT EXISTS seckill_start_at TIMESTAMPTZ',
            i
        );
        EXECUTE format(
            'ALTER TABLE coupon_%s ADD COLUMN IF NOT EXISTS seckill_end_at TIMESTAMPTZ',
            i
        );
    END LOOP;
END $$;

\c coupon_db_1
DO $$
DECLARE i INT;
BEGIN
    FOR i IN 0..15 LOOP
        EXECUTE format(
            'ALTER TABLE coupon_%s ADD COLUMN IF NOT EXISTS seckill_start_at TIMESTAMPTZ',
            i
        );
        EXECUTE format(
            'ALTER TABLE coupon_%s ADD COLUMN IF NOT EXISTS seckill_end_at TIMESTAMPTZ',
            i
        );
    END LOOP;
END $$;

-- ---------------------------------------------------------------------------
-- 2. 预约表落 order_db_0（ShardingSphere SINGLE → order-db-0）
--    状态机：PENDING → FIRING → QUEUED → SUCCESS / FAILED
--            PENDING → CANCELLED | EXPIRED
-- ---------------------------------------------------------------------------
\c order_db_0

CREATE TABLE IF NOT EXISTS seckill_reservation (
    id            BIGSERIAL PRIMARY KEY,
    user_id       BIGINT       NOT NULL,
    coupon_id     BIGINT       NOT NULL,
    status        VARCHAR(32)  NOT NULL DEFAULT 'PENDING',
    reserve_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    trigger_at    TIMESTAMPTZ,
    fired_at      TIMESTAMPTZ,
    request_id    VARCHAR(64),
    order_id      VARCHAR(64),
    fail_code     INT,
    fail_reason   VARCHAR(256),
    retry_count   INT          NOT NULL DEFAULT 0,
    version       INT          NOT NULL DEFAULT 0,
    create_time   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    update_time   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_reservation_active_user_coupon
    ON seckill_reservation (user_id, coupon_id)
    WHERE status IN ('PENDING', 'FIRING', 'QUEUED');

CREATE INDEX IF NOT EXISTS idx_reservation_pending_trigger
    ON seckill_reservation (status, trigger_at)
    WHERE status = 'PENDING';

CREATE INDEX IF NOT EXISTS idx_reservation_user
    ON seckill_reservation (user_id, create_time DESC);

CREATE INDEX IF NOT EXISTS idx_reservation_request
    ON seckill_reservation (request_id)
    WHERE request_id IS NOT NULL;
