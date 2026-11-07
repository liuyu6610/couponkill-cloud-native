// Package repo 是 connector 的 PostgreSQL 仓储（cnx schema）：SKU 绑定、价格映射与同步进度。
// 所有表名都带 cnx 前缀，不依赖连接的 search_path。
package repo

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/domain"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/idgen"
)

// IDSource 生成主键（snowflake，应用层生成）。
type IDSource interface{ Next() int64 }

// Store 是 cnx schema 的仓储，并发安全。
type Store struct {
	pool *pgxpool.Pool
	ids  IDSource
}

// NewStore 创建仓储。
func NewStore(pool *pgxpool.Pool, ids IDSource) *Store {
	return &Store{pool: pool, ids: ids}
}

const bindingCols = `id, platform, external_sku_id, coupon_id, sync_enabled, last_stock, last_sync_at,
	last_sync_status, last_error, created_at, updated_at`

const priceMapCols = `id, coupon_id, platform, external_sku_id, title, manual_price::text, currency, enabled,
	created_at, updated_at`

// 唯一约束名（见 0001_init.sql）。
const (
	ukBindingsCoupon      = "uk_bindings_coupon"
	ukBindingsPlatformSku = "uk_bindings_platform_sku"
)

// upsertAttempts 是 UpsertBinding 遇到并发唯一冲突时整体重试的次数上限。
const upsertAttempts = 2

func scanBinding(row pgx.Row) (domain.Binding, error) {
	var b domain.Binding
	var id, coupon int64
	var platform string
	err := row.Scan(&id, &platform, &b.ExternalSkuID, &coupon, &b.SyncEnabled, &b.LastStock, &b.LastSyncAt,
		&b.LastSyncStatus, &b.LastError, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return b, err
	}
	b.ID, b.CouponID, b.Platform = idgen.ID(id), idgen.ID(coupon), domain.Platform(platform)
	return b, nil
}

func scanPriceMap(row pgx.Row) (domain.PriceMap, error) {
	var p domain.PriceMap
	var id, coupon int64
	var platform string
	var price *string
	err := row.Scan(&id, &coupon, &platform, &p.ExternalSkuID, &p.Title, &price, &p.Currency, &p.Enabled,
		&p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return p, err
	}
	p.ID, p.CouponID, p.Platform = idgen.ID(id), idgen.ID(coupon), domain.Platform(platform)
	if price != nil {
		n := json.Number(*price)
		p.ManualPrice = &n
	}
	return p, nil
}

