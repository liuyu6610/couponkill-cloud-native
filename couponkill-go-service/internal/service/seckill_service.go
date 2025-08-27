package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"couponkill-go-service/internal/model"
	"couponkill-go-service/internal/repository"
	"couponkill-go-service/pkg/idgenerator"

	"github.com/google/uuid"
)

// SeckillService 秒杀服务
type SeckillService struct {
	mysqlRepo *repository.MysqlRepository
	redisRepo *repository.RedisRepository
}

// NewSeckillService 初始化秒杀服务
func NewSeckillService(mysqlRepo *repository.MysqlRepository, redisRepo *repository.RedisRepository) *SeckillService {
	return &SeckillService{
		mysqlRepo: mysqlRepo,
		redisRepo: redisRepo,
	}
}

// ProcessSeckill 处理单个用户秒杀（Go端独立处理完整流程）
func (s *SeckillService) ProcessSeckill(ctx context.Context, userID, couponID int64, validDays int) (bool, error) {
	// 1. 先查缓存：用户是否已领取
	cacheReceived, err := s.redisRepo.CheckUserReceivedCache(ctx, userID, couponID)
	if err != nil {
		return false, err
	}
	if cacheReceived {
		return false, errors.New("用户已领取")
	}

	// 2. 查DB确认：两端均未领取（双字段均为0）
	dbReceived, err := s.mysqlRepo.CheckUserReceived(ctx, userID, couponID)
	if err != nil {
		return false, err
	}
	if dbReceived {
		return false, errors.New("用户已领取")
	}

	// 3. 扣减库存（Redis原子操作）
	stockDeduced, err := s.redisRepo.DeductStock(ctx, couponID)
	if err != nil {
		return false, fmt.Errorf("扣减库存失败: %w", err)
	}
	if !stockDeduced {
		return false, errors.New("库存不足")
	}

	// 4. 创建订单（标记Go端来源）
	order := &model.Order{
		ID:            idgenerator.GenerateGoOrderID(), // 使用Go端雪花算法生成唯一ID
		UserID:        userID,
		CouponID:      couponID,
		VirtualID:     fmt.Sprintf("%d_%d", couponID, couponID%16), // 生成虚拟ID，与Java端保持一致
		Status:        1,                                           // 已创建
		GetTime:       time.Now(),
		ExpireTime:    time.Now().AddDate(0, 0, validDays),
		CreatedByJava: 0, // 非Java端创建
		CreatedByGo:   1, // 标记为Go端创建
		RequestID:     uuid.New().String(),
		Version:       0,
		CreateTime:    time.Now(),
		UpdateTime:    time.Now(),
	}

	// 5. 插入订单（依赖联合索引uk_user_coupon_source防止重复）
	if err := s.mysqlRepo.CreateOrder(ctx, order); err != nil {
		// 插入失败回滚库存
		s.redisRepo.IncrStock(ctx, couponID) // 简化处理
		return false, err
	}

	// 6. 更新用户优惠券数量统计
	if err := s.mysqlRepo.UpdateUserCouponCount(ctx, userID, 1, 1); err != nil {
		// 如果更新失败，需要回滚订单和库存
		s.mysqlRepo.DeleteOrder(ctx, order.ID, userID)
		s.redisRepo.IncrStock(ctx, couponID)
		return false, fmt.Errorf("更新用户优惠券数量失败: %w", err)
	}

	// 7. 更新优惠券库存（在数据库中同步更新）
	if err := s.mysqlRepo.UpdateCouponStock(ctx, couponID, -1); err != nil {
		// 如果更新失败，需要回滚之前的更改
		s.mysqlRepo.DeleteOrder(ctx, order.ID, userID)
		s.mysqlRepo.UpdateUserCouponCount(ctx, userID, -1, -1)
		s.redisRepo.IncrStock(ctx, couponID)
		return false, fmt.Errorf("更新优惠券库存失败: %w", err)
	}

	// 8. 更新缓存
	s.redisRepo.SetUserReceivedCache(ctx, userID, couponID)

	// 更新用户优惠券数量缓存
	s.redisRepo.UpdateUserCouponCountCache(ctx, userID, 1, 1)

	// 更新优惠券库存缓存
	s.redisRepo.UpdateCouponStockCache(ctx, couponID, -1)

	return true, nil
}
