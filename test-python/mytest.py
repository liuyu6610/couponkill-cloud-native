import pandas as pd

# 创建一个简单的数据框
data = {'姓名': ['小明', '小红', '小张'],
        '年龄': [18, 20, 19],
        '成绩': [85, 92, 78]}
df = pd.DataFrame(data)

print(df)