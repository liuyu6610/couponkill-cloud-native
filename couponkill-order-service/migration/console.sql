CREATE TABLE coupon
(
    id                      BIGINT AUTO_INCREMENT        NOT NULL COMMENT '优惠券ID',
    name                    VARCHAR(100)                 NOT NULL COMMENT '优惠券名称',
    `description`           VARCHAR(500)                 NULL COMMENT '优惠券描述',
    type                    TINYINT                      NOT NULL COMMENT '类型(1-常驻,2-秒抢)',
    face_value              DECIMAL(10, 2)               NOT NULL COMMENT '面值(元)',
    min_spend               DECIMAL(10, 2) DEFAULT 0     NOT NULL COMMENT '最低消费(元)',
    valid_days              INT            DEFAULT 15    NOT NULL COMMENT '有效期(天)',
    per_user_limit          INT            DEFAULT 1     NOT NULL COMMENT '每人限领数量（0表示无限制）',
    total_stock             INT                          NOT NULL COMMENT '总库存',
    seckill_total_stock     INT            DEFAULT 0     NOT NULL COMMENT '秒杀总库存（仅秒抢类型有效）',
    remaining_stock         INT                          NOT NULL COMMENT '剩余库存',
    seckill_remaining_stock INT            DEFAULT 0     NOT NULL COMMENT '秒杀剩余库存（仅秒抢类型有效）',
    status                  TINYINT        DEFAULT 0     NOT NULL COMMENT '状态(0-未上架,1-已上架,2-已下架)',
    create_time             datetime       DEFAULT NOW() NOT NULL COMMENT '创建时间',
    update_time             datetime       DEFAULT NOW() NOT NULL COMMENT '更新时间',
    CONSTRAINT `PRIMARY` PRIMARY KEY (id)
) COMMENT ='优惠券表';

CREATE TABLE `order`
(
    id              BIGINT                 NOT NULL COMMENT '订单ID（雪花算法生成，Java/Go双端区分段）',
    user_id         BIGINT                 NOT NULL COMMENT '用户ID',
    coupon_id       BIGINT                 NOT NULL COMMENT '优惠券ID',
    status          TINYINT                NOT NULL COMMENT '状态(1-已创建,2-已使用,3-已过期,4-已取消)',
    get_time        datetime               NOT NULL COMMENT '领取时间',
    expire_time     datetime               NOT NULL COMMENT '过期时间',
    use_time        datetime               NULL COMMENT '使用时间',
    cancel_time     datetime               NULL COMMENT '取消时间',
    create_time     datetime DEFAULT NOW() NOT NULL COMMENT '创建时间',
    update_time     datetime DEFAULT NOW() NOT NULL COMMENT '更新时间',
    created_by_java TINYINT  DEFAULT 0     NOT NULL COMMENT '是否Java端创建(1-是,0-否)',
    created_by_go   TINYINT  DEFAULT 0     NOT NULL COMMENT '是否Go端创建(1-是,0-否)',
    request_id      VARCHAR(64)            NULL COMMENT '请求唯一标识(用于幂等性控制)',
    version         INT      DEFAULT 0     NOT NULL COMMENT '版本号（乐观锁，用于并发控制）',
    CONSTRAINT `PRIMARY` PRIMARY KEY (id)
) COMMENT ='订单表';

CREATE TABLE orders
(
    id         BIGINT AUTO_INCREMENT NOT NULL,
    coupon_id  VARCHAR(255)          NULL,
    created_at datetime              NULL,
    request_id VARCHAR(255)          NULL,
    status     VARCHAR(255)          NULL,
    user_id    VARCHAR(255)          NULL,
    CONSTRAINT `PRIMARY` PRIMARY KEY (id)
);

