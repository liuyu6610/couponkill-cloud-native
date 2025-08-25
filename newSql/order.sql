create table order_0
(
    id              bigint                             not null comment '订单ID（雪花算法）'
        primary key,
    user_id         bigint                             not null comment '用户ID',
    coupon_id       bigint                             not null comment '优惠券ID',
    virtual_id      varchar(32)                        not null comment '优惠券虚拟ID',
    status          tinyint                            not null comment '状态(1-已创建,2-已使用,3-已过期,4-已取消)',
    get_time        datetime                           not null comment '领取时间',
    expire_time     datetime                           not null comment '过期时间',
    use_time        datetime                           null comment '使用时间',
    cancel_time     datetime                           null comment '取消时间',
    create_time     datetime default CURRENT_TIMESTAMP not null comment '创建时间',
    update_time     datetime default CURRENT_TIMESTAMP not null on update CURRENT_TIMESTAMP comment '更新时间',
    created_by_java tinyint  default 0                 not null comment '是否Java端创建',
    created_by_go   tinyint  default 0                 not null comment '是否Go端创建',
    request_id      varchar(64)                        null comment '请求唯一标识',
    version         int      default 0                 not null comment '乐观锁版本',
    constraint uk_user_coupon
        unique (user_id, coupon_id, status),
    constraint uk_user_coupon_source
        unique (user_id, coupon_id, created_by_java, created_by_go)
)
    comment '订单表（分表）' row_format = DYNAMIC;

create index idx_coupon_id
    on order_0 (coupon_id);

create index idx_request_id
    on order_0 (request_id);

create index idx_user_createtime
    on order_0 (user_id, create_time);

