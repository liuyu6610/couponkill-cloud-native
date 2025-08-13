create table coupon
(
    id                      bigint auto_increment comment '优惠券ID'
        primary key,
    name                    varchar(100)                             not null comment '优惠券名称',
    description             varchar(500)                             null comment '优惠券描述',
    type                    tinyint                                  not null comment '类型(1-常驻,2-秒抢)',
    face_value              decimal(10, 2)                           not null comment '面值(元)',
    min_spend               decimal(10, 2) default 0.00              not null comment '最低消费(元)',
    valid_days              int            default 15                not null comment '有效期(天)',
    per_user_limit          int            default 1                 not null comment '每人限领数量（0表示无限制）',
    total_stock             int                                      not null comment '总库存',
    seckill_total_stock     int            default 0                 not null comment '秒杀总库存（仅秒抢类型有效）',
    remaining_stock         int                                      not null comment '剩余库存',
    seckill_remaining_stock int            default 0                 not null comment '秒杀剩余库存（仅秒抢类型有效）',
    status                  tinyint        default 0                 not null comment '状态(0-未上架,1-已上架,2-已下架)',
    create_time             datetime       default CURRENT_TIMESTAMP not null comment '创建时间',
    update_time             datetime       default CURRENT_TIMESTAMP not null on update CURRENT_TIMESTAMP comment '更新时间'
)
    comment '优惠券表';

create table `order`
(
    id              bigint                             not null comment '订单ID（雪花算法生成，Java/Go双端区分段）'
        primary key,
    user_id         bigint                             not null comment '用户ID',
    coupon_id       bigint                             not null comment '优惠券ID',
    status          tinyint                            not null comment '状态(1-已创建,2-已使用,3-已过期,4-已取消)',
    get_time        datetime                           not null comment '领取时间',
    expire_time     datetime                           not null comment '过期时间',
    use_time        datetime                           null comment '使用时间',
    cancel_time     datetime                           null comment '取消时间',
    create_time     datetime default CURRENT_TIMESTAMP not null comment '创建时间',
    update_time     datetime default CURRENT_TIMESTAMP not null on update CURRENT_TIMESTAMP comment '更新时间',
    created_by_java tinyint  default 0                 not null comment '是否Java端创建(1-是,0-否)',
    created_by_go   tinyint  default 0                 not null comment '是否Go端创建(1-是,0-否)',
    request_id      varchar(64)                        not null comment '请求唯一标识(用于幂等性控制)',
    version         int      default 0                 not null comment '版本号（乐观锁，用于并发控制）',
    constraint uk_request_id
        unique (request_id) comment '通过请求ID保证幂等性',
    constraint uk_user_coupon
        unique (user_id, coupon_id, status) comment '同一用户不能重复领取同一优惠券',
    constraint uk_user_coupon_source
        unique (user_id, coupon_id, created_by_java, created_by_go) comment '同一用户对同一优惠券，两端均未领取时才能参与秒杀'
)
    comment '订单表';

create index idx_coupon_id
    on `order` (coupon_id);

create index idx_user_status
    on `order` (user_id, status)
    comment '按用户+状态查询订单';

create table orders
(
    id         bigint auto_increment
        primary key,
    coupon_id  varchar(255) null,
    created_at datetime(6)  null,
    request_id varchar(255) null,
    status     varchar(255) null,
    user_id    varchar(255) null,
    constraint UKtntc1qcum503inx2jy4nyalay
        unique (request_id)
);

create table seckill_activity
(
    id                      bigint auto_increment comment '活动ID'
        primary key,
    coupon_id               bigint                             not null comment '优惠券ID',
    start_time              datetime                           not null comment '开始时间',
    end_time                datetime                           not null comment '结束时间',
    activity_stock          int                                not null comment '活动总库存（与coupon表秒杀库存一致）',
    activity_per_user_limit int      default 1                 not null comment '活动每人限购数量（优先级高于coupon表）',
    status                  tinyint  default 0                 not null comment '状态(0-未开始,1-进行中,2-已结束)',
    create_time             datetime default CURRENT_TIMESTAMP not null comment '创建时间',
    update_time             datetime default CURRENT_TIMESTAMP not null on update CURRENT_TIMESTAMP comment '更新时间'
)
    comment '秒杀活动表';

create index idx_coupon_activity
    on seckill_activity (coupon_id, status)
    comment '按优惠券ID+状态查询活动';

create index idx_time_status
    on seckill_activity (start_time, end_time, status)
    comment '查询指定时间范围内的活动状态';

create table stock_log
(
    id           bigint auto_increment comment '日志ID'
        primary key,
    coupon_id    bigint                             not null comment '优惠券ID',
    order_id     bigint                             null comment '关联订单ID（用户领取/取消时填写）',
    activity_id  bigint                             null comment '关联秒杀活动ID（活动初始化时填写）',
    quantity     int                                not null comment '变动数量(正数为增加,负数为减少)',
    operate_type tinyint                            not null comment '操作类型(1-活动初始化,2-用户领取,3-用户取消,4-管理员调整)',
    operate_id   bigint                             null comment '操作人ID',
    stock_after  int                                not null comment '操作后库存',
    remark       varchar(500)                       null comment '备注',
    create_time  datetime default CURRENT_TIMESTAMP not null comment '创建时间'
)
    comment '库存日志表';

create index idx_coupon_id
    on stock_log (coupon_id);

create index idx_create_time
    on stock_log (create_time);

create index idx_related_id
    on stock_log (order_id, activity_id)
    comment '按订单/活动ID追踪库存变动';

create table user
(
    id          bigint auto_increment comment '用户ID'
        primary key,
    username    varchar(50)                        not null comment '用户名',
    password    varchar(100)                       not null comment '密码（加密存储，如bcrypt）',
    phone       varchar(20)                        null comment '手机号',
    email       varchar(100)                       null comment '邮箱',
    status      tinyint  default 1                 not null comment '状态(0-禁用,1-正常)',
    create_time datetime default CURRENT_TIMESTAMP not null comment '创建时间',
    update_time datetime default CURRENT_TIMESTAMP not null on update CURRENT_TIMESTAMP comment '更新时间',
    constraint uk_username
        unique (username) comment '用户名唯一'
)
    comment '用户表';

create index idx_email
    on user (email)
    comment '按邮箱查询用户';

create index idx_phone
    on user (phone)
    comment '按手机号查询用户';

create table user_coupon_count
(
    user_id       bigint                             not null comment '用户ID'
        primary key,
    total_count   int      default 0                 not null comment '总优惠券数量',
    seckill_count int      default 0                 not null comment '秒杀优惠券数量',
    normal_count  int                                null,
    expired_count int      default 0                 not null comment '已过期优惠券数量',
    update_time   datetime default CURRENT_TIMESTAMP not null on update CURRENT_TIMESTAMP comment '更新时间',
    version       int      default 0                 not null comment '版本号（乐观锁）'
)
    comment '用户优惠券数量限制表';

