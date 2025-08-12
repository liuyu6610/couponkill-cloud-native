package com.aliyun.seckill.couponkillorderservice.domain;
import org.springframework.data.jpa.repository.JpaRepository;
import java.util.Optional;
public interface OrderRepo extends JpaRepository<OrderEntity, Long> {
    Optional<OrderEntity> findByRequestId(String requestId);
}
