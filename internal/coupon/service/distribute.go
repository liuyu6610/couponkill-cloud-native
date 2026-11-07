package service

// Distribute 把 total 件库存拆到 n 个桶：每桶 total/n，余数依次多分给编号最小的桶。
// 结果之和恒等于 total、各桶之差不超过 1。n<=0 或 total<0 返回 nil（调用方应先校验）。
func Distribute(total int64, n int) []int64 {
	if n <= 0 || total < 0 {
		return nil
	}
	base, rem := total/int64(n), total%int64(n)
	out := make([]int64, n)
	for i := range out {
		out[i] = base
		if int64(i) < rem {
			out[i]++
		}
	}
	return out
}
