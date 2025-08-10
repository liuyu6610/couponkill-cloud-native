// com.aliyun.seckill.admin.service.SeckillActivityService.java
package com.aliyun.seckill.admin.service;

import com.aliyun.seckill.common.pojo.SeckillActivity;
import com.baomidou.mybatisplus.extension.service.IService;

import java.util.List;

public interface SeckillActivityService extends IService<SeckillActivity> {
    /**
     * 创建秒杀活动
     */
    SeckillActivity createActivity(SeckillActivity activity);

    /**
     * 更新秒杀活动状态
     */
    boolean updateActivityStatus(Long id, Integer status);

    /**
     * 获取所有秒杀活动
     */
    List<SeckillActivity> getAllActivities();

    /**
     * 获取进行中的秒杀活动
     */
    List<SeckillActivity> getActiveActivities();
}