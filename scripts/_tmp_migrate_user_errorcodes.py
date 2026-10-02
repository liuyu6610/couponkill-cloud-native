# -*- coding: utf-8 -*-
from pathlib import Path

p = Path(
    "couponkill-user-service/src/main/java/com/aliyun/seckill/"
    "couponkilluserservice/service/Impl/UserServiceImpl.java"
)
c = p.read_text(encoding="utf-8")
c = c.replace(
    "import com.aliyun.seckill.common.enums.ResultCode;\n",
    "import com.aliyun.seckill.common.api.ErrorCodes;\n",
)
repls = [
    (
        "new BusinessException(ResultCode.USER_EXIST)",
        'new BusinessException(ErrorCodes.USER_EXIST, "用户已存在")',
    ),
    (
        "new BusinessException(ResultCode.USER_NOT_FOUND)",
        'new BusinessException(ErrorCodes.USER_NOT_FOUND, "用户不存在")',
    ),
    (
        "new BusinessException(ResultCode.PASSWORD_ERROR)",
        'new BusinessException(ErrorCodes.PASSWORD_ERROR, "密码错误")',
    ),
    (
        "new BusinessException(ResultCode.SYSTEM_BUSY.getCode(), ",
        "new BusinessException(ErrorCodes.SYSTEM_BUSY, ",
    ),
    (
        "new BusinessException(ResultCode.SYSTEM_ERROR.getCode(), ",
        "new BusinessException(ErrorCodes.SYS_ERROR, ",
    ),
]
for old, new in repls:
    c = c.replace(old, new)
if "ResultCode" in c:
    idx = c.index("ResultCode")
    raise SystemExit(f"leftover: {c[idx - 40 : idx + 40]!r}")
p.write_text(c, encoding="utf-8")
print("user ResultCode cleared, ErrorCodes refs:", c.count("ErrorCodes."))
