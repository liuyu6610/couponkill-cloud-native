import threading
import time


class MyThread(threading.Thread):
    def __init__(self, name):
        super().__init__()
        self.name = name

    def run(self):
        # 重写run方法，定义线程要执行的代码
        for i in range(3):
            time.sleep(1)
            print(f"线程 {self.name}: {i}")


# 创建线程实例
thread1 = MyThread("A")
thread2 = MyThread("B")

# 启动线程
thread1.start()
thread2.start()

# 等待所有线程完成
thread1.join()
thread2.join()

print("所有线程执行完毕")

def print_numbers():
    for i in range(5):
        time.sleep(1)
        print(f"Number {i}")

# 创建线程
t = threading.Thread(target=print_numbers)

# 启动线程
t.start()

print("主线程继续执行")

# 等待线程结束
t.join()

print("所有线程执行完毕")