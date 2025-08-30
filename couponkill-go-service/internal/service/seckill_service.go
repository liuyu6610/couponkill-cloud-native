package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"couponkill-go-service/internal/model"
	"couponkill-go-service/internal/repository"
	"couponkill-go-service/pkg/idgenerator"
	"couponkill-go-service/pkg/pool"
	"couponkill-go-service/pkg/sharding"

	"github.com/google/uuid"
)

// SeckillService 秒杀服务
type SeckillService struct {
	mysqlRepo      *repository.MysqlRepository
	redisRepo      *repository.RedisRepository
	workerPool     *pool.WorkerPool
	couponSharding *sharding.CouponSharding
}

// NewSeckillService 初始化秒杀服务
func NewSeckillService(mysqlRepo *repository.MysqlRepository, redisRepo *repository.RedisRepository) *SeckillService {
	// 创建一个包含100个工作协程的工作池
	workerPool := pool.NewWorkerPool(100)

	return &SeckillService{
		mysqlRepo:      mysqlRepo,
		redisRepo:      redisRepo,
		workerPool:     workerPool,
		couponSharding: &sharding.CouponSharding{},
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
		// 尝试轮询其他分片查找库存
		stockDeduced, err = s.tryDeductStockFromOtherShards(ctx, couponID)
		if err != nil {
			return false, fmt.Errorf("轮询其他分片扣减库存失败: %w", err)
		}
		if !stockDeduced {
			return false, errors.New("库存不足")
		}
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

	// 使用通道来接收异步操作的结果
	resultChan := make(chan error, 1)

	// 5. 提交订单创建任务到工作池
	s.workerPool.Submit(func() {
		// 执行数据库操作
		err := s.processOrderCreation(context.Background(), order, userID, couponID)
		resultChan <- err
	})

	// 等待异步操作完成并获取结果
	if err := <-resultChan; err != nil {
		// 插入失败回滚库存
		s.redisRepo.IncrStock(ctx, couponID)
		return false, err
	}

	// 8. 更新缓存
	s.redisRepo.SetUserReceivedCache(ctx, userID, couponID)

	// 更新用户优惠券数量缓存
	s.redisRepo.UpdateUserCouponCountCache(ctx, userID, 1, 1)

	// 更新优惠券库存缓存
	s.redisRepo.UpdateCouponStockCache(ctx, couponID, -1)

	return true, nil
}

// tryDeductStockFromOtherShards 尝试从其他分片扣减库存
func (s *SeckillService) tryDeductStockFromOtherShards(ctx context.Context, couponID int64) (bool, error) {
	// 遍历所有可能的分片（0-15）
	for i := 0; i < 16; i++ {
		virtualID := fmt.Sprintf("%d_%d", couponID, i)
		// 检查该分片是否有库存
		hasStock, err := s.mysqlRepo.CheckShardStock(ctx, couponID, virtualID)
		if err != nil {
			// 记录错误但继续尝试其他分片
			continue
		}

		if hasStock {
			// 如果该分片有库存，尝试扣减
			deducted, err := s.mysqlRepo.DeductShardStock(ctx, couponID, virtualID)
			if err != nil {
				// 扣减失败，继续尝试其他分片
				continue
			}

			if deducted {
				// 扣减成功，更新订单的VirtualID
				return true, nil
			}
		}
	}

	// 所有分片都没有库存
	return false, nil
}

// processOrderCreation 处理订单创建的完整流程
func (s *SeckillService) processOrderCreation(ctx context.Context, order *model.Order, userID, couponID int64) error {
	// 6. 插入订单（依赖联合索引uk_user_coupon_source防止重复）
	if err := s.mysqlRepo.CreateOrder(ctx, order); err != nil {
		return err
	}

	// 7. 更新用户优惠券数量统计
	if err := s.mysqlRepo.UpdateUserCouponCount(ctx, userID, 1, 1); err != nil {
		// 如果更新失败，需要回滚订单和库存
		s.mysqlRepo.DeleteOrder(ctx, order.ID, userID)
		return fmt.Errorf("更新用户优惠券数量失败: %w", err)
	}

	// 8. 更新优惠券库存（在数据库中同步更新）
	if err := s.mysqlRepo.UpdateCouponStock(ctx, couponID, -1); err != nil {
		// 如果更新失败，需要回滚之前的更改
		s.mysqlRepo.DeleteOrder(ctx, order.ID, userID)
		s.mysqlRepo.UpdateUserCouponCount(ctx, userID, -1, -1)
		return fmt.Errorf("更新优惠券库存失败: %w", err)
	}

	return nil
}
