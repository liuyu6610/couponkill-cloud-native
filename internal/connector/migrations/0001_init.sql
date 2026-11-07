-- cnx schema：电商平台 SKU 绑定与同品比价映射。迁移执行时 search_path 已设为 cnx。

-- 平台 SKU ↔ 本地券绑定，同时承载库存同步任务的进度（last_sync_*）
CREATE TABLE platform_sku_bindings (
    id               BIGINT      NOT NULL,                    -- snowflake，应用生成
    platform         TEXT        NOT NULL,                    -- MOCK / JD / TB / PDD
    external_sku_id  TEXT        NOT NULL,
    coupon_id        BIGINT      NOT NULL,
    sync_enabled     BOOLEAN     NOT NULL DEFAULT true,
    last_stock       BIGINT,
    last_sync_at     TIMESTAMPTZ,
    last_sync_status TEXT,                                    -- NEW / SKIP / SUCCESS / FAIL
    last_error       TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT pk_platform_sku_bindings PRIMARY KEY (id),
    CONSTRAINT ck_bindings_platform CHECK (platform ~ '^[A-Z][A-Z0-9_]*$'),
    CONSTRAINT ck_bindings_sku_not_blank CHECK (btrim(external_sku_id) <> ''),
    CONSTRAINT ck_bindings_last_stock CHECK (last_stock IS NULL OR last_stock >= 0),
    CONSTRAINT ck_bindings_sync_status CHECK (last_sync_status IS NULL OR last_sync_status IN ('NEW', 'SKIP', 'SUCCESS', 'FAIL')),
    CONSTRAINT uk_bindings_platform_sku UNIQUE (platform, external_sku_id),
    -- 一张券至多绑定一个平台 SKU；同时是 coupon_id 的查询索引
    CONSTRAINT uk_bindings_coupon UNIQUE (coupon_id)
);

-- 库存同步任务取“最久未同步优先”（从未同步的排最前）
CREATE INDEX idx_bindings_sync_rotation ON platform_sku_bindings (last_sync_at NULLS FIRST, id)
    WHERE sync_enabled;

-- 同品比价手工映射（多平台参考；不参与库存同步）
CREATE TABLE price_maps (
    id               BIGINT        NOT NULL,                  -- snowflake，应用生成
    coupon_id        BIGINT        NOT NULL,
    platform         TEXT          NOT NULL,
    external_sku_id  TEXT          NOT NULL,
    title            TEXT,
    manual_price     NUMERIC(12,2),
    currency         TEXT          NOT NULL DEFAULT 'CNY',
    enabled          BOOLEAN       NOT NULL DEFAULT true,
    last_sync_at     TIMESTAMPTZ,
    last_sync_status TEXT,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ   NOT NULL DEFAULT now(),
    CONSTRAINT pk_price_maps PRIMARY KEY (id),
    CONSTRAINT ck_price_maps_platform CHECK (platform ~ '^[A-Z][A-Z0-9_]*$'),
    CONSTRAINT ck_price_maps_sku_not_blank CHECK (btrim(external_sku_id) <> ''),
    CONSTRAINT ck_price_maps_price CHECK (manual_price IS NULL OR manual_price >= 0),
    CONSTRAINT ck_price_maps_currency CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT ck_price_maps_sync_status CHECK (last_sync_status IS NULL OR last_sync_status IN ('NEW', 'SKIP', 'SUCCESS', 'FAIL')),
    -- 每张券每个平台至多一条映射；(coupon_id, platform) 前缀也覆盖按券查询
    CONSTRAINT uk_price_maps_coupon_platform UNIQUE (coupon_id, platform)
);