CREATE TABLE stock_log
(
    id           BIGINT AUTO_INCREMENT  NOT NULL COMMENT '日志ID',
    coupon_id    BIGINT                 NOT NULL COMMENT '优惠券ID',
    order_id     BIGINT                 NULL COMMENT '关联订单ID（用户领取/取消时填写）',
    activity_id  BIGINT                 NULL COMMENT '关联秒杀活动ID（活动初始化时填写）',
    quantity     INT                    NOT NULL COMMENT '变动数量(正数为增加,负数为减少)',
    operate_type TINYINT                NOT NULL COMMENT '操作类型(1-活动初始化,2-用户领取,3-用户取消,4-管理员调整)',
    operate_id   BIGINT                 NULL COMMENT '操作人ID',
    stock_after  INT                    NOT NULL COMMENT '操作后库存',
    remark       VARCHAR(500)           NULL COMMENT '备注',
    create_time  datetime DEFAULT NOW() NOT NULL COMMENT '创建时间',
    CONSTRAINT `PRIMARY` PRIMARY KEY (id)
) COMMENT ='库存日志表';

CREATE TABLE undo_log
(
    branch_id     BIGINT       NOT NULL COMMENT '分支事务ID',
    xid           VARCHAR(128) NOT NULL COMMENT '全局事务ID',
    context       VARCHAR(128) NOT NULL COMMENT '上下文信息',
    rollback_info BLOB         NOT NULL COMMENT '回滚信息',
    log_status    INT          NOT NULL COMMENT '日志状态：0-正常，1-已删除',
    log_created   datetime     NOT NULL COMMENT '创建时间',
    log_modified  datetime     NOT NULL COMMENT '修改时间',
    CONSTRAINT `PRIMARY` PRIMARY KEY (branch_id)
) COMMENT ='AT模式undo日志表';

CREATE TABLE user
(
    id               BIGINT AUTO_INCREMENT  NOT NULL COMMENT '用户ID',
    username         VARCHAR(50)            NOT NULL COMMENT '用户名',
    password         VARCHAR(100)           NOT NULL COMMENT '密码（加密存储，如bcrypt）',
    phone            VARCHAR(20)            NULL COMMENT '手机号',
    email            VARCHAR(100)           NULL COMMENT '邮箱',
    status           TINYINT  DEFAULT 1     NOT NULL COMMENT '状态(0-禁用,1-正常)',
    create_time      datetime DEFAULT NOW() NOT NULL COMMENT '创建时间',
    update_time      datetime DEFAULT NOW() NOT NULL COMMENT '更新时间',
    last_active_time datetime               NULL COMMENT '最后活跃时间',
    CONSTRAINT `PRIMARY` PRIMARY KEY (id)
) COMMENT ='用户表';

CREATE TABLE user_coupon_count
(
    user_id       BIGINT                 NOT NULL COMMENT '用户ID',
    total_count   INT      DEFAULT 0     NOT NULL COMMENT '总优惠券数量',
    seckill_count INT      DEFAULT 0     NOT NULL COMMENT '秒杀优惠券数量',
    normal_count  INT      DEFAULT 0     NULL COMMENT '普通优惠券数量',
    expired_count INT      DEFAULT 0     NOT NULL COMMENT '已过期优惠券数量',
    update_time   datetime DEFAULT NOW() NOT NULL COMMENT '更新时间',
    version       INT      DEFAULT 0     NOT NULL COMMENT '版本号（乐观锁）',
    CONSTRAINT `PRIMARY` PRIMARY KEY (user_id)
) COMMENT ='用户优惠券数量限制表';

ALTER TABLE orders
    ADD CONSTRAINT UKtntc1qcum503inx2jy4nyalay UNIQUE (request_id);

ALTER TABLE `order`
    ADD CONSTRAINT uk_request_id UNIQUE (request_id);

ALTER TABLE `order`
    ADD CONSTRAINT uk_user_coupon UNIQUE (user_id, coupon_id, status);

ALTER TABLE `order`
    ADD CONSTRAINT uk_user_coupon_source UNIQUE (user_id, coupon_id, created_by_java, created_by_go);

ALTER TABLE user
    ADD CONSTRAINT uk_username UNIQUE (username);

CREATE INDEX idx_coupon_id ON stock_log (coupon_id);

CREATE INDEX idx_coupon_id ON stock_log (coupon_id);

CREATE INDEX idx_create_time ON stock_log (create_time);

CREATE INDEX idx_related_id ON stock_log (order_id, activity_id);

CREATE INDEX idx_user_status ON `order` (user_id, status);

CREATE INDEX idx_xid ON undo_log (xid);