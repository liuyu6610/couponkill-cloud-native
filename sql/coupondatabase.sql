create table coupon_0
(
    id                      bigint                                   not null comment '优惠券ID',
    name                    varchar(100)                             not null comment '优惠券名称',
    description             varchar(500)                             null comment '描述',
    type                    tinyint                                  not null comment '类型(1-常驻,2-秒抢)',
    face_value              decimal(10, 2)                           not null comment '面值',
    min_spend               decimal(10, 2) default 0.00              not null comment '最低消费',
    valid_days              int            default 15                not null comment '有效期',
    per_user_limit          int            default 1                 not null comment '每人限领',
    total_stock             int                                      not null comment '总库存',
    seckill_total_stock     int            default 0                 not null comment '秒杀库存',
    remaining_stock         int            default 0                 not null comment '剩余库存',
    seckill_remaining_stock int            default 0                 not null comment '秒杀剩余库存',
    status                  tinyint        default 0                 not null comment '状态',
    create_time             datetime       default CURRENT_TIMESTAMP not null,
    update_time             datetime       default CURRENT_TIMESTAMP not null on update CURRENT_TIMESTAMP,
    version                 int            default 0                 not null comment '乐观锁',
    shard_index             int                                      not null comment '分片索引',
    primary key (id, shard_index)
)
    comment '优惠券表' row_format = DYNAMIC;

create index idx_coupon_status
    on coupon_0 (status);

create table coupon_1
(
    id                      bigint                                   not null comment '优惠券ID',
    name                    varchar(100)                             not null comment '优惠券名称',
    description             varchar(500)                             null comment '描述',
    type                    tinyint                                  not null comment '类型(1-常驻,2-秒抢)',
    face_value              decimal(10, 2)                           not null comment '面值',
    min_spend               decimal(10, 2) default 0.00              not null comment '最低消费',
    valid_days              int            default 15                not null comment '有效期',
    per_user_limit          int            default 1                 not null comment '每人限领',
    total_stock             int                                      not null comment '总库存',
    seckill_total_stock     int            default 0                 not null comment '秒杀库存',
    remaining_stock         int            default 0                 not null comment '剩余库存',
    seckill_remaining_stock int            default 0                 not null comment '秒杀剩余库存',
    status                  tinyint        default 0                 not null comment '状态',
    create_time             datetime       default CURRENT_TIMESTAMP not null,
    update_time             datetime       default CURRENT_TIMESTAMP not null on update CURRENT_TIMESTAMP,
    version                 int            default 0                 not null comment '乐观锁',
    shard_index             int                                      not null comment '分片索引',
    primary key (id, shard_index)
)
    comment '优惠券表' row_format = DYNAMIC;

create index idx_coupon_status
    on coupon_1 (status);

create table coupon_10
(
    id                      bigint                                   not null comment '优惠券ID',
    name                    varchar(100)                             not null comment '优惠券名称',
    description             varchar(500)                             null comment '描述',
    type                    tinyint                                  not null comment '类型(1-常驻,2-秒抢)',
    face_value              decimal(10, 2)                           not null comment '面值',
    min_spend               decimal(10, 2) default 0.00              not null comment '最低消费',
    valid_days              int            default 15                not null comment '有效期',
    per_user_limit          int            default 1                 not null comment '每人限领',
    total_stock             int                                      not null comment '总库存',
    seckill_total_stock     int            default 0                 not null comment '秒杀库存',
    remaining_stock         int            default 0                 not null comment '剩余库存',
    seckill_remaining_stock int            default 0                 not null comment '秒杀剩余库存',
    status                  tinyint        default 0                 not null comment '状态',
    create_time             datetime       default CURRENT_TIMESTAMP not null,
    update_time             datetime       default CURRENT_TIMESTAMP not null on update CURRENT_TIMESTAMP,
    version                 int            default 0                 not null comment '乐观锁',
    shard_index             int                                      not null comment '分片索引',
    primary key (id, shard_index)
)
    comment '优惠券表' row_format = DYNAMIC;

create index idx_coupon_status
    on coupon_10 (status);

