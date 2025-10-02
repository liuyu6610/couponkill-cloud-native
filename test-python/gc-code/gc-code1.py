import gc
import weakref


# 跟踪对象创建情况的类
class ObjectTracker:
    def __init__(self, object_type):
        self.object_type = object_type
        self.weak_refs = []  # 保存弱引用的列表

    def track(self, obj):
        """跟踪一个对象"""
        self.weak_refs.append(weakref.ref(obj))

    def count_alive(self):
        """计数还活着的对象"""
        count = 0
        for ref in self.weak_refs:
            if ref() is not None:  # 如果引用还有效
                count += 1
        return count


# 我们要监控的类
class TestClass:
    def __init__(self, name):
        self.name = name
        tracker.track(self)  # 注册跟踪


# 创建跟踪器
tracker = ObjectTracker(TestClass)

# 创建在函数外部仍然可访问的对象
global_obj = TestClass("global")


# 创建并丢失对象的引用
def create_and_lose_reference():
    local_obj = TestClass("local")
    # 在函数结束时，local_obj应该被回收


# 调用函数
create_and_lose_reference()

# 强制垃圾回收
gc.collect()

print(f"存活对象数: {tracker.count_alive()}")
print(f"期望结果: 1 (只有global_obj)")


# 创建循环引用并丢失
def create_cycle():
    a = TestClass("cycle_a")
    b = TestClass("cycle_b")
    a.ref = b
    b.ref = a
    # 当函数结束时，a和b形成循环引用


create_cycle()
gc.collect()  # 应该处理循环引用

print(f"垃圾回收后存活对象数: {tracker.count_alive()}")
print(f"期望结果: 1 (只有global_obj)")