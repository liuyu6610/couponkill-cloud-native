class DatabaseConnection:
    _connections = {}

    @classmethod
    def get_connection(cls, connection_string):
        if connection_string not in cls._connections:
            # 创建新连接
            print(f"创建新连接: {connection_string}")
            cls._connections[connection_string] = {"connection": connection_string, "users": 0}

        # 增加使用计数
        cls._connections[connection_string]["users"] += 1
        return ConnectionProxy(connection_string)

    @classmethod
    def release_connection(cls, connection_string):
        if connection_string in cls._connections:
            cls._connections[connection_string]["users"] -= 1
            print(f"连接 {connection_string} 的引用计数: {cls._connections[connection_string]['users']}")

            # 如果没有人使用此连接，关闭它
            if cls._connections[connection_string]["users"] <= 0:
                print(f"关闭连接: {connection_string}")
                del cls._connections[connection_string]


class ConnectionProxy:
    def __init__(self, connection_string):
        self.connection_string = connection_string

    def __del__(self):
        # 当对象被销毁时释放连接
        DatabaseConnection.release_connection(self.connection_string)


# 使用示例
def do_work():
    # 获取连接
    conn = DatabaseConnection.get_connection("mysql://localhost/db")
    # 使用连接...
    print("正在使用连接...")
    # 连接在函数结束时自动释放（当conn超出作用域）


print("开始第一个任务")
do_work()

print("\n开始第二个任务")
do_work()

print("\n开始第三个任务")
conn = DatabaseConnection.get_connection("mysql://localhost/db")
# 手动删除引用
del conn