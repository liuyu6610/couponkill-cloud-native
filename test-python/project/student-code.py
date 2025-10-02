# 定义学生成绩管理系统函数
def student_management_system():
    # 初始化学生数据字典
    students = {}

    while True:
        print("\n=== 学生成绩管理系统 ===")
        print("1. 添加学生")
        print("2. 查询学生成绩")
        print("3. 修改学生成绩")
        print("4. 删除学生")
        print("5. 显示所有学生")
        print("6. 退出系统")

        choice = input("请选择操作 (1-6): ")

        if choice == '1':
            name = input("请输入学生姓名: ")
            if name in students:
                print(f"学生 {name} 已存在!")
                continue

            try:
                chinese = float(input("请输入语文成绩: "))
                math = float(input("请输入数学成绩: "))
                english = float(input("请输入英语成绩: "))

                students[name] = {
                    "语文": chinese,
                    "数学": math,
                    "英语": english,
                    "总分": chinese + math + english,
                    "平均分": (chinese + math + english) / 3
                }
                print(f"学生 {name} 添加成功!")
            except ValueError:
                print("成绩必须是数字，请重新尝试!")

        elif choice == '2':
            name = input("请输入要查询的学生姓名: ")
            if name in students:
                print(f"\n--- {name} 的成绩信息 ---")
                for subject, score in students[name].items():
                    print(f"{subject}: {score:.2f}")
            else:
                print(f"学生 {name} 不存在!")

        elif choice == '3':
            name = input("请输入要修改成绩的学生姓名: ")
            if name in students:
                try:
                    chinese = float(input("请输入新的语文成绩: "))
                    math = float(input("请输入新的数学成绩: "))
                    english = float(input("请输入新的英语成绩: "))

                    students[name] = {
                        "语文": chinese,
                        "数学": math,
                        "英语": english,
                        "总分": chinese + math + english,
                        "平均分": (chinese + math + english) / 3
                    }
                    print(f"学生 {name} 的成绩修改成功!")
                except ValueError:
                    print("成绩必须是数字，请重新尝试!")
            else:
                print(f"学生 {name} 不存在!")

        elif choice == '4':
            name = input("请输入要删除的学生姓名: ")
            if name in students:
                del students[name]
                print(f"学生 {name} 已删除!")
            else:
                print(f"学生 {name} 不存在!")

        elif choice == '5':
            if not students:
                print("当前没有学生信息!")
            else:
                print("\n=== 所有学生信息 ===")
                for name, info in students.items():
                    print(f"\n--- {name} ---")
                    for subject, score in info.items():
                        print(f"{subject}: {score:.2f}")

        elif choice == '6':
            print("感谢使用学生成绩管理系统，再见!")
            break
        else:
            print("无效的选择，请重新输入!")

# 运行系统
if __name__ == "__main__":
    student_management_system()