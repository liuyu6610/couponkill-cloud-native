create table id_sequence
(
    id_type       varchar(50)   not null
        primary key,
    current_value bigint        not null,
    step          int default 1 not null
);

create table user
(
    id               bigint auto_increment comment '用户ID'
        primary key,
    username         varchar(50)                        not null comment '用户名',
    password         varchar(100)                       not null comment '密码（加密存储）',
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

