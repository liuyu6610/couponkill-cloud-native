import pandas as pd
import numpy as np

# 创建模拟的股票数据
dates = pd.date_range(start='2023-01-01', periods=10, freq='B')
stocks = ['AAPL', 'GOOGL', 'MSFT', 'AMZN']

# 生成随机价格
np.random.seed(42)
data = {}
for stock in stocks:
    data[stock] = np.random.normal(100, 5, size=len(dates)).round(2)

# 创建DataFrame
stock_df = pd.DataFrame(data, index=dates)
print("原始股票价格数据:")
print(stock_df.head())

# 1. 计算每支股票的每日回报率(百分比变化)
returns_df = stock_df.pct_change() * 100
print("\n每日回报率(%):")
print(returns_df.head())

# 2. 转换为长格式以便于可视化
stock_long = stock_df.reset_index()
stock_long = pd.melt(
    stock_long,
    id_vars=['index'],
    value_vars=stocks,
    var_name='stock',
    value_name='price'
)
stock_long.columns = ['date', 'stock', 'price']
print("\n长格式股票数据:")
print(stock_long.head())

# 3. 计算每支股票的累积回报率
stock_df_norm = stock_df / stock_df.iloc[0] * 100
print("\n累积回报(以第一天为100):")
print(stock_df_norm.head())

# 4. 计算每支股票的移动平均线
stock_df_ma = stock_df.rolling(window=3).mean()
print("\n3日移动平均线:")
print(stock_df_ma.head())