import pandas as pd
import numpy as np
from datetime import datetime

# 创建模拟销售数据
np.random.seed(42)
dates = pd.date_range(start='2023-01-01', end='2023-01-31')
products = ['Laptop', 'Phone', 'Tablet', 'Headphones', 'Monitor']

sales = []
for date in dates:
    for product in products:
        quantity = np.random.randint(1, 10)
        unit_price = {
            'Laptop': 1000,
            'Phone': 500,
            'Tablet': 300,
            'Headphones': 100,
            'Monitor': 200
        }[product]
        total = quantity * unit_price
        sales.append({
            'date': date,
            'product': product,
            'quantity': quantity,
            'unit_price': unit_price,
            'total': total
        })

# 创建DataFrame
sales_df = pd.DataFrame(sales)

# 1. 按产品汇总销售额
product_summary = sales_df.groupby('product').agg({
    'quantity': 'sum',
    'total': 'sum'
}).reset_index()
product_summary['average_price'] = product_summary['total'] / product_summary['quantity']

# 2. 按日期汇总销售额
date_summary = sales_df.groupby('date').agg({
    'total': 'sum'
}).reset_index()

# 3. 导出为Excel报告（多个表格）
with pd.ExcelWriter('sales_report.xlsx') as writer:
    sales_df.to_excel(writer, sheet_name='Raw Data', index=False)
    product_summary.to_excel(writer, sheet_name='Product Summary', index=False)
    date_summary.to_excel(writer, sheet_name='Daily Summary', index=False)

# 4. 导出汇总数据为CSV
product_summary.to_csv('product_sales.csv', index=False)

# 5. 导出为JSON（用于Web API）
date_summary.to_json('daily_sales.json', orient='records', date_format='iso')

# 6. 导出为HTML（用于报告）
with open('product_report.html', 'w') as f:
    f.write('<h1>Product Sales Summary</h1>')
    f.write(product_summary.to_html(index=False))

print("所有报告已成功导出!")