// collect 读完 rows 并用 scan 转换；调用方负责传入已 Query 的 rows。
func collect[T any](rows pgx.Rows, scan func(pgx.Row) (T, error)) ([]T, error) {
	defer rows.Close()
	out := []T{}
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// UpsertBinding 创建或更新绑定：同一 (platform, sku) 已存在则原地更新券与启用开关，否则新建（状态 NEW）。
// 一张券至多一条绑定：券已被另一个 SKU 占用返回 ErrBindingConflict。
// 并发的相同请求是幂等的：撞唯一约束时整体重试，第二轮会走更新分支。
func (s *Store) UpsertBinding(ctx context.Context, cmd domain.BindingCommand, enabled bool) (domain.Binding, error) {
	var out domain.Binding
	var err error
	for attempt := 0; attempt < upsertAttempts; attempt++ {
		out, err = s.upsertBindingOnce(ctx, cmd, enabled)
		if !dbx.IsUniqueViolation(err, "") {
			break
		}
	}
	if dbx.IsUniqueViolation(err, ukBindingsCoupon) || dbx.IsUniqueViolation(err, ukBindingsPlatformSku) {
		return domain.Binding{}, conflictError(cmd, err)
	}
	if domain.IsBindingConflict(err) {
		return domain.Binding{}, err
	}
	if err != nil {
		return domain.Binding{}, fmt.Errorf("upsert binding: %w", err)
	}
	return out, nil
}

func conflictError(cmd domain.BindingCommand, cause error) error {
	return domain.ErrBindingConflict.WithMessage(
		fmt.Sprintf("couponId=%d 已绑定其它平台 SKU，或 %s/%s 已被占用，请先停用或改绑同一条记录", cmd.CouponID, cmd.Platform, cmd.ExternalSkuID)).
		WithCause(cause)
}

func (s *Store) upsertBindingOnce(ctx context.Context, cmd domain.BindingCommand, enabled bool) (domain.Binding, error) {
	var out domain.Binding
	err := dbx.WithTx(ctx, s.pool, dbx.TxOptions{}, func(tx pgx.Tx) error {
		cur, err := scanBinding(tx.QueryRow(ctx, `SELECT `+bindingCols+` FROM cnx.platform_sku_bindings WHERE coupon_id = $1`, int64(cmd.CouponID)))
		switch {
		case err == nil:
			if cur.Platform != cmd.Platform || cur.ExternalSkuID != cmd.ExternalSkuID {
				return domain.ErrBindingConflict.WithMessage(fmt.Sprintf(
					"couponId=%d 已绑定 %s/%s，请先停用或改绑同一条记录", cmd.CouponID, cur.Platform, cur.ExternalSkuID))
			}
		case !dbx.IsNoRows(err):
			return err
		}
		out, err = scanBinding(tx.QueryRow(ctx, `UPDATE cnx.platform_sku_bindings
			SET coupon_id = $3, sync_enabled = $4, updated_at = now()
			WHERE platform = $1 AND external_sku_id = $2
			RETURNING `+bindingCols, string(cmd.Platform), cmd.ExternalSkuID, int64(cmd.CouponID), enabled))
		if err == nil {
			return nil
		}
		if !dbx.IsNoRows(err) {
			return err
		}
		out, err = scanBinding(tx.QueryRow(ctx, `INSERT INTO cnx.platform_sku_bindings
			(id, platform, external_sku_id, coupon_id, sync_enabled, last_sync_status)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING `+bindingCols, s.ids.Next(), string(cmd.Platform), cmd.ExternalSkuID, int64(cmd.CouponID), enabled, domain.SyncNew))
		return err
	})
	return out, err
}

// GetBinding 按 id 取绑定；不存在返回 ErrBindingNotFound。
func (s *Store) GetBinding(ctx context.Context, id int64) (domain.Binding, error) {
	b, err := scanBinding(s.pool.QueryRow(ctx, `SELECT `+bindingCols+` FROM cnx.platform_sku_bindings WHERE id = $1`, id))
	if dbx.IsNoRows(err) {
		return domain.Binding{}, domain.ErrBindingNotFound.WithMessage(fmt.Sprintf("绑定不存在: %d", id))
	}
	if err != nil {
		return domain.Binding{}, fmt.Errorf("get binding %d: %w", id, err)
	}
	return b, nil
}

// FindBindingByCoupon 按券取绑定；没有绑定返回 (nil, nil)。
func (s *Store) FindBindingByCoupon(ctx context.Context, couponID int64) (*domain.Binding, error) {
	b, err := scanBinding(s.pool.QueryRow(ctx, `SELECT `+bindingCols+` FROM cnx.platform_sku_bindings WHERE coupon_id = $1`, couponID))
	if dbx.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find binding by coupon %d: %w", couponID, err)
	}
	return &b, nil
}

