// 文件路径: com/aliyun/seckill/couponkilladminservice/mapper/SeckillActivityMapper.java
package com.aliyun.seckill.couponkilladminservice.mapper;

import com.aliyun.seckill.common.pojo.SeckillActivity;
import org.apache.ibatis.annotations.*;

import java.util.List;

@Mapper
public interface SeckillActivityMapper {

    @Insert("INSERT INTO seckill_activity(coupon_id, start_time, end_time, status, create_time, update_time) " +
            "VALUES(#{couponId}, #{startTime}, #{endTime}, #{status}, #{createTime}, #{updateTime})")
    @Options(useGeneratedKeys = true, keyProperty = "id")
    int insert(SeckillActivity activity);

    @Update("UPDATE seckill_activity SET status = #{status}, update_time = #{updateTime} WHERE id = #{id}")
    int updateStatus(@Param("id") Long id, @Param("status") Integer status, @Param("updateTime") java.time.LocalDateTime updateTime);

    @Select("SELECT * FROM seckill_activity")
    List<SeckillActivity> selectAll();

    @Select("SELECT * FROM seckill_activity WHERE status = 0")
    List<SeckillActivity> selectInactive();

    @Select("SELECT * FROM seckill_activity WHERE id = #{id}")
    SeckillActivity selectById(@Param("id") Long id);
}
