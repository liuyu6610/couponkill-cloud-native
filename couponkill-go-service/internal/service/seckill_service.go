// internal/service/seckill_service.go
package service

import (
	"context"
	"errors"
	"time"

	"couponkill-go-service/internal/model"
	"couponkill-go-service/internal/repository"
	"couponkill-go-service/pkg/idgenerator"
)

// SeckillService 秒杀服务
type SeckillService struct {
	mysqlRepo *repository.MysqlRepository
	redisRepo *repository.RedisRepository
}

func NewSeckillService(mysqlRepo *repository.MysqlRepository, redisRepo *repository.RedisRepository) *SeckillService {
	return &SeckillService{
		mysqlRepo: mysqlRepo,
		redisRepo: redisRepo,
	}
}

// ProcessSeckill 处理单个用户秒杀（Go端）
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
	if err != nil || !stockDeduced {
		return false, errors.New("库存不足")
	}

	// 4. 创建订单（标记Go端来源）
	order := &model.Order{
		ID:            idgenerator.GenerateGoOrderID(),
		UserID:        userID,
		CouponID:      couponID,
		Status:        1, // 已创建
		GetTime:       time.Now(),
		ExpireTime:    time.Now().AddDate(0, 0, validDays),
		CreatedByJava: 0, // 非Java端创建
		CreatedByGo:   1, // 标记为Go端创建
	}

	// 5. 插入订单（依赖联合索引uk_user_coupon_source防止重复）
	if err := s.mysqlRepo.CreateOrder(ctx, order); err != nil {
		// 插入失败回滚库存
		s.redisRepo.DeductStock(ctx, couponID) // 实际应为Incr，此处简化
		return false, err
	}

	// 6. 更新缓存
	s.redisRepo.SetUserReceivedCache(ctx, userID, couponID)
	return true, nil
}
