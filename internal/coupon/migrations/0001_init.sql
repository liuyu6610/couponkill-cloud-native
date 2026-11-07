-- cpn schema：优惠券、分桶库存、库存流水。迁移执行时 search_path 已设为 cpn。

CREATE TABLE coupons (
    id               BIGINT        NOT NULL,                  -- snowflake，应用生成
    name             TEXT          NOT NULL,
    description      TEXT,
    type             SMALLINT      NOT NULL,                  -- 1 常驻 / 2 秒杀
    face_value       NUMERIC(12,2) NOT NULL DEFAULT 0,
    min_spend        NUMERIC(12,2) NOT NULL DEFAULT 0,
    valid_days       INT           NOT NULL DEFAULT 15,
    per_user_limit   INT           NOT NULL DEFAULT 1,
    total_stock      INT           NOT NULL DEFAULT 0,        -- 发行总量（秒杀券即秒杀总库存）；剩余量 = coupon_stock 各桶之和
    status           SMALLINT      NOT NULL DEFAULT 1,        -- 0 停用 / 1 启用
    seckill_start_at TIMESTAMPTZ,
    seckill_end_at   TIMESTAMPTZ,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ   NOT NULL DEFAULT now(),
    CONSTRAINT pk_coupons PRIMARY KEY (id),
    CONSTRAINT ck_coupons_name_len CHECK (char_length(name) BETWEEN 1 AND 100),
    CONSTRAINT ck_coupons_type CHECK (type IN (1, 2)),
    CONSTRAINT ck_coupons_face_value CHECK (face_value >= 0),
    CONSTRAINT ck_coupons_min_spend CHECK (min_spend >= 0),
    CONSTRAINT ck_coupons_valid_days CHECK (valid_days > 0),
    CONSTRAINT ck_coupons_per_user_limit CHECK (per_user_limit > 0),
    CONSTRAINT ck_coupons_total_stock CHECK (total_stock >= 0),
    CONSTRAINT ck_coupons_status CHECK (status IN (0, 1)),
    -- 活动时间窗要么都不设，要么成对设置且结束晚于开始
    CONSTRAINT ck_coupons_seckill_window CHECK (
        (seckill_start_at IS NULL AND seckill_end_at IS NULL)
        OR (seckill_start_at IS NOT NULL AND seckill_end_at IS NOT NULL AND seckill_end_at > seckill_start_at)
    )
);

-- C 端列表：只看启用中的券，按类型过滤
CREATE INDEX idx_coupons_status_type ON coupons (status, type);

-- 库存桶：把库存拆成 N 个桶分散热点行锁。扣减 = UPDATE ... SET remaining = remaining - 1 WHERE remaining > 0。
CREATE TABLE coupon_stock (
    coupon_id BIGINT   NOT NULL,
    bucket    SMALLINT NOT NULL,
    remaining INT      NOT NULL,
    CONSTRAINT pk_coupon_stock PRIMARY KEY (coupon_id, bucket),       -- 前缀即 coupon_id，外键列已被索引覆盖
    CONSTRAINT fk_coupon_stock_coupon FOREIGN KEY (coupon_id) REFERENCES coupons (id) ON DELETE CASCADE,
    CONSTRAINT ck_coupon_stock_bucket CHECK (bucket >= 0),
    CONSTRAINT ck_coupon_stock_remaining CHECK (remaining >= 0)
);

-- 库存流水：request_id 主键保证对同一请求的扣减/归还严格幂等。
-- 扣减/归还与桶更新必须在同一事务里完成。故意不设外键：券被删除后流水仍作审计保留。
CREATE TABLE stock_ledger (
    request_id TEXT        NOT NULL,
    coupon_id  BIGINT      NOT NULL,
    bucket     SMALLINT    NOT NULL,
    user_id    BIGINT      NOT NULL,
    state      TEXT        NOT NULL DEFAULT 'deducted',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT pk_stock_ledger PRIMARY KEY (request_id),
    CONSTRAINT ck_stock_ledger_state CHECK (state IN ('deducted', 'released')),
    CONSTRAINT ck_stock_ledger_bucket CHECK (bucket >= 0)
);

-- 按券统计/对账；按用户排查
CREATE INDEX idx_stock_ledger_coupon_state ON stock_ledger (coupon_id, state);
CREATE INDEX idx_stock_ledger_user ON stock_ledger (user_id);
