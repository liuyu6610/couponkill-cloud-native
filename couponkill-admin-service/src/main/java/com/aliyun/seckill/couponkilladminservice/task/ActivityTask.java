package com.aliyun.seckill.couponkilladminservice.task;

import com.aliyun.seckill.common.pojo.SeckillActivity;
import com.aliyun.seckill.couponkilladminservice.seckill.SeckillActivityService;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Component;

import java.time.LocalDateTime;
import java.util.List;
import java.util.Random;
@Slf4j
@Component
public class ActivityTask {

    @Autowired
    private SeckillActivityService seckillActivityService;

    // 每天晚上8点执行 (cron表达式: 秒 分 时 日 月 周)
    @Scheduled(cron = "0 0 20 * * ?")
    public void scheduleSeckillActivities() {
        // 获取所有未开启秒杀的活动
        List<SeckillActivity> inactiveActivities = seckillActivityService.getInactiveActivities();

        // 如果未开启的活动少于2个，则全部开启
        if (inactiveActivities.size() < 2) {
            log.info( "未开启秒杀活动数量不足太少,请快点查看状态" );

        } else {
            // 随机选择2个活动开启
            Random random = new Random();
            for (int i = 0; i < 2; i++) {
                int index = random.nextInt(inactiveActivities.size());
                SeckillActivity selectedActivity = inactiveActivities.get(index);
                startSeckillActivity(selectedActivity,index);
                // 移除已选择的活动，避免重复选择
                inactiveActivities.remove(index);
            }
        }
    }

    /**
     * 开启秒杀活动，设置状态为开启并设置30分钟后结束
     */
    private void startSeckillActivity(SeckillActivity activity,int index) {
        // 设置活动状态为开启(假设1表示开启)
        activity.setStatus(1);
        // 设置开始时间为现在
        activity.setStartTime(LocalDateTime.now());
        // 设置结束时间为30分钟后
        activity.setEndTime(LocalDateTime.now().plusMinutes(30));
        // 更新活动信息
        seckillActivityService.updateActivityStatus((long)index,0);
    }
}
