def quick_sort(arr):
    """
    使用快速排序算法对列表进行排序

    参数:
        arr (list): 需要排序的列表

    返回:
        list: 排序后的列表
    """
    # 基本情况：空列表或只有一个元素的列表已经是有序的
    if len(arr) <= 1:
        return arr

    # 选择基准元素（这里简单地选择第一个元素）
    pivot = arr[0]

    # 将列表分为小于基准、等于基准和大于基准的三部分
    less = [x for x in arr[1:] if x < pivot]  # 所有小于基准的元素
    equal = [x for x in arr if x == pivot]    # 所有等于基准的元素
    greater = [x for x in arr[1:] if x > pivot]  # 所有大于基准的元素

    # 递归地对小于和大于部分进行排序，然后组合结果
    return quick_sort(less) + equal + quick_sort(greater)

# 测试快速排序函数
unsorted_list = [3, 6, 8, 10, 1, 2, 1]
print(f"排序前: {unsorted_list}")
sorted_list = quick_sort(unsorted_list)
print(f"排序后: {sorted_list}")

def calculate_total(items, discount=0):
    """计算购物车中所有商品的总价，并应用折扣"""
    total = 0
    for item in items:
        # 累加每个商品的价格
        total += item['price'] * item['quantity']

    # 应用折扣
    discounted_total = total * (1 - discount)

    # 调试信息
    # print(f"原价总计: {total}")
    # print(f"折扣: {discount * 100}%")
    # print(f"折后价: {discounted_total}")

    return discounted_total

# 测试函数
cart = [
    {'name': '笔记本电脑', 'price': 5000, 'quantity': 1},
    {'name': '鼠标', 'price': 100, 'quantity': 2},
    {'name': '键盘', 'price': 200, 'quantity': 1}
]

print(f"购物车总价: {calculate_total(cart, 0.1)}")

import re

# 电子邮件验证正则表达式
email_pattern = re.compile(r"""
    ^                       # 字符串开始
    [a-zA-Z0-9._%+-]+       # 用户名部分：允许字母、数字和某些特殊字符
    @                       # @ 符号
    [a-zA-Z0-9.-]+          # 域名部分
    \.                      # 点号
    [a-zA-Z]{2,}            # 顶级域名：至少2个字母
    $                       # 字符串结束
""", re.VERBOSE)

# 测试电子邮件格式
emails = ["user@example.com", "invalid@email", "another.valid@email.org"]
for email in emails:
    if email_pattern.match(email):
        print(f"{email} 是有效的电子邮件地址")
    else:
        print(f"{email} 不是有效的电子邮件地址")