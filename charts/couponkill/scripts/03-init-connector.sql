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
