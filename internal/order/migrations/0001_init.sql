-- ord schema：订单（按 user_id 哈希分区）、预约帮抢、站内通知。迁移执行时 search_path 已设为 ord。
-- 订单按 user_id 分 16 个分区，替代 Java 的 ShardingSphere user_id%2 库 × user_id%16 表。
-- 注意：分区表上的唯一约束违反时，PostgreSQL 报告的约束名是“分区自己的索引名”（如 orders_p05_user_id_request_id_key），
-- 不是下面声明的父索引名；调用方应优先用 INSERT ... ON CONFLICT 处理预期内的冲突，
-- 而不是按约束名匹配（dbx.IsUniqueViolation(err, "") 即可判断是唯一冲突）。

CREATE TABLE orders (
    id          BIGINT        NOT NULL,                       -- snowflake，应用生成
    user_id     BIGINT        NOT NULL,
    coupon_id   BIGINT        NOT NULL,
    request_id  TEXT          NOT NULL,                       -- 秒杀请求号；非秒杀来源由应用生成
    -- 下单时拷贝的券快照：列表/统计不再跨服务查询
    coupon_name TEXT          NOT NULL,
    coupon_type SMALLINT      NOT NULL,                       -- 1 常驻 / 2 秒杀
    face_value  NUMERIC(12,2) NOT NULL DEFAULT 0,
    min_spend   NUMERIC(12,2) NOT NULL DEFAULT 0,
    status      SMALLINT      NOT NULL DEFAULT 1,             -- 1 已创建 / 2 已使用 / 3 已过期 / 4 已取消
    source      TEXT          NOT NULL DEFAULT 'normal',
    get_time    TIMESTAMPTZ   NOT NULL DEFAULT now(),
    expire_time TIMESTAMPTZ   NOT NULL,
    use_time    TIMESTAMPTZ,
    cancel_time TIMESTAMPTZ,
    created_at  TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ   NOT NULL DEFAULT now(),
    -- 分区表的主键必须包含分区键
    CONSTRAINT pk_orders PRIMARY KEY (user_id, id),
    CONSTRAINT ck_orders_coupon_type CHECK (coupon_type IN (1, 2)),
    CONSTRAINT ck_orders_status CHECK (status IN (1, 2, 3, 4)),
    CONSTRAINT ck_orders_source CHECK (source IN ('seckill', 'normal', 'reservation', 'admin')),
    CONSTRAINT ck_orders_face_value CHECK (face_value >= 0),
    CONSTRAINT ck_orders_min_spend CHECK (min_spend >= 0),
    CONSTRAINT ck_orders_expire_after_get CHECK (expire_time > get_time),
    CONSTRAINT ck_orders_cancelled_has_time CHECK (status <> 4 OR cancel_time IS NOT NULL),
    CONSTRAINT ck_orders_used_has_time CHECK (status <> 2 OR use_time IS NOT NULL),
    -- 同一请求号对同一用户至多一单：消费者重放时 INSERT ... ON CONFLICT DO NOTHING 靠它幂等
    CONSTRAINT uk_orders_user_request UNIQUE (user_id, request_id)
) PARTITION BY HASH (user_id);

DO $$
BEGIN
    FOR i IN 0..15 LOOP
        EXECUTE format(
            'CREATE TABLE %I PARTITION OF orders FOR VALUES WITH (MODULUS 16, REMAINDER %s)',
            'orders_p' || lpad(i::text, 2, '0'), i);
    END LOOP;
END $$;

-- 同一用户同一张券只允许一笔未取消的订单；取消(status=4)后可以再抢
CREATE UNIQUE INDEX uk_orders_user_coupon_active ON orders (user_id, coupon_id) WHERE status <> 4;
-- 我的订单列表（按时间倒序分页）
CREATE INDEX idx_orders_user_created ON orders (user_id, created_at DESC);
-- 按券统计 / 管理端查询
CREATE INDEX idx_orders_coupon ON orders (coupon_id);
-- 仅凭订单号的点查（管理端）；不带 user_id 时只能靠它避免全分区扫描
CREATE INDEX idx_orders_id ON orders (id);

-- 预约帮抢。状态机：PENDING → FIRING → QUEUED → SUCCESS / FAILED；PENDING → CANCELLED | EXPIRED
CREATE TABLE reservations (
    id          BIGINT      NOT NULL,                         -- snowflake，应用生成
    user_id     BIGINT      NOT NULL,
    coupon_id   BIGINT      NOT NULL,
    status      TEXT        NOT NULL DEFAULT 'PENDING',
    reserve_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    trigger_at  TIMESTAMPTZ,                                  -- 到点自动触发的时间（通常 = 券的开售时间）
    fired_at    TIMESTAMPTZ,
    request_id  TEXT,
    order_id    BIGINT,
    fail_code   INT,
    fail_reason TEXT,
    retry_count INT         NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT pk_reservations PRIMARY KEY (id),
    CONSTRAINT ck_reservations_status CHECK (status IN
        ('PENDING', 'FIRING', 'QUEUED', 'SUCCESS', 'FAILED', 'CANCELLED', 'EXPIRED')),
    CONSTRAINT ck_reservations_retry_count CHECK (retry_count >= 0),
    CONSTRAINT ck_reservations_pending_has_trigger CHECK (status <> 'PENDING' OR trigger_at IS NOT NULL)
);

-- 同一用户对同一张券同时只能有一个“进行中”的预约
CREATE UNIQUE INDEX uk_reservations_active_user_coupon ON reservations (user_id, coupon_id)
    WHERE status IN ('PENDING', 'FIRING', 'QUEUED');
-- 到点扫描（FOR UPDATE SKIP LOCKED）：只含待触发的行，不随历史数据增长
CREATE INDEX idx_reservations_pending_trigger ON reservations (status, trigger_at) WHERE status = 'PENDING';
-- 我的预约列表
CREATE INDEX idx_reservations_user_created ON reservations (user_id, created_at DESC);
-- 按秒杀请求号回查结果
CREATE INDEX idx_reservations_request ON reservations (request_id) WHERE request_id IS NOT NULL;
-- 排队中的结果回写扫描
CREATE INDEX idx_reservations_queued_updated ON reservations (updated_at)
    WHERE status = 'QUEUED' AND request_id IS NOT NULL;
-- 回收卡在 FIRING 的预约
CREATE INDEX idx_reservations_firing ON reservations (id) WHERE status = 'FIRING';
-- 券维度的外键列索引（按券查预约/统计）
CREATE INDEX idx_reservations_coupon ON reservations (coupon_id);

-- 站内通知（预约成功/失败/过期等）
CREATE TABLE notifications (
    id         BIGINT      NOT NULL,                          -- snowflake，应用生成
    user_id    BIGINT      NOT NULL,
    type       TEXT        NOT NULL,
    title      TEXT        NOT NULL,
    content    TEXT,
    ref_id     BIGINT,
    read_flag  BOOLEAN     NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT pk_notifications PRIMARY KEY (id)
);

CREATE INDEX idx_notifications_user_created ON notifications (user_id, created_at DESC);
-- 未读数 / 未读列表
CREATE INDEX idx_notifications_unread ON notifications (user_id) WHERE NOT read_flag;
