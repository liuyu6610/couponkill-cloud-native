package service

import (
	"context"
	"couponkill-go-service/model"
	"couponkill-go-service/repositorys"
	"couponkill-go-service/util"
	"time"
)

type SeckillService struct {
	dbRepo    *repositorys.DBRepository
	redisRepo *repositorys.RedisRepository
}

func NewSeckillService(dbRepo *repository.DBRepository, redisRepo *repository.RedisRepository) *SeckillService {
	return &SeckillService{
		dbRepo:    dbRepo,
		redisRepo: redisRepo,
	}
}

// CreateSeckillOrder 创建秒杀订单
func (s *SeckillService) CreateSeckillOrder(ctx context.Context, userId, couponId int64, validDays int) (*model.Order, error) {
	// 1. 生成订单ID
	orderId := util.GenerateOrderID()

	// 2. 构建订单信息
	now := time.Now()
	order := &model.Order{
		ID:         orderId,
		UserId:     userId,
		CouponId:   couponId,
		Status:     1, // 已创建
		GetTime:    now,
		ExpireTime: now.AddDate(0, 0, validDays),
		CreateTime: now,
		UpdateTime: now,
	}

	// 3. 保存订单到数据库
	if err := s.dbRepo.CreateOrder(ctx, order); err != nil {
		return nil, err
	}

	// 4. 更新Redis缓存
	if err := s.redisRepo.RecordUserReceived(ctx, userId, couponId); err != nil {
		// 日志记录缓存更新失败，但不影响主流程
		util.Log.Errorf("记录用户领取状态失败: %v", err)
	}

	return order, nil
}

func (s *SeckillService) GetCouponValidDays(ctx context.Context, id int64) (interface{}, interface{}) {

}
