// 在 util/errors.go 或 util/util.go 中添加
package util

import "errors"

var ErrDuplicateOrder = errors.New("duplicate order exists")
