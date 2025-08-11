package com.aliyun.seckill.order.domain;
import jakarta.persistence.*;
import lombok.*;
import java.time.Instant;
@Entity @Table(name="orders", uniqueConstraints=@UniqueConstraint(columnNames={"requestId"}))
@Getter
@Setter
@Builder
@AllArgsConstructor
@NoArgsConstructor
public class OrderEntity {
    @Id @GeneratedValue(strategy=GenerationType.IDENTITY) private Long id;
    private String requestId;
    private String couponId;
    private String userId;
    private String status; // CREATED/PAID/CANCELLED
    private Instant createdAt;
}
