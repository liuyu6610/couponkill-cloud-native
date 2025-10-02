import time


class Logger:
    log_file = "app.log"

    def __init__(self, name):
        self.name = name

    def log(self, message):
        with open(self.__class__.log_file, 'a') as f:
            f.write(f"[{time.strftime('%Y-%m-%d %H:%M:%S')}] {self.name}: {message}\n")

    @classmethod
    def set_log_file(cls, filename):
        cls.log_file = filename
        with open(cls.log_file, 'w') as f:
            f.write(f"Log file initialized at {time.strftime('%Y-%m-%d %H:%M:%S')}\n")
        return f"Log file changed to {cls.log_file}"


# 创建logger实例
user_logger = Logger("UserSystem")
payment_logger = Logger("PaymentSystem")

# 记录日志
user_logger.log("New user registered")
payment_logger.log("Payment processed")

# 更改日志文件
print(Logger.set_log_file("new_app.log"))  # 输出: Log file changed to new_app.log

# 继续记录日志，现在日志将写入新文件
user_logger.log("User logged in")
payment_logger.log("Refund processed")