create table coupon_11
(
    id                      bigint                                   not null comment '优惠券ID',
    name                    varchar(100)                             not null comment '优惠券名称',
    description             varchar(500)                             null comment '描述',
    type                    tinyint                                  not null comment '类型(1-常驻,2-秒抢)',
    face_value              decimal(10, 2)                           not null comment '面值',
    min_spend               decimal(10, 2) default 0.00              not null comment '最低消费',
    valid_days              int            default 15                not null comment '有效期',
    per_user_limit          int            default 1                 not null comment '每人限领',
    total_stock             int                                      not null comment '总库存',
    seckill_total_stock     int            default 0                 not null comment '秒杀库存',
    remaining_stock         int            default 0                 not null comment '剩余库存',
    seckill_remaining_stock int            default 0                 not null comment '秒杀剩余库存',
    status                  tinyint        default 0                 not null comment '状态',
    create_time             datetime       default CURRENT_TIMESTAMP not null,
    update_time             datetime       default CURRENT_TIMESTAMP not null on update CURRENT_TIMESTAMP,
    version                 int            default 0                 not null comment '乐观锁',
    shard_index             int                                      not null comment '分片索引',
    primary key (id, shard_index)
)
    comment '优惠券表' row_format = DYNAMIC;

create index idx_coupon_status
    on coupon_11 (status);

create table coupon_12
(
    id                      bigint                                   not null comment '优惠券ID',
    name                    varchar(100)                             not null comment '优惠券名称',
    description             varchar(500)                             null comment '描述',
    type                    tinyint                                  not null comment '类型(1-常驻,2-秒抢)',
    face_value              decimal(10, 2)                           not null comment '面值',
    min_spend               decimal(10, 2) default 0.00              not null comment '最低消费',
    valid_days              int            default 15                not null comment '有效期',
    per_user_limit          int            default 1                 not null comment '每人限领',
    total_stock             int                                      not null comment '总库存',
    seckill_total_stock     int            default 0                 not null comment '秒杀库存',
    remaining_stock         int            default 0                 not null comment '剩余库存',
    seckill_remaining_stock int            default 0                 not null comment '秒杀剩余库存',
    status                  tinyint        default 0                 not null comment '状态',
    create_time             datetime       default CURRENT_TIMESTAMP not null,
    update_time             datetime       default CURRENT_TIMESTAMP not null on update CURRENT_TIMESTAMP,
    version                 int            default 0                 not null comment '乐观锁',
    shard_index             int                                      not null comment '分片索引',
    primary key (id, shard_index)
)
    comment '优惠券表' row_format = DYNAMIC;

create index idx_coupon_status
    on coupon_12 (status);

create table coupon_13
(
    id                      bigint                                   not null comment '优惠券ID',
    name                    varchar(100)                             not null comment '优惠券名称',
    description             varchar(500)                             null comment '描述',
    type                    tinyint                                  not null comment '类型(1-常驻,2-秒抢)',
    face_value              decimal(10, 2)                           not null comment '面值',
    min_spend               decimal(10, 2) default 0.00              not null comment '最低消费',
    valid_days              int            default 15                not null comment '有效期',
    per_user_limit          int            default 1                 not null comment '每人限领',
    total_stock             int                                      not null comment '总库存',
    seckill_total_stock     int            default 0                 not null comment '秒杀库存',
    remaining_stock         int            default 0                 not null comment '剩余库存',
    seckill_remaining_stock int            default 0                 not null comment '秒杀剩余库存',
    status                  tinyint        default 0                 not null comment '状态',
    create_time             datetime       default CURRENT_TIMESTAMP not null,
    update_time             datetime       default CURRENT_TIMESTAMP not null on update CURRENT_TIMESTAMP,
    version                 int            default 0                 not null comment '乐观锁',
    shard_index             int                                      not null comment '分片索引',
    primary key (id, shard_index)
)
    comment '优惠券表' row_format = DYNAMIC;

create index idx_coupon_status
    on coupon_13 (status);

create table coupon_14
(
    id                      bigint                                   not null comment '优惠券ID',
    name                    varchar(100)                             not null comment '优惠券名称',
    description             varchar(500)                             null comment '描述',
    type                    tinyint                                  not null comment '类型(1-常驻,2-秒抢)',
    face_value              decimal(10, 2)                           not null comment '面值',
    min_spend               decimal(10, 2) default 0.00              not null comment '最低消费',
    valid_days              int            default 15                not null comment '有效期',
    per_user_limit          int            default 1                 not null comment '每人限领',
    total_stock             int                                      not null comment '总库存',
    seckill_total_stock     int            default 0                 not null comment '秒杀库存',
    remaining_stock         int            default 0                 not null comment '剩余库存',
    seckill_remaining_stock int            default 0                 not null comment '秒杀剩余库存',
    status                  tinyint        default 0                 not null comment '状态',
    create_time             datetime       default CURRENT_TIMESTAMP not null,
    update_time             datetime       default CURRENT_TIMESTAMP not null on update CURRENT_TIMESTAMP,
    version                 int            default 0                 not null comment '乐观锁',
    shard_index             int                                      not null comment '分片索引',
    primary key (id, shard_index)
)
    comment '优惠券表' row_format = DYNAMIC;

