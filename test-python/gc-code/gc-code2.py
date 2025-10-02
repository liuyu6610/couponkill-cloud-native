import sys

# 创建一个对象
a = [1, 2, 3]
# 查看引用计数
print(sys.getrefcount(a))  # 输出: 2（1个实际引用 + 函数参数引用）

# 创建另一个引用
b = a
print(sys.getrefcount(a))  # 输出: 3

# 删除一个引用
del b
print(sys.getrefcount(a))  # 输出: 2