// ListBindings 返回全部绑定，新建的在前（id 降序）。
func (s *Store) ListBindings(ctx context.Context) ([]domain.Binding, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+bindingCols+` FROM cnx.platform_sku_bindings ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("list bindings: %w", err)
	}
	out, err := collect(rows, scanBinding)
	if err != nil {
		return nil, fmt.Errorf("list bindings: %w", err)
	}
	return out, nil
}

// ListSyncCandidates 返回启用同步的绑定，“最久未同步优先”：从未同步的最先，其后按 last_sync_at 升序，
// 同时间按 id。顺序由部分索引 idx_bindings_sync_rotation 直接提供。
func (s *Store) ListSyncCandidates(ctx context.Context) ([]domain.Binding, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+bindingCols+` FROM cnx.platform_sku_bindings
		WHERE sync_enabled ORDER BY last_sync_at NULLS FIRST, id`)
	if err != nil {
		return nil, fmt.Errorf("list sync candidates: %w", err)
	}
	out, err := collect(rows, scanBinding)
	if err != nil {
		return nil, fmt.Errorf("list sync candidates: %w", err)
	}
	return out, nil
}

// RecordSync 写回一次同步尝试的结果并盖上 last_sync_at。SUCCESS、FAIL、SKIP 一视同仁地刷新时间，
// 否则一直失败的绑定会永远排在最前面把预算吃光。LastStock 为 nil 时保持原值。
// 绑定不存在返回 ErrBindingNotFound。
func (s *Store) RecordSync(ctx context.Context, id int64, o domain.SyncOutcome) (domain.Binding, error) {
	b, err := scanBinding(s.pool.QueryRow(ctx, `UPDATE cnx.platform_sku_bindings SET
			last_stock = COALESCE($2, last_stock),
			last_sync_at = $3,
			last_sync_status = $4,
			last_error = $5,
			updated_at = now()
		WHERE id = $1
		RETURNING `+bindingCols, id, o.LastStock, o.At, o.Status, o.Error))
	if dbx.IsNoRows(err) {
		return domain.Binding{}, domain.ErrBindingNotFound.WithMessage(fmt.Sprintf("绑定不存在: %d", id))
	}
	if err != nil {
		return domain.Binding{}, fmt.Errorf("record sync for binding %d: %w", id, err)
	}
	return b, nil
}

// UpsertPriceMap 按 (coupon_id, platform) 创建或更新价格映射（一条 SQL，天然并发安全）。
// currency 与 enabled 由调用方解析好缺省值。
func (s *Store) UpsertPriceMap(ctx context.Context, cmd domain.PriceMapCommand, currency string, enabled bool) (domain.PriceMap, error) {
	var price *string
	if cmd.ManualPrice != nil {
		p := cmd.ManualPrice.String()
		price = &p
	}
	p, err := scanPriceMap(s.pool.QueryRow(ctx, `INSERT INTO cnx.price_maps
			(id, coupon_id, platform, external_sku_id, title, manual_price, currency, enabled)
		VALUES ($1, $2, $3, $4, $5, $6::numeric, $7, $8)
		ON CONFLICT (coupon_id, platform) DO UPDATE SET
			external_sku_id = EXCLUDED.external_sku_id,
			title = EXCLUDED.title,
			manual_price = EXCLUDED.manual_price,
			currency = EXCLUDED.currency,
			enabled = EXCLUDED.enabled,
			updated_at = now()
		RETURNING `+priceMapCols,
		s.ids.Next(), int64(cmd.CouponID), string(cmd.Platform), cmd.ExternalSkuID, cmd.Title, price, currency, enabled))
	if err != nil {
		return domain.PriceMap{}, fmt.Errorf("upsert price map: %w", err)
	}
	return p, nil
}

// ListPriceMaps 返回券的价格映射（id 升序）；enabledOnly 时只返回启用的。
func (s *Store) ListPriceMaps(ctx context.Context, couponID int64, enabledOnly bool) ([]domain.PriceMap, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+priceMapCols+` FROM cnx.price_maps
		WHERE coupon_id = $1 AND (NOT $2 OR enabled) ORDER BY id`, couponID, enabledOnly)
	if err != nil {
		return nil, fmt.Errorf("list price maps: %w", err)
	}
	out, err := collect(rows, scanPriceMap)
	if err != nil {
		return nil, fmt.Errorf("list price maps: %w", err)
	}
	return out, nil
}

// DeletePriceMap 删除价格映射；不存在也视为成功（幂等删除）。
func (s *Store) DeletePriceMap(ctx context.Context, id int64) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM cnx.price_maps WHERE id = $1`, id); err != nil {
		return fmt.Errorf("delete price map %d: %w", id, err)
	}
	return nil
}