create index idx_coupon_status
    on coupon_14 (status);

create table coupon_15
(
    id                      bigint                                   not null comment '优惠券ID',
    name                    varchar(100)                             not null comment '优惠券名称',
    description             varchar(500)                             null comment '描述',
    type                    tinyint                                  not null comment '类型(1-常驻,2-秒抢)',
    face_value              decimal(10, 2)                           not null comment '面值',
    min_spend               decimal(10, 2) default 0.00              not null comment '最低消费',
    valid_days              int            default 15                not null comment '有效期',
    per_user_limit          int            default 1                 not null comment '每人限领',
    total_stock             int                                      not null comment '总库存',
    seckill_total_stock     int            default 0                 not null comment '秒杀库存',
    remaining_stock         int            default 0                 not null comment '剩余库存',
    seckill_remaining_stock int            default 0                 not null comment '秒杀剩余库存',
    status                  tinyint        default 0                 not null comment '状态',
    create_time             datetime       default CURRENT_TIMESTAMP not null,
    update_time             datetime       default CURRENT_TIMESTAMP not null on update CURRENT_TIMESTAMP,
    version                 int            default 0                 not null comment '乐观锁',
    shard_index             int                                      not null comment '分片索引',
    primary key (id, shard_index)
)
    comment '优惠券表' row_format = DYNAMIC;

create index idx_coupon_status
    on coupon_15 (status);

create table coupon_2
(
    id                      bigint                                   not null comment '优惠券ID',
    name                    varchar(100)                             not null comment '优惠券名称',
    description             varchar(500)                             null comment '描述',
    type                    tinyint                                  not null comment '类型(1-常驻,2-秒抢)',
    face_value              decimal(10, 2)                           not null comment '面值',
    min_spend               decimal(10, 2) default 0.00              not null comment '最低消费',
    valid_days              int            default 15                not null comment '有效期',
    per_user_limit          int            default 1                 not null comment '每人限领',
    total_stock             int                                      not null comment '总库存',
    seckill_total_stock     int            default 0                 not null comment '秒杀库存',
    remaining_stock         int            default 0                 not null comment '剩余库存',
    seckill_remaining_stock int            default 0                 not null comment '秒杀剩余库存',
    status                  tinyint        default 0                 not null comment '状态',
    create_time             datetime       default CURRENT_TIMESTAMP not null,
    update_time             datetime       default CURRENT_TIMESTAMP not null on update CURRENT_TIMESTAMP,
    version                 int            default 0                 not null comment '乐观锁',
    shard_index             int                                      not null comment '分片索引',
    primary key (id, shard_index)
)
    comment '优惠券表' row_format = DYNAMIC;

create index idx_coupon_status
    on coupon_2 (status);

create table coupon_3
(
    id                      bigint                                   not null comment '优惠券ID',
    name                    varchar(100)                             not null comment '优惠券名称',
    description             varchar(500)                             null comment '描述',
    type                    tinyint                                  not null comment '类型(1-常驻,2-秒抢)',
    face_value              decimal(10, 2)                           not null comment '面值',
    min_spend               decimal(10, 2) default 0.00              not null comment '最低消费',
    valid_days              int            default 15                not null comment '有效期',
    per_user_limit          int            default 1                 not null comment '每人限领',
    total_stock             int                                      not null comment '总库存',
    seckill_total_stock     int            default 0                 not null comment '秒杀库存',
    remaining_stock         int            default 0                 not null comment '剩余库存',
    seckill_remaining_stock int            default 0                 not null comment '秒杀剩余库存',
    status                  tinyint        default 0                 not null comment '状态',
    create_time             datetime       default CURRENT_TIMESTAMP not null,
    update_time             datetime       default CURRENT_TIMESTAMP not null on update CURRENT_TIMESTAMP,
    version                 int            default 0                 not null comment '乐观锁',
    shard_index             int                                      not null comment '分片索引',
    primary key (id, shard_index)
)
    comment '优惠券表' row_format = DYNAMIC;

