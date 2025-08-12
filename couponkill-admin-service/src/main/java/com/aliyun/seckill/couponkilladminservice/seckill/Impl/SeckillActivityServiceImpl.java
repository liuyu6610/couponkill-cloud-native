// 文件路径: com/aliyun/seckill/couponkilladminservice/seckill/Impl/SeckillActivityServiceImpl.java
package com.aliyun.seckill.couponkilladminservice.seckill.Impl;

import com.aliyun.seckill.common.pojo.SeckillActivity;
import com.aliyun.seckill.couponkilladminservice.mapper.SeckillActivityMapper;
import com.aliyun.seckill.couponkilladminservice.seckill.SeckillActivityService;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Service;

import java.time.LocalDateTime;
import java.util.List;

@Service
public class SeckillActivityServiceImpl implements SeckillActivityService {

    @Autowired
    private SeckillActivityMapper seckillActivityMapper;

    @Override
    public SeckillActivity createActivity(SeckillActivity activity) {
        activity.setCreateTime(LocalDateTime.now());
        activity.setUpdateTime(LocalDateTime.now());
        seckillActivityMapper.insert(activity);
        return activity;
    }

    @Override
    public Boolean updateActivityStatus(Long id, Integer status) {
        return seckillActivityMapper.updateStatus(id, status, LocalDateTime.now()) > 0;
    }

    @Override
    public List<SeckillActivity> getAllActivities() {
        return seckillActivityMapper.selectAll();
    }

    @Override
    public List<SeckillActivity> getInactiveActivities() {
        return seckillActivityMapper.selectInactive();
    }
}
