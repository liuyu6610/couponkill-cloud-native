# mathtools/basic/operations.py
"""提供基本的数学运算"""

class Calculator:
    """一个简单的计算器类"""
    
    def __init__(self):
        self.history = []
    
    def add(self, a, b):
        """返回两个数的和"""
        result = a + b
        self.history.append(f"{a} + {b} = {result}")
        return result
    
    def subtract(self, a, b):
        """返回两个数的差"""
        result = a - b
        self.history.append(f"{a} - {b} = {result}")
        return result
    
    def multiply(self, a, b):
        """返回两个数的乘积"""
        result = a * b
        self.history.append(f"{a} * {b} = {result}")
        return result
    
    def divide(self, a, b):
        """返回两个数的商"""
        if b == 0:
            raise ZeroDivisionError("除数不能为零")
        result = a / b
        self.history.append(f"{a} / {b} = {result}")
        return result
    
    def get_history(self):
        """返回计算历史"""
        return self.history.copy()

def add(a, b):
    """返回两个数的和"""
    return a + b

def subtract(a, b):
    """返回两个数的差"""
    return a - b

def multiply(a, b):
    """返回两个数的乘积"""
    return a * b

def divide(a, b):
    """返回两个数的商"""
    if b == 0:
        raise ZeroDivisionError("除数不能为零")
    return a / b