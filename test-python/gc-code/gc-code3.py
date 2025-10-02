import weakref


class Cache:
    def __init__(self):
        # 使用弱引用字典，当键没有其他引用时自动删除
        self._cache = weakref.WeakKeyDictionary()

    def get(self, key):
        return self._cache.get(key)

    def set(self, key, value):
        self._cache[key] = value

    def __len__(self):
        return len(self._cache)


# 使用缓存
cache = Cache()


# 创建一些对象
class DataObject:
    def __init__(self, value):
        self.value = value


# 添加到缓存
obj1 = DataObject("data1")
obj2 = DataObject("data2")

cache.set(obj1, "cached_value_1")
cache.set(obj2, "cached_value_2")

print(f"缓存大小: {len(cache)}")  # 输出: 缓存大小: 2
print(f"obj1的缓存值: {cache.get(obj1)}")  # 输出: obj1的缓存值: cached_value_1

# 删除obj1的引用
del obj1

# 垃圾回收后，缓存中的obj1条目自动清除
import gc

gc.collect()  # 强制垃圾回收

print(f"缓存大小: {len(cache)}")  # 输出: 缓存大小: 1