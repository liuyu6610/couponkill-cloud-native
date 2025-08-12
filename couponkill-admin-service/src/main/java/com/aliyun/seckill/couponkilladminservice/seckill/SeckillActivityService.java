// com/aliyun/seckill/couponkilladminservice/service/seckill/SeckillActivityService.java
package com.aliyun.seckill.couponkilladminservice.seckill;

import com.aliyun.seckill.common.pojo.SeckillActivity;

import java.util.List;

public interface SeckillActivityService {
    /**
     * 创建秒杀活动
     * @param activity
     * @return
     */
    SeckillActivity createActivity(SeckillActivity activity);

    /**
     * 更新秒杀活动状态
     * @param id
     * @param status
     * @return
     */
    Boolean updateActivityStatus(Long id, Integer status);

    /**
     * 获取所有秒杀活动
     * @return
     */
    List<SeckillActivity> getAllActivities();

    List<SeckillActivity> getInactiveActivities();
}