create index idx_coupon_status
    on coupon_3 (status);

create table coupon_4
(
    id                      bigint                                   not null comment '优惠券ID',
    name                    varchar(100)                             not null comment '优惠券名称',
    description             varchar(500)                             null comment '描述',
    type                    tinyint                                  not null comment '类型(1-常驻,2-秒抢)',
    face_value              decimal(10, 2)                           not null comment '面值',
    min_spend               decimal(10, 2) default 0.00              not null comment '最低消费',
    valid_days              int            default 15                not null comment '有效期',
    per_user_limit          int            default 1                 not null comment '每人限领',
    total_stock             int                                      not null comment '总库存',
    seckill_total_stock     int            default 0                 not null comment '秒杀库存',
    remaining_stock         int            default 0                 not null comment '剩余库存',
    seckill_remaining_stock int            default 0                 not null comment '秒杀剩余库存',
    status                  tinyint        default 0                 not null comment '状态',
    create_time             datetime       default CURRENT_TIMESTAMP not null,
    update_time             datetime       default CURRENT_TIMESTAMP not null on update CURRENT_TIMESTAMP,
    version                 int            default 0                 not null comment '乐观锁',
    shard_index             int                                      not null comment '分片索引',
    primary key (id, shard_index)
)
    comment '优惠券表' row_format = DYNAMIC;

create index idx_coupon_status
    on coupon_4 (status);

create table coupon_5
(
    id                      bigint                                   not null comment '优惠券ID',
    name                    varchar(100)                             not null comment '优惠券名称',
    description             varchar(500)                             null comment '描述',
    type                    tinyint                                  not null comment '类型(1-常驻,2-秒抢)',
    face_value              decimal(10, 2)                           not null comment '面值',
    min_spend               decimal(10, 2) default 0.00              not null comment '最低消费',
    valid_days              int            default 15                not null comment '有效期',
    per_user_limit          int            default 1                 not null comment '每人限领',
    total_stock             int                                      not null comment '总库存',
    seckill_total_stock     int            default 0                 not null comment '秒杀库存',
    remaining_stock         int            default 0                 not null comment '剩余库存',
    seckill_remaining_stock int            default 0                 not null comment '秒杀剩余库存',
    status                  tinyint        default 0                 not null comment '状态',
    create_time             datetime       default CURRENT_TIMESTAMP not null,
    update_time             datetime       default CURRENT_TIMESTAMP not null on update CURRENT_TIMESTAMP,
    version                 int            default 0                 not null comment '乐观锁',
    shard_index             int                                      not null comment '分片索引',
    primary key (id, shard_index)
)
    comment '优惠券表' row_format = DYNAMIC;

create index idx_coupon_status
    on coupon_5 (status);

create table coupon_6
(
    id                      bigint                                   not null comment '优惠券ID',
    name                    varchar(100)                             not null comment '优惠券名称',
    description             varchar(500)                             null comment '描述',
    type                    tinyint                                  not null comment '类型(1-常驻,2-秒抢)',
    face_value              decimal(10, 2)                           not null comment '面值',
    min_spend               decimal(10, 2) default 0.00              not null comment '最低消费',
    valid_days              int            default 15                not null comment '有效期',
    per_user_limit          int            default 1                 not null comment '每人限领',
    total_stock             int                                      not null comment '总库存',
    seckill_total_stock     int            default 0                 not null comment '秒杀库存',
    remaining_stock         int            default 0                 not null comment '剩余库存',
    seckill_remaining_stock int            default 0                 not null comment '秒杀剩余库存',
    status                  tinyint        default 0                 not null comment '状态',
    create_time             datetime       default CURRENT_TIMESTAMP not null,
    update_time             datetime       default CURRENT_TIMESTAMP not null on update CURRENT_TIMESTAMP,
    version                 int            default 0                 not null comment '乐观锁',
    shard_index             int                                      not null comment '分片索引',
    primary key (id, shard_index)
)
    comment '优惠券表' row_format = DYNAMIC;

create index idx_coupon_status
    on coupon_6 (status);

