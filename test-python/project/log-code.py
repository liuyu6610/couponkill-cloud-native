def log_message(message, level="INFO", *tags, timestamp=True, **metadata):
    """
    记录日志消息

    参数:
    - message: 日志消息内容
    - level: 日志级别，默认为"INFO"
    - *tags: 可选的标签列表
    - timestamp: 是否包含时间戳，默认为True
    - **metadata: 额外的元数据
    """
    import datetime

    log_entry = {}

    # 添加时间戳
    if timestamp:
        log_entry["timestamp"] = datetime.datetime.now().strftime("%Y-%m-%d %H:%M:%S")

    # 添加消息和级别
    log_entry["message"] = message
    log_entry["level"] = level

    # 添加标签
    if tags:
        log_entry["tags"] = list(tags)

    # 添加元数据
    log_entry.update(metadata)

    # 在实际应用中，这里可能会将日志写入文件或发送到日志系统
    print(log_entry)

# 使用示例
log_message(
    "用户登录成功",
    "DEBUG",
    "security", "user-activity",
    user_id="12345",
    ip_address="192.168.1.1"
)
def build_config(app_name, version, environment="development", *features, db_config=None, **extra_settings):
    """
    创建应用程序配置

    参数:
    - app_name: 应用程序名称
    - version: 应用版本
    - environment: 运行环境，默认为"development"
    - *features: 启用的功能列表
    - db_config: 数据库配置信息
    - **extra_settings: 其他配置设置
    """
    # 基本配置
    config = {
        "app_name": app_name,
        "version": version,
        "environment": environment
    }

    # 添加功能标志
    if features:
        config["enabled_features"] = list(features)

    # 添加数据库配置
    if db_config:
        config["database"] = db_config

    # 添加额外设置
    if extra_settings:
        config["settings"] = extra_settings

    return config

# 使用示例
app_config = build_config(
    "MyWebApp",
    "1.0.0",
    "production",
    "user-auth", "payment-processing", "notifications",
    db_config={
        "host": "localhost",
        "port": 5432,
        "name": "myapp_db"
    },
    debug=False,
    cache_timeout=3600,
    max_upload_size="10MB"
)

print(app_config)
if __name__ == "__main__":
    print(log_message("这是一条测试日志"))
