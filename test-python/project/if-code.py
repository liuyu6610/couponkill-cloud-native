weight = float(input("请输入您的体重(kg): "))
height = float(input("请输入您的身高(m): "))

# 计算BMI
# 如果用户输入的是厘米，需要转换为米
if height > 3:
    height = height / 100

bmi = weight / (height ** 2)

print(f"您的BMI是: {bmi:.2f}")

if bmi < 18.5:
    print("体重过轻")
elif bmi < 24:
    print("体重正常")
elif bmi < 28:
    print("超重")
elif bmi < 32:
    print("肥胖")
else:
    print("重度肥胖")

# 可能的输出：
# 您的BMI是: 22.86
# 体重正常