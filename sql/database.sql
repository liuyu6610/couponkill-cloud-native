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
    remaining_stock         int            default 0                 not null comment '剩余库存',
    seckill_remaining_stock int            default 0                 not null comment '秒杀剩余库存（仅秒抢类型有效）',
    status                  tinyint        default 0                 not null comment '状态(0-未上架,1-已上架,2-已下架)',
    create_time             datetime       default CURRENT_TIMESTAMP not null comment '创建时间',
    update_time             datetime       default CURRENT_TIMESTAMP not null on update CURRENT_TIMESTAMP comment '更新时间',
    version                 int            default 0                 not null
)
    comment '优惠券表' row_format = DYNAMIC;

create index idx_coupon_create_time
    on coupon (create_time);

create index idx_coupon_seckill_remaining_stock
    on coupon (seckill_remaining_stock);

create index idx_coupon_status_stock
    on coupon (status, remaining_stock);

create index idx_coupon_type
    on coupon (type);

create index idx_coupon_type_status
    on coupon (type, status);

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
    request_id      varchar(64)                        null comment '请求唯一标识(用于幂等性控制)',
    version         int      default 0                 not null comment '版本号（乐观锁，用于并发控制）',
    constraint uk_user_coupon
        unique (user_id, coupon_id, status),
    constraint uk_user_coupon_source
        unique (user_id, coupon_id, created_by_java, created_by_go)
)
    comment '订单表' row_format = DYNAMIC;

create index idx_create_time
    on `order` (create_time);

create index idx_create_time_status
    on `order` (create_time, status);

create index idx_createtime_status
    on `order` (create_time, status);

create index idx_order_request_id
    on `order` (request_id);

create index idx_user_coupon_status
    on `order` (user_id, coupon_id, status);

create index idx_user_createtime
    on `order` (user_id, create_time);

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
    comment '库存日志表' row_format = DYNAMIC;

create index idx_related_id
    on stock_log (order_id, activity_id);

create index idx_stocklog_coupon_createtime
    on stock_log (coupon_id, create_time);

create table undo_log
(
    branch_id     bigint       not null comment '分支事务ID'
        primary key,
    xid           varchar(128) not null comment '全局事务ID',
    context       varchar(128) not null comment '上下文信息',
    rollback_info longblob     not null comment '回滚信息',
    log_status    int          not null comment '日志状态：0-正常，1-已删除',
    log_created   datetime     not null comment '创建时间',
    log_modified  datetime     not null comment '修改时间'
)
    comment 'AT模式undo日志表' row_format = DYNAMIC;

create index idx_xid
    on undo_log (xid);

create table user
(
    id               bigint auto_increment comment '用户ID'
        primary key,
    username         varchar(50)                        not null comment '用户名',
    password         varchar(100)                       not null comment '密码（加密存储，如bcrypt）',
    phone            varchar(20)                        null comment '手机号',
    email            varchar(100)                       null comment '邮箱',
    status           tinyint  default 1                 not null comment '状态(0-禁用,1-正常)',
    create_time      datetime default CURRENT_TIMESTAMP not null comment '创建时间',
    update_time      datetime default CURRENT_TIMESTAMP not null on update CURRENT_TIMESTAMP comment '更新时间',
    last_active_time datetime                           null comment '最后活跃时间',
    constraint uk_username
        unique (username)
)
    comment '用户表' row_format = DYNAMIC;

create table user_coupon_count
(
    user_id       bigint                             not null comment '用户ID'
        primary key,
    total_count   int      default 0                 not null comment '总优惠券数量',
    seckill_count int      default 0                 not null comment '秒杀优惠券数量',
    normal_count  int      default 0                 null comment '普通优惠券数量',
    expired_count int      default 0                 not null comment '已过期优惠券数量',
    update_time   datetime default CURRENT_TIMESTAMP not null on update CURRENT_TIMESTAMP comment '更新时间',
    version       int      default 0                 not null comment '版本号（乐观锁）'
)
    comment '用户优惠券数量限制表' row_format = DYNAMIC;

create index idx_user_coupon_count_update_time
    on user_coupon_count (update_time);

create index idx_user_coupon_count_user_id
    on user_coupon_count (user_id);

