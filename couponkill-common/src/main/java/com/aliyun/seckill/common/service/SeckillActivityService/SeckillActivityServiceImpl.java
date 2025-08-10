package com.aliyun.seckill.common.service.SeckillActivityService;

import com.aliyun.seckill.common.mapper.SeckillActivityMapper;
import com.aliyun.seckill.common.pojo.SeckillActivity;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Service;

import java.util.List;
@Service
public class SeckillActivityServiceImpl implements SeckillActivityService {
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
        return seckillActivityMapper.selectList(null);
    }

    @Override
    public List<SeckillActivity> getInactiveActivities () {
        return List.of();
    }
}