create table coupon_7
(
    id                      bigint                                   not null comment '优惠券ID',
    name                    varchar(100)                             not null comment '优惠券名称',
    description             varchar(500)                             null comment '描述',
    type                    tinyint                                  not null comment '类型(1-常驻,2-秒抢)',
    face_value              decimal(10, 2)                           not null comment '面值',
    min_spend               decimal(10, 2) default 0.00              not null comment '最低消费',
    valid_days              int            default 15                not null comment '有效期',
    per_user_limit          int            default 1                 not null comment '每人限领',
    total_stock             int                                      not null comment '总库存',
    seckill_total_stock     int            default 0                 not null comment '秒杀库存',
    remaining_stock         int            default 0                 not null comment '剩余库存',
    seckill_remaining_stock int            default 0                 not null comment '秒杀剩余库存',
    status                  tinyint        default 0                 not null comment '状态',
    create_time             datetime       default CURRENT_TIMESTAMP not null,
    update_time             datetime       default CURRENT_TIMESTAMP not null on update CURRENT_TIMESTAMP,
    version                 int            default 0                 not null comment '乐观锁',
    shard_index             int                                      not null comment '分片索引',
    primary key (id, shard_index)
)
    comment '优惠券表' row_format = DYNAMIC;

create index idx_coupon_status
    on coupon_7 (status);

create table coupon_8
(
    id                      bigint                                   not null comment '优惠券ID',
    name                    varchar(100)                             not null comment '优惠券名称',
    description             varchar(500)                             null comment '描述',
    type                    tinyint                                  not null comment '类型(1-常驻,2-秒抢)',
    face_value              decimal(10, 2)                           not null comment '面值',
    min_spend               decimal(10, 2) default 0.00              not null comment '最低消费',
    valid_days              int            default 15                not null comment '有效期',
    per_user_limit          int            default 1                 not null comment '每人限领',
    total_stock             int                                      not null comment '总库存',
    seckill_total_stock     int            default 0                 not null comment '秒杀库存',
    remaining_stock         int            default 0                 not null comment '剩余库存',
    seckill_remaining_stock int            default 0                 not null comment '秒杀剩余库存',
    status                  tinyint        default 0                 not null comment '状态',
    create_time             datetime       default CURRENT_TIMESTAMP not null,
    update_time             datetime       default CURRENT_TIMESTAMP not null on update CURRENT_TIMESTAMP,
    version                 int            default 0                 not null comment '乐观锁',
    shard_index             int                                      not null comment '分片索引',
    primary key (id, shard_index)
)
    comment '优惠券表' row_format = DYNAMIC;

create index idx_coupon_status
    on coupon_8 (status);

create table coupon_9
(
    id                      bigint                                   not null comment '优惠券ID',
    name                    varchar(100)                             not null comment '优惠券名称',
    description             varchar(500)                             null comment '描述',
    type                    tinyint                                  not null comment '类型(1-常驻,2-秒抢)',
    face_value              decimal(10, 2)                           not null comment '面值',
    min_spend               decimal(10, 2) default 0.00              not null comment '最低消费',
    valid_days              int            default 15                not null comment '有效期',
    per_user_limit          int            default 1                 not null comment '每人限领',
    total_stock             int                                      not null comment '总库存',
    seckill_total_stock     int            default 0                 not null comment '秒杀库存',
    remaining_stock         int            default 0                 not null comment '剩余库存',
    seckill_remaining_stock int            default 0                 not null comment '秒杀剩余库存',
    status                  tinyint        default 0                 not null comment '状态',
    create_time             datetime       default CURRENT_TIMESTAMP not null,
    update_time             datetime       default CURRENT_TIMESTAMP not null on update CURRENT_TIMESTAMP,
    version                 int            default 0                 not null comment '乐观锁',
    shard_index             int                                      not null comment '分片索引',
    primary key (id, shard_index)
)
    comment '优惠券表' row_format = DYNAMIC;

create index idx_coupon_status
    on coupon_9 (status);

create table stock_log_0
(
    id           bigint auto_increment comment '日志ID'
        primary key,
    coupon_id    bigint                             not null comment '优惠券ID',
    shard_index  int                                not null comment '分片索引',
    order_id     bigint                             null comment '订单ID',
    quantity     int                                not null comment '变动数量',
    operate_type tinyint                            not null comment '操作类型',
    stock_after  int                                not null comment '操作后库存',
    create_time  datetime default CURRENT_TIMESTAMP not null
)
    comment '库存日志表';

create index idx_coupon_id
    on stock_log_0 (coupon_id);

