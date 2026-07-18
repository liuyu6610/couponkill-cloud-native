// com.aliyun.seckill.common.pojo.User.java
package com.aliyun.seckill.common.pojo;

import com.fasterxml.jackson.annotation.JsonFormat;
import lombok.Data;

import java.io.Serializable;
import java.time.LocalDateTime;

@Data

public class User implements Serializable {
    private static final long serialVersionUID = 1L;


    /** 跨 JS 边界以字符串传输，避免大整数精度丢失 */
    @JsonFormat(shape = JsonFormat.Shape.STRING)
    private Long id; // 用户ID（对应表中id）


    private String username;


    private String password;


    private String phone;


    private String email;


    private Integer status;


    private LocalDateTime createTime;


    private LocalDateTime updateTime;


    private LocalDateTime lastActiveTime; // 补充表中存在的字段
}