// com.aliyun.seckill.common.pojo.User.java
package com.aliyun.seckill.common.pojo;

import lombok.Data;

import java.io.Serializable;
import java.time.LocalDateTime;

@Data

public class User extends UserCouponCount implements Serializable {
    private static final long serialVersionUID = 1L;


    private Long id;

    private String username;

    private String password;

    private String phone;
    private String email;

    private Integer status;

    private LocalDateTime createTime;

    private LocalDateTime updateTime;
}