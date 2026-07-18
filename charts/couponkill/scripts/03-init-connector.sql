-- =============================================================================
-- Connector 服务库表（电商 SKU ↔ 本地券绑定）
-- =============================================================================
SELECT 'CREATE DATABASE connector_db' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'connector_db')\gexec

\c connector_db

CREATE TABLE IF NOT EXISTS platform_sku_binding (
    id               BIGSERIAL PRIMARY KEY,
    platform         VARCHAR(32)  NOT NULL,
    external_sku_id  VARCHAR(128) NOT NULL,
    coupon_id        BIGINT       NOT NULL,
    sync_enabled     BOOLEAN      NOT NULL DEFAULT TRUE,
    last_stock       BIGINT,
    last_sync_at     TIMESTAMP,
    last_sync_status VARCHAR(32),
    last_error       TEXT,
    create_time      TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,
    update_time      TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_platform_sku UNIQUE (platform, external_sku_id),
    CONSTRAINT uk_binding_coupon UNIQUE (coupon_id)
);

CREATE INDEX IF NOT EXISTS idx_binding_coupon_id ON platform_sku_binding (coupon_id);

-- 同品比价手工映射（多平台参考；不参与库存同步）
CREATE TABLE IF NOT EXISTS coupon_price_map (
    id               BIGSERIAL PRIMARY KEY,
    coupon_id        BIGINT       NOT NULL,
    platform         VARCHAR(32)  NOT NULL,
    external_sku_id  VARCHAR(128) NOT NULL,
    title            VARCHAR(256),
    manual_price     NUMERIC(18, 2),
    currency         VARCHAR(8)   NOT NULL DEFAULT 'CNY',
    enabled          BOOLEAN      NOT NULL DEFAULT TRUE,
    create_time      TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,
    update_time      TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_price_map_coupon_platform UNIQUE (coupon_id, platform)
);

CREATE INDEX IF NOT EXISTS idx_price_map_coupon_id ON coupon_price_map (coupon_id);
