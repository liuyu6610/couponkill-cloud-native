# test-my.py
# 用于演示 Python 包导入机制的测试文件

print("=" * 50)
print("Python 包导入机制演示")
print("=" * 50)

# 方法1: 导入整个包
print("\n1. 导入整个 mathtools 包:")
import mathtools
print(f"mathtools 版本: {mathtools.__version__}")
print(f"使用 mathtools.add(2, 3) = {mathtools.add(2, 3)}")
print(f"使用 mathtools.mean([1, 2, 3, 4, 5]) = {mathtools.mean([1, 2, 3, 4, 5])}")

# 方法2: 从包中导入特定函数
print("\n2. 从 mathtools 包导入特定函数:")
from mathtools import add, multiply, mean, median
print(f"使用 add(5, 7) = {add(5, 7)}")
print(f"使用 multiply(3, 4) = {multiply(3, 4)}")
print(f"使用 mean([10, 20, 30]) = {mean([10, 20, 30])}")
print(f"使用 median([1, 2, 3, 4]) = {median([1, 2, 3, 4])}")

# 方法3: 导入子包
print("\n3. 导入 mathtools 的子包:")
from mathtools import basic, stats
print(f"使用 basic.operations.add(1, 2) = {basic.operations.add(1, 2)}")
print(f"使用 stats.descriptive.mean([1, 2, 3]) = {stats.descriptive.mean([1, 2, 3])}")

# 方法4: 直接导入模块
print("\n4. 直接导入模块:")
from mathtools.basic import operations
from mathtools.stats import descriptive
print(f"使用 operations.subtract(10, 3) = {operations.subtract(10, 3)}")
print(f"使用 descriptive.median([1, 3, 2]) = {descriptive.median([1, 3, 2])}")

# 方法5: 导入特定函数
print("\n5. 从模块中导入特定函数:")
from mathtools.basic.operations import add as basic_add, divide
from mathtools.stats.descriptive import mean as stats_mean
print(f"使用 basic_add(8, 2) = {basic_add(8, 2)}")
print(f"使用 divide(15, 3) = {divide(15, 3)}")
print(f"使用 stats_mean([4, 5, 6]) = {stats_mean([4, 5, 6])}")

# 方法6: 使用绝对导入
print("\n6. 使用绝对导入:")
import mathtools.basic.operations
import mathtools.stats.descriptive
print(f"使用 mathtools.basic.operations.add(3, 4) = {mathtools.basic.operations.add(3, 4)}")
print(f"使用 mathtools.stats.descriptive.median([5, 6, 7]) = {mathtools.stats.descriptive.median([5, 6, 7])}")

# 方法7: 导入模块并重命名
print("\n7. 导入模块并重命名:")
from mathtools.basic import operations as op
from mathtools.stats import descriptive as desc
print(f"使用 op.multiply(2, 5) = {op.multiply(2, 5)}")
print(f"使用 desc.mean([2, 4, 6]) = {desc.mean([2, 4, 6])}")

# 方法8: 使用类
print("\n8. 使用类:")
from mathtools import Calculator
calc = Calculator()
print(f"使用 Calculator 计算 5 + 3 = {calc.add(5, 3)}")
print(f"使用 Calculator 计算 10 - 4 = {calc.subtract(10, 4)}")
print(f"计算历史: {calc.get_history()}")

# 方法9: 从模块导入类
print("\n9. 从模块导入类:")
from mathtools.basic.operations import Calculator as Calc
calc2 = Calc()
print(f"使用 Calc 计算 7 * 6 = {calc2.multiply(7, 6)}")
print(f"计算历史: {calc2.get_history()}")

# 方法10: 通过模块访问类
print("\n10. 通过模块访问类:")
import mathtools.basic.operations as ops
calc3 = ops.Calculator()
print(f"使用 ops.Calculator 计算 20 / 4 = {calc3.divide(20, 4)}")
print(f"计算历史: {calc3.get_history()}")

print("\n" + "=" * 50)
print("演示完成")
print("=" * 50)
