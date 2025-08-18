package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"couponkill-go-service/internal/model"
	"couponkill-go-service/internal/repository"
	"couponkill-go-service/pkg/idgenerator"

	"github.com/google/uuid"
)

// SeckillService 秒杀服务
type SeckillService struct {
	mysqlRepo     *repository.MysqlRepository
	redisRepo     *repository.RedisRepository
	couponService *CouponService // 添加couponService字段
}

// CouponService 优惠券服务（对接Java的coupon-service）
type CouponService struct {
	javaCouponServiceURL string // Java服务的优惠券查询接口地址
}

// NewCouponService 初始化优惠券服务
func NewCouponService(javaURL string) *CouponService {
	return &CouponService{javaCouponServiceURL: javaURL}
}

// GetCouponByID 调用Java服务查询优惠券信息
func (s *CouponService) GetCouponByID(ctx context.Context, couponID int64) (*model.Coupon, error) {
	// 示例URL：http://coupon-service/coupon/{id}
	url := s.javaCouponServiceURL + "/coupon/" + fmt.Sprintf("%d", couponID) // 修复类型转换

	// 模拟HTTP请求（省略具体实现，实际需处理超时、重试等）
	resp, err := http.Get(url)
	if err != nil {
		return nil, errors.New("查询优惠券失败：" + err.Error())
	}
	defer resp.Body.Close()

	var coupon model.Coupon
	if err := json.NewDecoder(resp.Body).Decode(&coupon); err != nil {
		return nil, errors.New("解析优惠券数据失败：" + err.Error())
	}

	if coupon.ID == 0 {
		return nil, errors.New("优惠券不存在")
	}
	return &coupon, nil
}

// 修改构造函数，添加couponService参数
func NewSeckillService(mysqlRepo *repository.MysqlRepository, redisRepo *repository.RedisRepository, couponService *CouponService) *SeckillService {
	return &SeckillService{
		mysqlRepo:     mysqlRepo,
		redisRepo:     redisRepo,
		couponService: couponService, // 初始化couponService
	}
}

// ProcessSeckill 处理单个用户秒杀（Go端）
func (s *SeckillService) ProcessSeckill(ctx context.Context, userID, couponID int64, validDays int) (bool, error) {
	// 1. 查询优惠券信息（获取类型、状态等）
	coupon, err := s.couponService.GetCouponByID(ctx, couponID) // 修复：使用couponID而不是req.CouponID
	if err != nil {
		return false, err
	}

	// 2. 校验优惠券状态
	if coupon.Type != 1 && coupon.Type != 2 { // 假设有效的Type值为1或2
		return false, errors.New("优惠券类型无效")
	}
	// 3. 先查缓存：用户是否已领取
	cacheReceived, err := s.redisRepo.CheckUserReceivedCache(ctx, userID, couponID)
	if err != nil {
		return false, err
	}
	if cacheReceived {
		return false, errors.New("用户已领取")
	}

	// 4. 查DB确认：两端均未领取（双字段均为0）
	dbReceived, err := s.mysqlRepo.CheckUserReceived(ctx, userID, couponID)
	if err != nil {
		return false, err
	}
	if dbReceived {
		return false, errors.New("用户已领取")
	}

	// 5. 扣减库存（Redis原子操作）
	stockDeduced, err := s.redisRepo.DeductStock(ctx, couponID)
	if err != nil {
		return false, fmt.Errorf("扣减库存失败: %w", err)
	}
	if !stockDeduced {
		return false, errors.New("库存不足")
	}

	// 6. 创建订单（标记Go端来源）
	order := &model.Order{
		ID:            idgenerator.GenerateGoOrderID(),
		UserID:        userID,
		CouponID:      couponID,
		Status:        1, // 已创建
		GetTime:       time.Now(),
		ExpireTime:    time.Now().AddDate(0, 0, validDays),
		CreatedByJava: 0, // 非Java端创建
		CreatedByGo:   1, // 标记为Go端创建
		RequestID:     uuid.New().String(),
		Version:       0,
	}

	// 7. 插入订单（依赖联合索引uk_user_coupon_source防止重复）
	if err := s.mysqlRepo.CreateOrder(ctx, order); err != nil {
		// 插入失败回滚库存
		s.redisRepo.IncrStock(ctx, couponID) // 简化处理
		return false, err
	}

	// 8. 更新缓存
	s.redisRepo.SetUserReceivedCache(ctx, userID, couponID)
	return true, nil
}
