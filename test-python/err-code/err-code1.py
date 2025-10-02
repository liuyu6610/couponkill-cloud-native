try:
    filename = input("请输入要打开的文件名: ")
    with open(filename , 'r') as f:
        content = f.read()
        print(content)
except FileNotFoundError:
    print(f"错误：文件 '{filename}' 不存在")
except PermissionError:
    print(f"错误：没有权限读取文件 '{filename}'")
except Exception as e:
    print(f"发生了其他错误: {type(e).__name__}, {e}")

import requests

try:
    response = requests.get('https://api.example.com/data' , timeout=5)
    response.raise_for_status()  # 如果HTTP请求返回了不成功的状态码，会引发HTTPError异常
    data = response.json()
    print(f"成功获取数据: {data}")
except requests.exceptions.HTTPError as e:
    print(f"HTTP错误: {e}")
except requests.exceptions.ConnectionError:
    print("连接错误: 无法连接到服务器")
except requests.exceptions.Timeout:
    print("超时错误: 请求超时")
except requests.exceptions.JSONDecodeError:
    print("解析错误: 返回的不是有效的JSON数据")
except Exception as e:
    print(f"发生了其他错误: {e}")


class InsufficientFundsError(Exception):
    """当尝试取款金额超过账户余额时引发"""

    def __init__(self, balance, amount):
        self.balance = balance
        self.amount = amount
        self.deficit = amount - balance
        message = f"余额不足。当前余额: {balance}，尝试取款: {amount}，缺少: {self.deficit}"
        super().__init__(message)


class BankAccount:
    def __init__(self, name, balance=0):
        self.name = name
        self.balance = balance

    def deposit(self, amount):
        if amount <= 0:
            raise ValueError("存款金额必须为正数")
        self.balance += amount
        return self.balance

    def withdraw(self, amount):
        if amount <= 0:
            raise ValueError("取款金额必须为正数")
        if amount > self.balance:
            raise InsufficientFundsError(self.balance, amount)
        self.balance -= amount
        return self.balance


# 使用示例
try:
    account = BankAccount("张三", 100)
    account.withdraw(150)
except InsufficientFundsError as e:
    print(f"错误: {e}")
    print(f"尝试再存入 {e.deficit} 元后再取款")