import matplotlib.pyplot as plt
import seaborn as sns
import pandas as pd
import numpy as np

# 解决中文显示问题
plt.rcParams['font.sans-serif'] = ['SimHei', 'Arial Unicode MS', 'DejaVu Sans']
plt.rcParams['axes.unicode_minus'] = False

# 创建示例数据
np.random.seed(42)
data = {
    '年龄': np.random.normal(35, 10, 1000),
    '特征1': np.random.randn(1000),
    '特征2': np.random.randn(1000),
    '类别': np.random.choice(['A', 'B', 'C'], 1000)
}
df = pd.DataFrame(data)

# 绘制直方图
plt.figure(figsize=(10, 6))
sns.histplot(df['年龄'], bins=20, kde=True)
plt.title('年龄分布')
plt.show()

# 绘制散点图
plt.figure(figsize=(10, 6))
sns.scatterplot(x='特征1', y='特征2', hue='类别', data=df)
plt.title('特征关系散点图')
plt.show()