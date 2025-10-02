# 正确的缩进
if True:
    print("这是缩进的代码块")
    if True:
        print("更深层的缩进")

# 错误的缩进
if True:
    print("这会导致语法错误")


# 输出
print("Hello, World!")

# 格式化输出
name = "Python"
age = 30
print(f"{name} 已经 {age} 岁了")

# 获取用户输入
user_input = input("请输入您的名字：")
print(f"您好，{user_input}!")

# 算术运算符
a = 10
b = 3
print(a + b)  # 13（加法）
print(a - b)  # 7（减法）
print(a * b)  # 30（乘法）
print(a / b)  # 3.3333333333333335（除法，返回浮点数）
print(a // b) # 3（整除，返回整数）
print(a % b)  # 1（取余）
print(a ** b) # 1000（幂运算）

# 比较运算符
print(a == b)  # False（等于）
print(a != b)  # True（不等于）
print(a > b)   # True（大于）
print(a < b)   # False（小于）
print(a >= b)  # True（大于等于）
print(a <= b)  # False（小于等于）

# 逻辑运算符
x = True
y = False
print(x and y)  # False（逻辑与）
print(x or y)   # True（逻辑或）
print(not x)    # False（逻辑非）

# 条件判断
score = 85

if score >= 90:
    print("优秀")
elif score >= 80:
    print("良好")
elif score >= 60:
    print("及格")
else:
    print("不及格")
# 遍历列表,循环语法
fruits = ["苹果", "香蕉", "橙子"]
for fruit in fruits:
    print(f"我喜欢吃{fruit}")

# 使用range()函数
for i in range(5):  # 生成 0, 1, 2, 3, 4
    print(i)

# 使用while循环
count = 0
while count < 5:
    print(f"当前计数: {count}")
    count += 1

# 使用try...except...finally处理异常
try:
    x = int(input("请输入一个数字: "))
    y = int(input("请输入另一个数字: "))
    result = x / y
    print(f"结果是: {result}")
except ValueError:
    print("请输入数字")
except ZeroDivisionError:
    print("除数不能为零")
finally:
    print("无论是否发生异常，都会执行的代码")
    print("程序结束")

# break 示例
for i in range(10):
    if i == 5:
        break
    print(i)

print("---")

# continue 示例
for i in range(10):
    if i % 2 == 0:
        continue
    print(i)

# 定义一个简单的函数
def greet(name):
    """这是函数的文档字符串，用于解释函数的功能"""
    return f"你好，{name}！"

# 调用函数
message = greet("小明")
print(message)

# 带有默认参数的函数
def power(x, n=2):
    return x ** n

print(power(2))     # 默认计算平方：4
print(power(2, 3))  # 计算立方：8

# 创建列表
fruits = ["苹果", "香蕉", "橙子"]

# 访问列表元素
print(fruits[0])  # 苹果（第一个元素）
print(fruits[-1])  # 橙子（最后一个元素）

# 修改列表元素
fruits[1] = "葡萄"
print(fruits)  # ['苹果', '葡萄', '橙子']

# 添加元素
fruits.append("草莓")
print(fruits)  # ['苹果', '葡萄', '橙子', '草莓']

# 插入元素
fruits.insert(1, "梨子")
print(fruits)  # ['苹果', '梨子', '葡萄', '橙子', '草莓']

# 删除元素
fruits.remove("葡萄")
print(fruits)  # ['苹果', '梨子', '橙子', '草莓']

# 获取列表长度
print(len(fruits))  # 4

# 列表切片
print(fruits[1:3])  # ['梨子', '橙子']

print("-------------------")
# 创建字典
student = {
    "name": "小明",
    "age": 18,
    "grade": "高三",
    "subjects": ["数学", "物理", "化学"]
}

# 访问字典元素
print(student["name"])  # 小明

# 修改字典元素
student["age"] = 19
print(student["age"])  # 19

# 添加新键值对
student["school"] = "第一中学"
print(student)

# 删除键值对
del student["subjects"]
print(student)

# 检查键是否存在
print("grade" in student)  # True
print("subjects" in student)  # False

# 获取所有键和值
print(student.keys())
print(student.values())
print(student.items())
print("-------------------")
# 创建元组
coordinates = (10, 20, 30)

# 访问元组元素
print(coordinates[0])  # 10

# 尝试修改元组会引发错误
# coordinates[0] = 15  # TypeError

# 元组解包
x, y, z = coordinates
print(x, y, z)  # 10 20 30

# 单元素元组需要逗号
single_item = (42,)
print(type(single_item))  # <class 'tuple'>
print("-------------------")
# 创建集合
unique_numbers = {1, 2, 3, 4, 5}
print(unique_numbers)

# 添加元素
unique_numbers.add(6)
print(unique_numbers)

# 删除元素
unique_numbers.remove(3)
print(unique_numbers)

# 集合操作
set1 = {1, 2, 3, 4, 5}
set2 = {4, 5, 6, 7, 8}

# 并集
print(set1 | set2)  # {1, 2, 3, 4, 5, 6, 7, 8}

# 交集
print(set1 & set2)  # {4, 5}

# 差集
print(set1 - set2)  # {1, 2, 3}
print("--------------------")
