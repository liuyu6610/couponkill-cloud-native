def merge_sort(arr):
    """归并排序"""
    if len(arr) <= 1:
        return arr

    # 分解
    mid = len(arr) // 2
    left = merge_sort(arr[:mid])
    right = merge_sort(arr[mid:])

    # 合并
    return merge(left, right)


def merge(left, right):
    """合并两个已排序的数组"""
    result = []
    i = j = 0

    while i < len(left) and j < len(right):
        if left[i] < right[j]:
            result.append(left[i])
            i += 1
        else:
            result.append(right[j])
            j += 1

    result.extend(left[i:])
    result.extend(right[j:])
    return result


# 测试归并排序
arr = [38, 27, 43, 3, 9, 82, 10]
sorted_arr = merge_sort(arr)
print("排序结果:", sorted_arr)  # 输出: [3, 9, 10, 27, 38, 43, 82]

# 未优化的斐波那契数列计算
def fib(n):
    if n <= 1:
        return n
    return fib(n-1) + fib(n-2)

# 使用记忆化的斐波那契数列计算
def fib_memo(n, memo={}):
    if n in memo:
        return memo[n]
    if n <= 1:
        return n
    memo[n] = fib_memo(n-1, memo) + fib_memo(n-2, memo)
    return memo[n]

# 使用Python内置的functools.lru_cache进行记忆化
from functools import lru_cache

@lru_cache(maxsize=None)
def fib_cached(n):
    if n <= 1:
        return n
    return fib_cached(n-1) + fib_cached(n-2)

# 比较性能
import time

n = 35

start = time.time()
print(f"fib({n}) = {fib(n)}")
print(f"未优化用时: {time.time() - start:.2f}秒")

start = time.time()
print(f"fib_memo({n}) = {fib_memo(n)}")
print(f"记忆化优化用时: {time.time() - start:.2f}秒")

start = time.time()
print(f"fib_cached({n}) = {fib_cached(n)}")
print(f"lru_cache优化用时: {time.time() - start:.2f}秒")