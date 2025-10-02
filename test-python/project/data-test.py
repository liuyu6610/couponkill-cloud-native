# 数据过滤
def filter_data():
    numbers = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10]

    # 使用比较和逻辑运算符过滤数据
    even_numbers = [num for num in numbers if num % 2 == 0]
    odd_numbers = [num for num in numbers if num % 2 != 0]

    # 使用算术和比较运算符找出大于平均值的数字
    average = sum(numbers) / len(numbers)
    above_average = [num for num in numbers if num > average]

    return (f"原始数据: {numbers}\n"
            f"偶数: {even_numbers}\n"
            f"奇数: {odd_numbers}\n"
            f"平均值: {average}\n"
            f"高于平均值的数字: {above_average}")

# 调用函数
print(filter_data())
if __name__ == "__main__":
    print(filter_data())
