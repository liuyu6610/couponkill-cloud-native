-- usr schema：用户。迁移执行时 search_path 已设为 usr，这里不带 schema 前缀。
-- 全局唯一用户名靠单表 UNIQUE（不再分库）。

CREATE TABLE users (
    id             BIGINT      NOT NULL,                       -- snowflake，应用生成
    username       TEXT        NOT NULL,
    password_hash  TEXT        NOT NULL,                       -- bcrypt(cost 10)
    phone          TEXT,
    email          TEXT,
    role           TEXT        NOT NULL DEFAULT 'user',
    status         SMALLINT    NOT NULL DEFAULT 1,             -- 0 停用 / 1 正常
    last_active_at TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT pk_users PRIMARY KEY (id),
    CONSTRAINT uk_users_username UNIQUE (username),
    CONSTRAINT ck_users_username_len CHECK (char_length(username) BETWEEN 1 AND 50),
    CONSTRAINT ck_users_role CHECK (role IN ('user', 'admin')),
    CONSTRAINT ck_users_status CHECK (status IN (0, 1))
);

-- 失效用户扫描：按 COALESCE(last_active_at, created_at) 过滤
CREATE INDEX idx_users_inactive_at ON users ((COALESCE(last_active_at, created_at)));
