import pandas as pd
import matplotlib.pyplot as plt
import seaborn as sns
import numpy as np

# 解决中文显示问题
plt.rcParams['font.sans-serif'] = ['SimHei', 'Arial Unicode MS', 'Microsoft YaHei', 'DejaVu Sans']
plt.rcParams['axes.unicode_minus'] = False

# 创建示例销售数据
data = {
    '日期': pd.date_range(start='2023-01-01', periods=90),
    '产品': ['产品A', '产品B', '产品C'] * 30,
    '区域': ['北区', '南区', '东区', '西区'] * 22 + ['北区', '南区'],
    '销售额': [100 + i * 0.5 + np.random.randint(-20, 30) for i in range(90)],
    '数量': [10 + i * 0.1 + np.random.randint(-5, 8) for i in range(90)]
}

# 创建DataFrame
sales_df = pd.DataFrame(data)
sales_df['月份'] = sales_df['日期'].dt.strftime('%Y-%m')

# 1. 按月份和产品分析销售趋势
monthly_product_sales = sales_df.groupby(['月份', '产品'])['销售额'].sum().unstack()

plt.figure(figsize=(12, 6))
monthly_product_sales.plot(marker='o')
plt.title('各产品月度销售额趋势')
plt.xlabel('月份')
plt.ylabel('销售额（元）')
plt.grid(True)
plt.legend(title='产品')
plt.tight_layout()
plt.show()

# 2. 分析各区域的销售额分布
plt.figure(figsize=(10, 6))
sns.boxplot(x='区域', y='销售额', data=sales_df)
plt.title('各区域销售额分布')
plt.show()

# 3. 分析产品和区域的销售额关系
plt.figure(figsize=(12, 8))
sns.barplot(x='产品', y='销售额', hue='区域', data=sales_df)
plt.title('各区域不同产品销售额比较')
plt.show()

# 4. 销售额与销售量的关系散点图
plt.figure(figsize=(10, 6))
sns.scatterplot(x='数量', y='销售额', hue='产品', size='区域',
               sizes=(50, 200), alpha=0.7, data=sales_df)
plt.title('销售额与销售量关系')
plt.grid(True, linestyle='--', alpha=0.7)
plt.show()