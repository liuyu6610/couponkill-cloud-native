package reservation

// Status 是预约帮抢的状态，取值与 ord.reservations.status 的 CHECK 约束、前端 ReservationStatus 一致。
type Status string

// 预约状态词表（大小写敏感）。
const (
	// StatusPending 待开抢：已预约，等到 trigger_at 由调度器触发。
	StatusPending Status = "PENDING"
	// StatusFiring 抢购中：调度器已认领，正在调用秒杀入口。
	StatusFiring Status = "FIRING"
	// StatusQueued 已入队：秒杀入口受理，等待建单结果（request_id 已记录）。
	StatusQueued Status = "QUEUED"
	// StatusSuccess 已抢到：订单已生成（终态）。
	StatusSuccess Status = "SUCCESS"
	// StatusFailed 失败：库存不足、重复、重试耗尽等（终态，fail_code/fail_reason 说明原因）。
	StatusFailed Status = "FAILED"
	// StatusCancelled 已取消：用户在 PENDING 时取消（终态）。
	StatusCancelled Status = "CANCELLED"
	// StatusExpired 已过期：活动结束仍未能触发（终态）。
	StatusExpired Status = "EXPIRED"
)

// allStatuses 按生命周期顺序列出全部状态。
var allStatuses = []Status{
	StatusPending, StatusFiring, StatusQueued, StatusSuccess, StatusFailed, StatusCancelled, StatusExpired,
}

// transitions 是状态机的唯一定义：键是当前状态，值是允许迁往的状态。
// 所有写状态的 SQL 都由它推导出 WHERE status = ANY(合法来源)，所以代码里不存在绕过它的路径。
//
//	PENDING → FIRING     调度器认领（到点）
//	PENDING → CANCELLED  用户取消
//	FIRING  → QUEUED     秒杀入口受理，记录 request_id
//	FIRING  → FAILED     入口返回终态错误 / 重试耗尽 / 崩溃回收时已无重试额度
//	FIRING  → PENDING    瞬态失败或崩溃回收：退避后重试（retry_count+1）
//	FIRING  → EXPIRED    触发时发现活动已结束
//	FIRING  → SUCCESS    结果事件先于 QUEUED 回写到达
//	QUEUED  → SUCCESS    建单成功
//	QUEUED  → FAILED     建单失败
var transitions = map[Status][]Status{
	StatusPending: {StatusFiring, StatusCancelled},
	StatusFiring:  {StatusQueued, StatusFailed, StatusPending, StatusExpired, StatusSuccess},
	StatusQueued:  {StatusSuccess, StatusFailed},
}

// Valid 判断是否为词表内的状态。
func (s Status) Valid() bool {
	for _, v := range allStatuses {
		if s == v {
			return true
		}
	}
	return false
}

// Terminal 判断是否为终态（没有任何出边）。
func (s Status) Terminal() bool { return s.Valid() && len(transitions[s]) == 0 }

// Active 判断是否为“进行中”状态（PENDING/FIRING/QUEUED），与部分唯一索引
// uk_reservations_active_user_coupon 的谓词一致：同一用户同一张券至多一个。
func (s Status) Active() bool { return s.Valid() && !s.Terminal() }

// CanTransition 判断 from → to 是否为合法迁移。未知状态一律非法。
func CanTransition(from, to Status) bool {
	for _, v := range transitions[from] {
		if v == to {
			return true
		}
	}
	return false
}

// sourcesOf 返回能迁往 to 的全部来源状态（用于生成 UPDATE 的 WHERE status = ANY(...)）。
func sourcesOf(to Status) []string {
	var out []string
	for _, from := range allStatuses {
		if CanTransition(from, to) {
			out = append(out, string(from))
		}
	}
	return out
}
