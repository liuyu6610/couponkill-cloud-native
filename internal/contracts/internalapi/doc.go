// Package internalapi 定义服务间内部 HTTP API（设计 §10）的路径、请求/响应类型，
// 以及只依赖标准库 net/http 的类型化客户端。
//
// # 端点与类型
//
//	coupon  POST /internal/v1/stock/deduct    DeductRequest  -> DeductResponse   幂等（按 requestId）
//	coupon  POST /internal/v1/stock/release   ReleaseRequest -> ReleaseResponse  幂等（按 requestId）
//	coupon  POST /internal/v1/stock/sync      SyncRequest    -> SyncResponse     按 mode 调整（缺省 min=只降不升，set=强制设置），客户端不重试
//	coupon  GET  /internal/v1/coupons/{id}                   -> CouponInfo       只读
//	order   POST /internal/v1/orders/grant    GrantRequest   -> GrantResponse    幂等（按 (userId, requestId)）
//	user    GET  /internal/v1/users/{id}                     -> UserInfo         只读
//
// 服务端用 Path*Route 常量（gin 的 ":id" 语法）注册路由；响应放在平台信封
// {"code":200,"message":"ok","data":...} 的 data 里，失败按 apperr 返回信封与真实 HTTP 状态码。
// 所有请求必须带 X-Internal-Token；X-Request-Id 由客户端从 context 传播。
//
// # 约定（coupon/order 实现者共同遵守）
//
// ID 一律是 Go 的 int64、JSON 里的字符串（字段标签 `json:",string"`），与 events/keys 一致。
// 计数、桶号、可用量是普通 JSON 数字。未知响应字段被忽略（向前兼容）。
// 每个请求类型都有 Validate()，失败返回 apperr.InvalidRequest（HTTP 400）；
// 客户端发请求前会先校验，服务端 handler 绑定后也应调用同一个方法。
//
// 规格没有写明、由本包补充的字段形状：
//
//	DeductResponse  {couponId, bucket:int, alreadyDeducted:bool}
//	ReleaseRequest  {requestId, couponId}；ReleaseResponse {couponId, released, alreadyReleased}
//	SyncRequest     {couponId, available:int>=0, source, mode?:"min"|"set"}；SyncResponse {couponId, available=实际生效值}
//	GrantRequest    {requestId, userId, couponId}；GrantResponse {orderId, alreadyGranted}
//	CouponInfo      {id,name,type,status,faceValue,minSpend,validDays,perUserLimit,seckillStartAt,seckillEndAt}
//	UserInfo        {id,username,role,status}
//
// # 服务端语义要求
//
// stock/deduct：同一 requestId 重放不得再扣，返回第一次的桶并置 alreadyDeducted=true。
// 失败信封：全部桶售罄 409/OutOfStock；券不存在 404/CouponNotFound；券停用 409/CouponDisabled。
// 对已「归还」的 requestId 再 deduct 应被拒绝（409/OutOfStock），这样 sweeper 回滚之后
// 迟到的消费者不会把库存再扣走。
//
// stock/release：同一 requestId 重放幂等。若该 requestId 从未扣减，建议写入一条已归还的墓碑
// （响应里 released 与 alreadyReleased 都为 false），让之后迟到的 deduct 被上面的规则拒绝。
// 库存计数键 ck:stock:* 的回补由调用方（order）负责，coupon 的 release 只处理 DB 桶。
//
// orders/grant：不触碰库存。管理员发券的调用方（coupon）应先用同一个 requestId 调 stock/deduct，
// 建单失败再 stock/release。用户已持有该券返回 409/RepeatSeckill。
//
// # 客户端行为
//
// 构造：NewCouponClient/NewOrderClient/NewUserClient(baseURL, token, *http.Client, ...Option)；
// 传 nil 的 *http.Client 使用 DefaultHTTPClient()（单次尝试 3s 超时、64 个空闲连接）。
// 选项：WithRetryPolicy、WithRequestIDSource（接 logx 的请求 ID 读取）、WithLogger。
// 不依赖 platform-core；请求 ID 默认来自本包的 WithRequestID(ctx, id)。
//
// 错误映射（apperr 一律保持原业务码与 HTTP 状态码）：
//
//	下游 4xx 信封（409 OutOfStock、404 CouponNotFound ...） 原样转成 *apperr.Error，永不重试
//	下游 5xx 信封                                           保留原码/状态码，加上 ErrTransient，可重试
//	下游 5xx 无信封（HTML、空体）                            coupon 客户端为 CouponServiceUnavailable，
//	                                                        order/user 客户端为 SystemBusy，加上 ErrTransient
//	网络错误、单次尝试超时、连接被断开                        SystemBusy + ErrTransient
//	下游返回终端用户鉴权类码（20001/20002/20005/20006）      降级为 SysError，不透传（内部令牌配错不应把用户登出）
//	无法识别的 4xx、成功信封却没有/不合法的 data              SysError，不重试
//	调用方 ctx 取消或超时                                    返回包装了 ctx.Err() 的错误，不是 ErrTransient
//
// ErrTransient 包装 apperr.SystemBusy：errors.Is(err, ErrTransient) 判断「可稍后重试」
// （IsTransient 是同义便捷函数），errors.Is(err, apperr.SystemBusy) 也成立。
// 最外层始终是 *apperr.Error，可直接交给 httpx 渲染。
//
// 重试：只对幂等调用（DeductStock、ReleaseStock、GrantOrder、GetCoupon、GetUser）重试 transient
// 失败，默认最多 3 次尝试，退避 50ms 起指数增长、上限 500ms，带抖动（[基准/2, 基准]）；
// SyncStock 不重试；ctx 取消会立刻打断退避等待并停止。
// 重试耗尽后返回的错误满足 IsTransient，消费者应让 Kafka 重试同一条消息（同 requestId 安全）。
//
// # 测试替身
//
// 使用方应依赖本包的小接口 StockDeductor、StockReleaser、StockSyncer、CouponGetter、
// OrderGranter、UserGetter，而不是具体客户端类型，测试里用假实现替换。
package internalapi