create table stock_log_1
(
    id           bigint auto_increment comment '日志ID'
        primary key,
    coupon_id    bigint                             not null comment '优惠券ID',
    shard_index  int                                not null comment '分片索引',
    order_id     bigint                             null comment '订单ID',
    quantity     int                                not null comment '变动数量',
    operate_type tinyint                            not null comment '操作类型',
    stock_after  int                                not null comment '操作后库存',
    create_time  datetime default CURRENT_TIMESTAMP not null
)
    comment '库存日志表';

create index idx_coupon_id
    on stock_log_1 (coupon_id);

create table stock_log_10
(
    id           bigint auto_increment comment '日志ID'
        primary key,
    coupon_id    bigint                             not null comment '优惠券ID',
    shard_index  int                                not null comment '分片索引',
    order_id     bigint                             null comment '订单ID',
    quantity     int                                not null comment '变动数量',
    operate_type tinyint                            not null comment '操作类型',
    stock_after  int                                not null comment '操作后库存',
    create_time  datetime default CURRENT_TIMESTAMP not null
)
    comment '库存日志表';

create index idx_coupon_id
    on stock_log_10 (coupon_id);

create table stock_log_11
(
    id           bigint auto_increment comment '日志ID'
        primary key,
    coupon_id    bigint                             not null comment '优惠券ID',
    shard_index  int                                not null comment '分片索引',
    order_id     bigint                             null comment '订单ID',
    quantity     int                                not null comment '变动数量',
    operate_type tinyint                            not null comment '操作类型',
    stock_after  int                                not null comment '操作后库存',
    create_time  datetime default CURRENT_TIMESTAMP not null
)
    comment '库存日志表';

create index idx_coupon_id
    on stock_log_11 (coupon_id);

create table stock_log_12
(
    id           bigint auto_increment comment '日志ID'
        primary key,
    coupon_id    bigint                             not null comment '优惠券ID',
    shard_index  int                                not null comment '分片索引',
    order_id     bigint                             null comment '订单ID',
    quantity     int                                not null comment '变动数量',
    operate_type tinyint                            not null comment '操作类型',
    stock_after  int                                not null comment '操作后库存',
    create_time  datetime default CURRENT_TIMESTAMP not null
)
    comment '库存日志表';

create index idx_coupon_id
    on stock_log_12 (coupon_id);

create table stock_log_13
(
    id           bigint auto_increment comment '日志ID'
        primary key,
    coupon_id    bigint                             not null comment '优惠券ID',
    shard_index  int                                not null comment '分片索引',
    order_id     bigint                             null comment '订单ID',
    quantity     int                                not null comment '变动数量',
    operate_type tinyint                            not null comment '操作类型',
    stock_after  int                                not null comment '操作后库存',
    create_time  datetime default CURRENT_TIMESTAMP not null
)
    comment '库存日志表';

create index idx_coupon_id
    on stock_log_13 (coupon_id);

create table stock_log_14
(
    id           bigint auto_increment comment '日志ID'
        primary key,
    coupon_id    bigint                             not null comment '优惠券ID',
    shard_index  int                                not null comment '分片索引',
    order_id     bigint                             null comment '订单ID',
    quantity     int                                not null comment '变动数量',
    operate_type tinyint                            not null comment '操作类型',
    stock_after  int                                not null comment '操作后库存',
    create_time  datetime default CURRENT_TIMESTAMP not null
)
    comment '库存日志表';

create index idx_coupon_id
    on stock_log_14 (coupon_id);

create table stock_log_15
(
    id           bigint auto_increment comment '日志ID'
        primary key,
    coupon_id    bigint                             not null comment '优惠券ID',
    shard_index  int                                not null comment '分片索引',
    order_id     bigint                             null comment '订单ID',
    quantity     int                                not null comment '变动数量',
    operate_type tinyint                            not null comment '操作类型',
    stock_after  int                                not null comment '操作后库存',
    create_time  datetime default CURRENT_TIMESTAMP not null
)
    comment '库存日志表';

create index idx_coupon_id
    on stock_log_15 (coupon_id);

create table stock_log_2
(
    id           bigint auto_increment comment '日志ID'
        primary key,
    coupon_id    bigint                             not null comment '优惠券ID',
    shard_index  int                                not null comment '分片索引',
    order_id     bigint                             null comment '订单ID',
    quantity     int                                not null comment '变动数量',
    operate_type tinyint                            not null comment '操作类型',
    stock_after  int                                not null comment '操作后库存',
    create_time  datetime default CURRENT_TIMESTAMP not null
)
    comment '库存日志表';

