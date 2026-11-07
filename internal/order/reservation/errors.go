package reservation

import (
	"fmt"
	"net/http"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

// ErrNotFound 表示预约不存在或不属于调用者（对外同一个 404，避免泄露他人预约的存在）。
var ErrNotFound = apperr.New(apperr.CodeInvalidRequest, http.StatusNotFound, "预约不存在")

// ErrDuplicate 表示该用户对该券已有进行中的预约（PENDING/FIRING/QUEUED），409。
var ErrDuplicate = apperr.RepeatSeckill.WithMessage("您已预约过该券，请勿重复预约")

// errNotCancellable 表示预约不在 PENDING 状态，不能取消，409。
func errNotCancellable(current Status) *apperr.Error {
	return apperr.New(apperr.CodeInvalidRequest, http.StatusConflict,
		fmt.Sprintf("仅待开抢(PENDING)状态可取消，当前=%s", current))
}
