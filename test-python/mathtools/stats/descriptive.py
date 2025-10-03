# mathtools/stats/descriptive.py
"""提供描述性统计函数"""


def mean(numbers):
    """计算平均值"""
    if not numbers:
        raise ValueError("输入列表不能为空")
    return sum(numbers) / len(numbers)


def median(numbers):
    """计算中位数"""
    if not numbers:
        raise ValueError("输入列表不能为空")

    sorted_numbers = sorted(numbers)
    n = len(sorted_numbers)

    if n % 2 == 0:
        # 偶数个元素，取中间两个的平均值
        return (sorted_numbers[n // 2 - 1] + sorted_numbers[n // 2]) / 2
    else:
        # 奇数个元素，取中间的一个
        return sorted_numbers[n // 2]