create index idx_coupon_id
    on stock_log_2 (coupon_id);

create table stock_log_3
(
    id           bigint auto_increment comment '日志ID'
        primary key,
    coupon_id    bigint                             not null comment '优惠券ID',
    shard_index  int                                not null comment '分片索引',
    order_id     bigint                             null comment '订单ID',
    quantity     int                                not null comment '变动数量',
    operate_type tinyint                            not null comment '操作类型',
    stock_after  int                                not null comment '操作后库存',
    create_time  datetime default CURRENT_TIMESTAMP not null
)
    comment '库存日志表';

create index idx_coupon_id
    on stock_log_3 (coupon_id);

create table stock_log_4
(
    id           bigint auto_increment comment '日志ID'
        primary key,
    coupon_id    bigint                             not null comment '优惠券ID',
    shard_index  int                                not null comment '分片索引',
    order_id     bigint                             null comment '订单ID',
    quantity     int                                not null comment '变动数量',
    operate_type tinyint                            not null comment '操作类型',
    stock_after  int                                not null comment '操作后库存',
    create_time  datetime default CURRENT_TIMESTAMP not null
)
    comment '库存日志表';

create index idx_coupon_id
    on stock_log_4 (coupon_id);

create table stock_log_5
(
    id           bigint auto_increment comment '日志ID'
        primary key,
    coupon_id    bigint                             not null comment '优惠券ID',
    shard_index  int                                not null comment '分片索引',
    order_id     bigint                             null comment '订单ID',
    quantity     int                                not null comment '变动数量',
    operate_type tinyint                            not null comment '操作类型',
    stock_after  int                                not null comment '操作后库存',
    create_time  datetime default CURRENT_TIMESTAMP not null
)
    comment '库存日志表';

create index idx_coupon_id
    on stock_log_5 (coupon_id);

create table stock_log_6
(
    id           bigint auto_increment comment '日志ID'
        primary key,
    coupon_id    bigint                             not null comment '优惠券ID',
    shard_index  int                                not null comment '分片索引',
    order_id     bigint                             null comment '订单ID',
    quantity     int                                not null comment '变动数量',
    operate_type tinyint                            not null comment '操作类型',
    stock_after  int                                not null comment '操作后库存',
    create_time  datetime default CURRENT_TIMESTAMP not null
)
    comment '库存日志表';

create index idx_coupon_id
    on stock_log_6 (coupon_id);

create table stock_log_7
(
    id           bigint auto_increment comment '日志ID'
        primary key,
    coupon_id    bigint                             not null comment '优惠券ID',
    shard_index  int                                not null comment '分片索引',
    order_id     bigint                             null comment '订单ID',
    quantity     int                                not null comment '变动数量',
    operate_type tinyint                            not null comment '操作类型',
    stock_after  int                                not null comment '操作后库存',
    create_time  datetime default CURRENT_TIMESTAMP not null
)
    comment '库存日志表';

create index idx_coupon_id
    on stock_log_7 (coupon_id);

create table stock_log_8
(
    id           bigint auto_increment comment '日志ID'
        primary key,
    coupon_id    bigint                             not null comment '优惠券ID',
    shard_index  int                                not null comment '分片索引',
    order_id     bigint                             null comment '订单ID',
    quantity     int                                not null comment '变动数量',
    operate_type tinyint                            not null comment '操作类型',
    stock_after  int                                not null comment '操作后库存',
    create_time  datetime default CURRENT_TIMESTAMP not null
)
    comment '库存日志表';

create index idx_coupon_id
    on stock_log_8 (coupon_id);

create table stock_log_9
(
    id           bigint auto_increment comment '日志ID'
        primary key,
    coupon_id    bigint                             not null comment '优惠券ID',
    shard_index  int                                not null comment '分片索引',
    order_id     bigint                             null comment '订单ID',
    quantity     int                                not null comment '变动数量',
    operate_type tinyint                            not null comment '操作类型',
    stock_after  int                                not null comment '操作后库存',
    create_time  datetime default CURRENT_TIMESTAMP not null
)
    comment '库存日志表';

create index idx_coupon_id
    on stock_log_9 (coupon_id);

