# 全局变量
total_score = 0
game_active = True

def add_score(points):
    """增加玩家得分"""
    global total_score
    total_score += points
    print(f"获得{points}分! 当前总分: {total_score}")

    # 检查得分，显示不同消息
    def check_achievement():
        # 使用外层函数的变量
        if points >= 100:
            print("太棒了! 大丰收!")
        elif points >= 50:
            print("干得好!")

    check_achievement()

def game_over():
    """游戏结束"""
    global game_active
    game_active = False
    print(f"游戏结束! 最终得分: {total_score}")

# 游戏主循环
while game_active:
    # 模拟玩家获得分数
    user_input = input("按回车获得分数，输入'q'退出: ")
    if user_input.lower() == 'q':
        game_over()
    else:
        import random
        score = random.randint(10, 100)
        add_score(score)