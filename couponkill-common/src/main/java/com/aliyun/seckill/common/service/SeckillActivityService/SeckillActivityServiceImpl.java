package com.aliyun.seckill.common.service.SeckillActivityService;

import com.aliyun.seckill.common.mapper.SeckillActivityMapper;
import com.aliyun.seckill.common.pojo.SeckillActivity;
import com.baomidou.mybatisplus.core.conditions.query.QueryWrapper;
import com.baomidou.mybatisplus.extension.service.impl.ServiceImpl;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Service;

import java.util.List;
@Service
public class SeckillActivityServiceImpl extends ServiceImpl<SeckillActivityMapper, SeckillActivity> implements SeckillActivityService {
    @Autowired
    private SeckillActivityMapper seckillActivityMapper;
    @Override
    public SeckillActivity createActivity(SeckillActivity activity) {
        return seckillActivityMapper.insert(activity) > 0 ? activity : null;
    }

    @Override
    public Boolean updateActivityStatus(Long id, Integer status) {
        SeckillActivity activity = new SeckillActivity();
        activity.setId(id);
        activity.setStatus(status);
        return seckillActivityMapper.updateById(activity) > 0;
    }

    @Override
    public List<SeckillActivity> getAllActivities() {
        return list();
    }

    @Override
    public List<SeckillActivity> getInactiveActivities() {
        QueryWrapper<SeckillActivity> queryWrapper = new QueryWrapper<>();
        queryWrapper.eq("status", 0);
        return list(queryWrapper);
    }
}
