import random

# 简单的猜数字游戏
def number_game():
    secret = random.randint(1, 100)
    attempts = 0
    max_attempts = 7

    print("我想了一个1到100之间的数字，你有7次机会猜对它。")

    while attempts < max_attempts:
        try:
            guess = int(input(f"尝试 {attempts + 1}/{max_attempts}: 请猜一个数字: "))
            attempts += 1

            if guess < secret:
                print("太小了！")
            elif guess > secret:
                print("太大了！")
            else:
                print(f"恭喜！你用了{attempts}次猜对了！")
                return
        except ValueError:
            print("请输入一个有效的数字！")

    print(f"游戏结束！正确答案是{secret}。")

# 实际运行游戏需要调用此函数
# number_game()
if __name__ == "__main__":
    print(number_game())