# mathtools/__init__.py
"""
MathTools: 一个简单的数学工具包

包导入机制说明：
1. 当用户执行 'import mathtools' 时，此文件会被执行
2. 此文件中的所有代码都会被执行
3. 通过 'from .module import function' 导入的函数会成为包的公共API
4. 用户可以直接通过 'mathtools.function_name()' 访问这些函数

导入方式示例：
1. import mathtools
   使用: mathtools.add(1, 2)

2. from mathtools import add, multiply
   使用: add(1, 2), multiply(3, 4)

3. from mathtools.basic import operations
   使用: operations.add(1, 2)

4. from mathtools.basic.operations import add
   使用: add(1, 2)

5. import mathtools.basic.operations as ops
   使用: ops.add(1, 2)

注意：尽管我们没有显式导入 basic 和 stats 子包，
但由于我们从这些子包中导入了函数，所以这些子包也变得可访问。
例如: from mathtools import basic, stats

类的导入和使用：
如果在子包中定义了类，需要显式导入才能使用
例如: from .basic.operations import Calculator
"""

__version__ = '0.1.0'

# 导入常用函数到包的顶层命名空间
# 这些函数可以直接通过 mathtools.function_name() 访问
from .basic.operations import add, subtract, multiply, divide
from .stats.descriptive import mean, median

# 如果要使用类，需要显式导入
from .basic.operations import Calculator

# 如果要让用户能够直接访问子包，可以显式导入它们：
from . import basic
from . import stats
# 这样用户就可以使用: from mathtools import basic, stats

# 如果只想让用户访问子包中的特定模块，可以这样做：
# from .basic import operations
# from .stats import descriptive
# 这样用户就可以使用: mathtools.operations.add(1, 2)
