# 无限循环示例
count = 0
while True:
    print(f"当前计数：{count}")
    count += 1
    if count >= 5:
        print("达到5，退出循环")
        break  # 必须使用break来退出无限循环
# 使用break
print("使用break:")
for i in range(1, 6):
    for j in range(1, 4):
        if j == 2:
            break
        print(f"i = {i}, j = {j}")

# 使用continue
print("\n使用continue:")
for i in range(1, 6):
    for j in range(1, 4):
        if j == 2:
            continue
        print(f"i = {i}, j = {j}")

import time
import random

def sensor_data_simulator():
    """模拟传感器数据流"""
    while True:
        # 生成随机温度数据
        yield {
            'timestamp': time.time(),
            'temperature': round(random.uniform(20, 30), 2)
        }
        time.sleep(1)  # 每秒生成一次数据

# 模拟读取5秒的传感器数据
sensor = sensor_data_simulator()
for _ in range(5):
    data = next(sensor)
    print(f"时间: {data['timestamp']}, 温度: {data['temperature']}°C")

def display_menu():
    print("\n==== 简易计算器 ====")
    print("1. 加法")
    print("2. 减法")
    print("3. 乘法")
    print("4. 除法")
    print("0. 退出")
    print("==================")

running = True
while running:
    display_menu()
    choice = input("请输入你的选择 (0-4): ")

    if choice == '0':
        print("谢谢使用！再见！")
        running = False
    elif choice in ['1', '2', '3', '4']:
        num1 = float(input("输入第一个数字: "))
        num2 = float(input("输入第二个数字: "))

        if choice == '1':
            print(f"{num1} + {num2} = {num1 + num2}")
        elif choice == '2':
            print(f"{num1} - {num2} = {num1 - num2}")
        elif choice == '3':
            print(f"{num1} × {num2} = {num1 * num2}")
        elif choice == '4':
            if num2 == 0:
                print("错误：除数不能为零！")
            else:
                print(f"{num1} ÷ {num2} = {num1 / num2}")
    else:
        print("无效选择！请输入0到4之间的数字。")
if __name__ == "__main__":
    print(sensor_data_simulator())