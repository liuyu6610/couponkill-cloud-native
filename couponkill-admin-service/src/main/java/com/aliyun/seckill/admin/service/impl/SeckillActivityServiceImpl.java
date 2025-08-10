// com.aliyun.seckill.admin.service.impl.SeckillActivityServiceImpl.java
package com.aliyun.seckill.admin.service.impl;

import com.aliyun.seckill.admin.mapper.SeckillActivityMapper;
import com.aliyun.seckill.admin.service.SeckillActivityService;
import com.aliyun.seckill.common.exception.BusinessException;
import com.aliyun.seckill.common.feign.CouponFeignService;
import com.aliyun.seckill.common.enums.ResultCode;
import com.aliyun.seckill.common.pojo.Coupon;
import com.aliyun.seckill.common.pojo.SeckillActivity;
import com.baomidou.mybatisplus.core.conditions.query.QueryWrapper;
import com.baomidou.mybatisplus.extension.service.impl.ServiceImpl;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.LocalDateTime;
import java.util.List;

@Service
public class SeckillActivityServiceImpl extends ServiceImpl<SeckillActivityMapper, SeckillActivity> implements SeckillActivityService {

    @Autowired
    private SeckillActivityMapper activityMapper;

    @Autowired
    private CouponFeignService couponFeignService;

    @Override
    @Transactional
    public SeckillActivity createActivity(SeckillActivity activity) {
        // 检查优惠券是否存在
        Coupon coupon = couponFeignService.getCouponById(activity.getCouponId()).getData();
        if (coupon == null) {
            throw new BusinessException(ResultCode.COUPON_NOT_FOUND);
        }

        // 设置初始状态
        LocalDateTime now = LocalDateTime.now();
        if (activity.getStartTime().isAfter(now)) {
            activity.setStatus(0); // 未开始
        } else if (activity.getEndTime().isBefore(now)) {
            activity.setStatus(2); // 已结束
        } else {
            activity.setStatus(1); // 进行中
        }

        activity.setCreateTime(now);
        activity.setUpdateTime(now);
        activityMapper.insert(activity);
        return activity;
    }

    @Override
    @Transactional
    public boolean updateActivityStatus(Long id, Integer status) {
        SeckillActivity activity = getById(id);
        if (activity == null) {
            return false;
        }

        activity.setStatus(status);
        activity.setUpdateTime(LocalDateTime.now());
        return updateById(activity);
    }

    @Override
    public List<SeckillActivity> getAllActivities() {
        return list();
    }

    @Override
    public List<SeckillActivity> getActiveActivities() {
        LocalDateTime now = LocalDateTime.now();
        QueryWrapper<SeckillActivity> queryWrapper = new QueryWrapper<>();
        queryWrapper.eq("status", 1)
                .lt("start_time", now)
                .gt("end_time", now);
        return list(queryWrapper);
    }

    // 定时任务，每分钟检查一次活动状态
    @Scheduled(cron = "0 * * * * ?")
    public void checkActivityStatus() {
        LocalDateTime now = LocalDateTime.now();

        // 将未开始且已到时间的活动设置为进行中
        QueryWrapper<SeckillActivity> startWrapper = new QueryWrapper<>();
        startWrapper.eq("status", 0)
                .le("start_time", now)
                .gt("end_time", now);
        SeckillActivity startActivity = new SeckillActivity();
        startActivity.setStatus(1);
        startActivity.setUpdateTime(now);
        update(startActivity, startWrapper);

        // 将进行中且已结束的活动设置为已结束
        QueryWrapper<SeckillActivity> endWrapper = new QueryWrapper<>();
        endWrapper.eq("status", 1)
                .le("end_time", now);
        SeckillActivity endActivity = new SeckillActivity();
        endActivity.setStatus(2);
        endActivity.setUpdateTime(now);
        update(endActivity, endWrapper);
    }
}