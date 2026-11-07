package repo

// SetCrashHook 注入「桶已更新、事务未提交」时的故障（测试专用）；传 nil 清除。
func (r *Repo) SetCrashHook(f func() error) { r.afterBucketUpdate = f }

// StartBucket 暴露起始桶计算给外部测试包。
func StartBucket(userID int64, buckets int) int { return startBucket(userID, buckets) }
